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
	defaultWriteTimeout    = 10 * time.Second
	defaultMaxMessageBytes = 16 << 20
)

// DialConfig 是 WebSocket 连接的配置。
type DialConfig struct {
	// URL 是 ws:// 或 wss:// 地址。
	URL string

	// AccessToken 用于鉴权：优先 Authorization: Bearer，
	// 握手返回 401 时自动降级为 ?access_token= 查询参数。
	AccessToken string

	// Header 是额外的握手请求头。
	Header http.Header

	// Dialer 可注入自定义 dialer（代理、TLS 等）。
	Dialer *websocket.Dialer

	// DialTimeout 是拨号超时，零值默认 10s。
	DialTimeout time.Duration

	// IdleTimeout 是读空闲超时，零值默认 90s；负值表示不设置读超时。
	IdleTimeout time.Duration

	// PingInterval 是 ping 帧间隔，零值默认 30s；负值表示不发送 ping。
	PingInterval time.Duration

	// WriteTimeout 是单次写入超时，零值默认 10s。
	WriteTimeout time.Duration

	// MaxMessageBytes 限制单条消息大小，零值默认 16 MiB。
	MaxMessageBytes int64

	// Logger 用于传输层日志，零值丢弃。
	Logger *slog.Logger
}

func (c *DialConfig) setDefaults() {
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.PingInterval == 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaultWriteTimeout
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

// Conn 是一条已建立的 WebSocket JSON 连接。
//
// Read/Write 可并发调用（写入串行化）。Close 可重复调用。
type Conn struct {
	ws     *websocket.Conn
	cfg    DialConfig
	header http.Header

	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    chan struct{}
	pingDone  chan struct{}
}

// Dial 建立一条正向 WebSocket 连接。
func Dial(ctx context.Context, cfg DialConfig) (*Conn, error) {
	cfg.setDefaults()
	if cfg.URL == "" {
		return nil, errors.New("transport: URL 不能为空")
	}
	header := cfg.Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	if cfg.AccessToken != "" {
		header.Set("Authorization", "Bearer "+cfg.AccessToken)
	}

	dialCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	ws, resp, err := cfg.Dialer.DialContext(dialCtx, cfg.URL, header)
	if err != nil {
		// 某些环境会拒绝自定义 Header，降级为查询参数再试一次。
		if cfg.AccessToken != "" && resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if resp.Body != nil {
				resp.Body.Close()
			}
			fallback, ferr := queryTokenURL(cfg.URL, cfg.AccessToken)
			if ferr == nil {
				retryHeader := cfg.Header.Clone()
				if retryHeader == nil {
					retryHeader = http.Header{}
				}
				ws, resp, err = cfg.Dialer.DialContext(dialCtx, fallback, retryHeader)
			}
		}
	}
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
			}
			return nil, &StatusError{StatusCode: resp.StatusCode, Status: resp.Status}
		}
		return nil, fmt.Errorf("transport: 连接 %s 失败: %w", cfg.URL, err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	c := newConn(ws, cfg, header)
	c.start()
	return c, nil
}

func newConn(ws *websocket.Conn, cfg DialConfig, header http.Header) *Conn {
	cfg.setDefaults()
	ws.SetReadLimit(cfg.MaxMessageBytes)
	c := &Conn{
		ws:       ws,
		cfg:      cfg,
		header:   header,
		closed:   make(chan struct{}),
		pingDone: make(chan struct{}),
	}
	if cfg.IdleTimeout > 0 {
		_ = ws.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
	}
	ws.SetPongHandler(func(string) error {
		if cfg.IdleTimeout > 0 {
			return ws.SetReadDeadline(time.Now().Add(cfg.IdleTimeout))
		}
		return nil
	})
	return c
}

func (c *Conn) start() {
	if c.cfg.PingInterval <= 0 {
		close(c.pingDone)
		return
	}
	go func() {
		defer close(c.pingDone)
		ticker := time.NewTicker(c.cfg.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.closed:
				return
			case <-ticker.C:
				deadline := time.Now().Add(c.cfg.WriteTimeout)
				if err := c.ws.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					c.cfg.Logger.Debug("transport: ping 发送失败，关闭连接", "err", err)
					_ = c.Close()
					return
				}
			}
		}
	}()
}

// Read 读取下一条消息。
func (c *Conn) Read(ctx context.Context) ([]byte, error) {
	select {
	case <-c.closed:
		return nil, ErrClosed
	default:
	}
	if c.cfg.IdleTimeout > 0 {
		_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.IdleTimeout))
	}
	_, data, err := c.ws.ReadMessage()
	if err != nil {
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil, ErrClosed
		}
		return nil, contextError(ctx, fmt.Errorf("transport: 读取消息失败: %w", err))
	}
	return data, nil
}

// Write 写入一条消息。并发安全。
func (c *Conn) Write(ctx context.Context, data []byte) error {
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	deadline := time.Now().Add(c.cfg.WriteTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.ws.SetWriteDeadline(deadline)
	if err := c.ws.WriteMessage(websocket.TextMessage, data); err != nil {
		return contextError(ctx, fmt.Errorf("transport: 写入消息失败: %w", err))
	}
	return nil
}

// Close 关闭连接（发送正常关闭帧后释放资源）。
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		deadline := time.Now().Add(time.Second)
		_ = c.ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
		err = c.ws.Close()
	})
	return err
}

// Done 在连接关闭后关闭。
func (c *Conn) Done() <-chan struct{} { return c.closed }

// RemoteAddr 返回对端地址。
func (c *Conn) RemoteAddr() string { return c.ws.RemoteAddr().String() }

// Header 返回握手请求头（反向 WS 中可用于读取 X-Self-ID 等）。
func (c *Conn) Header() http.Header { return c.header.Clone() }

func queryTokenURL(rawURL, token string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("access_token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
