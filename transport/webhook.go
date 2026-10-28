package transport

import (
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
)

const defaultWebHookBuffer = 256

// WebHookConfig 是 WebHook 事件源的配置。
type WebHookConfig struct {
	// AccessToken 非空且 VerifyToken 为 true 时，校验
	// `Authorization: Bearer <AccessToken>`。
	AccessToken string

	// VerifyToken 控制是否校验 Authorization 头。
	//
	// Milky 1.1 起 WebHook 支持基于 Authorization 的鉴权，但协议端不保证发送；
	// 因此默认关闭，需要时显式开启。
	VerifyToken bool

	// Buffer 是事件队列容量，零值默认 256。
	//
	// 队列满时 Handler 返回 429，由协议端决定是否重试（多数实现会丢弃）。
	Buffer int

	// Logger 用于传输层日志。零值丢弃。
	Logger *slog.Logger
}

func (c *WebHookConfig) setDefaults() {
	if c.Buffer <= 0 {
		c.Buffer = defaultWebHookBuffer
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// WebHook 是 WebHook 事件源：协议端 POST 事件到应用端。
//
// 与 WS/SSE 不同，它没有"连接"的概念：Handler 接收的请求被放入队列，
// Connect 返回一个从队列读取的会话。同一个 WebHook 实例只应被一个 Client 使用。
type WebHook struct {
	cfg WebHookConfig

	queue   chan []byte
	dropped atomic.Uint64

	closeOnce sync.Once
	closed    chan struct{}
}

// NewWebHook 创建 WebHook 事件源。
func NewWebHook(cfg WebHookConfig) *WebHook {
	cfg.setDefaults()
	return &WebHook{
		cfg:    cfg,
		queue:  make(chan []byte, cfg.Buffer),
		closed: make(chan struct{}),
	}
}

// Name 实现 Source。
func (w *WebHook) Name() string { return SourceWebHook }

// Connect 实现 Source：返回从队列读取的会话，不做任何网络操作。
func (w *WebHook) Connect(ctx context.Context) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-w.closed:
		return nil, ErrClosed
	default:
	}
	return &webhookSession{queue: w.queue, closed: w.closed, done: make(chan struct{})}, nil
}

// Handler 返回接收事件的 http.Handler，由调用方挂载到任意路径。
//
// 期望 POST + JSON body（与 WS 推送的格式相同），成功返回 204。
func (w *WebHook) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.Header().Set("Allow", http.MethodPost)
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if w.cfg.VerifyToken && w.cfg.AccessToken != "" {
			got := r.Header.Get("Authorization")
			want := "Bearer " + w.cfg.AccessToken
			// 常量时间比较，避免逐字符时序泄漏。
			if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
				w.cfg.Logger.Warn("milky/transport: WebHook 鉴权失败", "remote", r.RemoteAddr)
				rw.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if ct := r.Header.Get("Content-Type"); ct != "" && !hasJSONPrefix(ct) {
			rw.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, defaultMaxMessageBytes))
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(body) == 0 {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		select {
		case <-w.closed:
			rw.WriteHeader(http.StatusServiceUnavailable)
			return
		default:
		}
		select {
		case w.queue <- body:
			rw.WriteHeader(http.StatusNoContent)
		default:
			w.dropped.Add(1)
			w.cfg.Logger.Warn("milky/transport: WebHook 队列已满，拒绝事件", "buffer", w.cfg.Buffer)
			rw.WriteHeader(http.StatusTooManyRequests)
		}
	})
}

func hasJSONPrefix(ct string) bool {
	const p = "application/json"
	if len(ct) < len(p) {
		return false
	}
	return ct[:len(p)] == p
}

// Dropped 返回因队列满被拒绝的事件数。
func (w *WebHook) Dropped() uint64 { return w.dropped.Load() }

// Close 关闭事件源：之后的 Connect 返回 ErrClosed，Handler 返回 503。
func (w *WebHook) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
}

// webhookSession 从 WebHook 的队列读取事件。
type webhookSession struct {
	queue  <-chan []byte
	closed chan struct{}
	done   chan struct{}
	once   sync.Once
}

// Next 实现 Session。
func (s *webhookSession) Next(ctx context.Context) ([]byte, error) {
	select {
	case raw := <-s.queue:
		return raw, nil
	case <-s.done:
		return nil, ErrClosed
	case <-s.closed:
		return nil, ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close 实现 Session。
func (s *webhookSession) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}
