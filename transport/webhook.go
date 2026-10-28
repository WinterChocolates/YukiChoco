package transport

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
)

// WebhookConfig 是 WebHook 事件接收器配置。
type WebhookConfig struct {
	// AccessToken 非空且 VerifyToken 为 true 时校验
	// `Authorization: Bearer <AccessToken>` 或 `?access_token=`。
	AccessToken string

	// VerifyToken 控制是否校验 access_token（AccessToken 非空时默认开启）。
	VerifyToken bool

	// Secret 非空时校验 `X-Signature: sha1=<hex(HMAC-SHA1(body))>`。
	Secret string

	// MaxBytes 限制请求体大小，零值默认 16 MiB。
	MaxBytes int64

	// Logger 用于传输层日志，零值丢弃。
	Logger *slog.Logger
}

func (c *WebhookConfig) setDefaults() {
	if c.MaxBytes <= 0 {
		c.MaxBytes = defaultMaxMessageBytes
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// Webhook 接收协议端 POST 过来的事件。
//
// 它只做校验与读取，事件如何分发由 OnEvent 回调决定。
type Webhook struct {
	cfg     WebhookConfig
	onEvent func([]byte)
	closing atomic.Bool
}

// NewWebhook 创建 WebHook 接收器。
func NewWebhook(cfg WebhookConfig) *Webhook {
	cfg.setDefaults()
	return &Webhook{cfg: cfg}
}

// OnEvent 设置事件回调。回调在 HTTP 请求 goroutine 中同步执行，应尽快返回。
func (w *Webhook) OnEvent(fn func([]byte)) { w.onEvent = fn }

// Close 标记接收器已关闭；之后的请求返回 503。
func (w *Webhook) Close() { w.closing.Store(true) }

// Handler 返回 http.Handler。
func (w *Webhook) Handler() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if w.closing.Load() {
			http.Error(rw, "closed", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPost {
			rw.Header().Set("Allow", http.MethodPost)
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, w.cfg.MaxBytes+1))
		if err != nil {
			http.Error(rw, "read error", http.StatusBadRequest)
			return
		}
		if int64(len(body)) > w.cfg.MaxBytes {
			http.Error(rw, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if !w.authorized(r) {
			w.cfg.Logger.Warn("transport: WebHook 鉴权失败", "remote", r.RemoteAddr)
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !w.verifySignature(r, body) {
			w.cfg.Logger.Warn("transport: WebHook 签名校验失败", "remote", r.RemoteAddr)
			http.Error(rw, "invalid signature", http.StatusForbidden)
			return
		}
		if !json.Valid(body) {
			http.Error(rw, "invalid json", http.StatusBadRequest)
			return
		}
		if w.onEvent != nil {
			w.onEvent(body)
		}
		rw.WriteHeader(http.StatusNoContent)
	})
}

func (w *Webhook) authorized(r *http.Request) bool {
	if w.cfg.AccessToken == "" || !w.cfg.VerifyToken {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	if got == "" && r.URL.Query().Get("access_token") != "" {
		got = "Bearer " + r.URL.Query().Get("access_token")
	}
	want := "Bearer " + w.cfg.AccessToken
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (w *Webhook) verifySignature(r *http.Request, body []byte) bool {
	if w.cfg.Secret == "" {
		return true
	}
	sig := r.Header.Get("X-Signature")
	sig = strings.TrimPrefix(sig, "sha1=")
	if sig == "" {
		return false
	}
	mac := hmac.New(sha1.New, []byte(w.cfg.Secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(want))
}
