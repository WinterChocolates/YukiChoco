package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultDialTimeout     = 10 * time.Second
	defaultIdleTimeout     = 90 * time.Second
	defaultPingInterval    = 30 * time.Second
	defaultMaxMessageBytes = 16 << 20
	writeTimeout           = 10 * time.Second
)

// WSConfig 是 WebSocket 事件源的配置。
type WSConfig struct {
	// URL 是事件通道地址，例如 ws://127.0.0.1:6700/event。
	URL string

	// AccessToken 用于鉴权：优先 Authorization: Bearer，握手返回 401 时
	// 自动降级为 ?access_token= 查询参数。
	AccessToken string

	// Header 是额外的握手请求头。
	Header http.Header

	// Dialer 可注入自定义 dialer（代理、TLS 等）。零值使用默认 dialer。
	Dialer *websocket.Dialer

	// DialTimeout 是单次拨号超时。零值默认 10s。
	DialTimeout time.Duration

	// IdleTimeout 是读空闲超时：超过该时间没有收到任何帧（含 pong）即判定
	// 连接已死，返回错误让上层重连。零值默认 90s。
	IdleTimeout time.Duration

	// PingInterval 是 ping 帧发送间隔，用于探测半开连接。零值默认 30s。
	PingInterval time.Duration

	// MaxMessageBytes 限制单条事件大小。零值默认 16 MiB。
	MaxMessageBytes int64

	// Logger 用于传输层日志。零值丢弃。
	Logger *slog.Logger
}

func (c *WSConfig) setDefaults() {
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.PingInterval <= 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultMaxMessageBytes
	}
	if c.Dialer == nil {
		c.Dialer = websocket.DefaultDialer
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// WS 是 WebSocket 事件源。
//
// Milky 的事件通道没有应用层心跳或 opcode 帧：每条消息就是一个事件 JSON，
// 因此这里只使用 WebSocket 自带的 ping/pong 维持与探测连接。
type WS struct {
	cfg WSConfig
}

// NewWS 创建 WebSocket 事件源。
func NewWS(cfg WSConfig) *WS {
	cfg.setDefaults()
	return &WS{cfg: cfg}
}

// Name 实现 Source。
func (w *WS) Name() string { return SourceWS }

// Connect 实现 Source。
func (w *WS) Connect(ctx context.Context) (Session, error) {
	header := w.cfg.Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	if w.cfg.AccessToken != "" {
		header.Set("Authorization", "Bearer "+w.cfg.AccessToken)
	}

	dialCtx, cancel := context.WithTimeout(ctx, w.cfg.DialTimeout)
	defer cancel()

	conn, resp, err := w.cfg.Dialer.DialContext(dialCtx, w.cfg.URL, header)
	if err != nil {
		// 握手被拒（通常是某些环境不允许自定义 Header）：降级为 query 传 token 再试一次。
		if w.cfg.AccessToken != "" && resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if fallback, ferr := queryTokenURL(w.cfg.URL, w.cfg.AccessToken); ferr == nil {
				w.cfg.Logger.Debug("milky/transport: Authorization 头被拒，改用 query 参数重试")
				retryHeader := w.cfg.Header.Clone()
				if retryHeader == nil {
					retryHeader = http.Header{}
				}
				conn, resp, err = w.cfg.Dialer.DialContext(dialCtx, fallback, retryHeader)
			}
		}
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
			}
			if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
				return nil, fmt.Errorf("%w: %d %s", &StatusError{StatusCode: resp.StatusCode, Status: resp.Status}, resp.StatusCode, resp.Status)
			}
			return nil, fmt.Errorf("milky/transport: 连接 %s 失败: %w", w.cfg.URL, err)
		}
	}

	sess := &wsSession{
		conn:      conn,
		logger:    w.cfg.Logger,
		idle:      w.cfg.IdleTimeout,
		pingEvery: w.cfg.PingInterval,
		pingDone:  make(chan struct{}),
	}
	sess.start(ctx, w.cfg.MaxMessageBytes)
	return sess, nil
}

// queryTokenURL 在 URL 上附加 access_token 查询参数。
func queryTokenURL(raw, token string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("access_token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// wsSession 是一条 WebSocket 连接。
type wsSession struct {
	conn      *websocket.Conn
	logger    *slog.Logger
	idle      time.Duration
	pingEvery time.Duration

	writeMu   sync.Mutex
	closeOnce sync.Once
	pingDone  chan struct{}
	cancel    context.CancelFunc
}

// start 初始化读限制、ping/pong 处理与看门狗。
func (s *wsSession) start(parent context.Context, maxMessage int64) {
	s.conn.SetReadLimit(maxMessage)

	// 收到 pong 说明对端还活着，续期读超时。
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(s.idle))
	})
	// 自定义 ping handler 后需自行回 pong（gorilla 默认行为被覆盖）。
	s.conn.SetPingHandler(func(data string) error {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		if err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			return err
		}
		return s.conn.WriteMessage(websocket.PongMessage, []byte(data))
	})

	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel

	// 看门狗：上层取消 ctx 时关闭连接，让阻塞中的 Next 立刻返回。
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.pingDone:
		}
	}()

	go s.pingLoop(ctx)
}

// pingLoop 定期发送 ping 帧。
func (s *wsSession) pingLoop(ctx context.Context) {
	defer close(s.pingDone)
	ticker := time.NewTicker(s.pingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.writeMu.Lock()
			err := s.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err == nil {
				err = s.conn.WriteMessage(websocket.PingMessage, nil)
			}
			s.writeMu.Unlock()
			if err != nil {
				s.logger.Debug("milky/transport: ping 失败，关闭连接", "err", err)
				_ = s.Close()
				return
			}
		}
	}
}

// Next 实现 Session。
func (s *wsSession) Next(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.conn.SetReadDeadline(time.Now().Add(s.idle)); err != nil {
			return nil, err
		}
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if errors.Is(err, websocket.ErrCloseSent) {
				return nil, ErrClosed
			}
			return nil, fmt.Errorf("milky/transport: 读取事件失败: %w", err)
		}
		// 事件一定是文本消息；二进制消息按同样规则处理以兼容实现差异。
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}
		if len(data) == 0 {
			continue
		}
		return data, nil
	}
}

// Close 实现 Session，可重复调用。
func (s *wsSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		err = s.conn.Close()
	})
	return err
}
