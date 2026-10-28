package YukiChoco

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

// TransportKind 选择 OneBot v11 的通信方式。
type TransportKind string

const (
	// TransportHTTP 使用 HTTP 调用 API，事件通过 WebHook 推送到
	// Client.WebhookHandler() 返回的 handler。
	TransportHTTP TransportKind = "http"

	// TransportForwardWS 由 SDK 主动连接协议端 WebSocket。
	// 未设置 HTTP.APIBaseURL 时，API 调用也走该连接（echo 关联）。
	TransportForwardWS TransportKind = "forward-ws"

	// TransportReverseWS 由 SDK 启动 WebSocket 服务，等待协议端连接。
	// API 调用通过已建立的连接发送。
	TransportReverseWS TransportKind = "reverse-ws"

	// TransportNone 只调用 HTTP API，不接收事件。
	TransportNone TransportKind = "none"
)

// Config 是客户端配置。零值字段会由 New 填充默认值。
type Config struct {
	// AccessToken 是协议端配置的 access_token。
	//
	// 用于 HTTP Authorization、WS 握手、反向 WS / WebHook 鉴权。
	// 该值不会出现在日志与错误信息中。
	AccessToken string

	// HTTPClient 可注入自定义 HTTP 客户端（代理、TLS、连接池等）。
	HTTPClient *http.Client

	// APITimeout 是单次 API 调用的超时。零值默认 15s；负值表示完全交给 ctx。
	APITimeout time.Duration

	// Retry 控制重试。
	Retry RetryConfig

	// RateLimit 控制客户端限流。
	RateLimit RateLimitConfig

	// Logger 是结构化日志器，零值丢弃全部日志。
	Logger *slog.Logger

	// UserAgent 是 HTTP 请求的 User-Agent。
	// 零值使用 "YukiChoco/<版本> (OneBot v11)"。
	UserAgent string

	// DebugDump 非空时，每次调用后写入一行脱敏后的调试信息。
	DebugDump io.Writer

	// MaxResponseBytes 限制单次响应体 / 单条事件的字节数，零值默认 16 MiB。
	MaxResponseBytes int64

	// Transport 选择通信方式，零值等价于 TransportHTTP。
	Transport TransportKind

	// HTTP 配置 HTTP API 与 WebHook。
	HTTP HTTPConfig

	// ForwardWS 配置正向 WebSocket。
	ForwardWS ForwardWSConfig

	// ReverseWS 配置反向 WebSocket 服务端。
	ReverseWS ReverseWSConfig

	// Webhook 配置 WebHook 鉴权与签名。
	Webhook WebhookConfig

	// Dispatch 控制事件分发。
	Dispatch DispatchConfig
}

// HTTPConfig 是 HTTP API / WebHook 的配置。
type HTTPConfig struct {
	// APIBaseURL 是协议端 HTTP API 根地址，例如 "http://127.0.0.1:5700"。
	//
	// TransportHTTP / TransportNone 时必填（零值默认 http://127.0.0.1:5700）。
	// 其它模式下非空时，API 调用改为走 HTTP（事件仍走 WS）。
	APIBaseURL string
}

// ForwardWSConfig 是正向 WebSocket 的配置。
type ForwardWSConfig struct {
	// URL 是连接地址。
	//
	// 通用模式：事件与 API 响应共用一个连接，例如 "ws://127.0.0.1:3001/"。
	// 分离模式：把 EventURL 指向 "/event"，URL 指向 "/api"。
	URL string

	// EventURL 是单独的事件连接地址；留空表示通用单连接模式。
	EventURL string

	// Header 是额外的握手请求头。
	Header http.Header

	// Dialer 可注入自定义 dialer。
	Dialer *websocket.Dialer

	// DialTimeout 是拨号超时，零值默认 10s。
	DialTimeout time.Duration

	// IdleTimeout 是读空闲超时，零值默认 90s。
	IdleTimeout time.Duration

	// PingInterval 是 ping 间隔，零值默认 30s。
	PingInterval time.Duration

	// Reconnect 控制断线重连。
	Reconnect ReconnectConfig
}

// ReverseWSConfig 是反向 WebSocket 服务端的配置。
type ReverseWSConfig struct {
	// Listen 是监听地址，零值默认 "127.0.0.1:8080"。
	Listen string

	// Path 是 WebSocket 路径，零值默认 "/onebot/v11/ws"。
	Path string

	// CheckOrigin 可自定义 Origin 校验；零值允许所有来源。
	CheckOrigin func(*http.Request) bool

	// IdleTimeout 是读空闲超时，零值默认 90s。
	IdleTimeout time.Duration

	// PingInterval 是 ping 间隔，零值默认 30s。
	PingInterval time.Duration
}

// WebhookConfig 是 WebHook 接收器的配置。
type WebhookConfig struct {
	// VerifyToken 为 true 时校验 Authorization: Bearer <AccessToken>。
	// AccessToken 非空时默认开启。
	VerifyToken bool

	// Secret 非空时校验 X-Signature（go-cqhttp 的 HMAC-SHA1 约定）。
	Secret string
}

// ReconnectConfig 控制正向 WS 的自动重连。
type ReconnectConfig struct {
	// Disable 为 true 时断开后不再重连。
	Disable bool

	// InitialDelay 是首次重连等待，零值默认 1s。
	InitialDelay time.Duration

	// MaxDelay 是退避上限，零值默认 60s。
	MaxDelay time.Duration

	// Multiplier 是退避倍数，零值默认 2。
	Multiplier float64

	// Jitter 是抖动比例，0.2 表示 ±20%。零值默认 0.2；负值关闭抖动。
	Jitter float64

	// MaxAttempts 是最大重连次数，0 表示无限。
	MaxAttempts int
}

// DispatchConfig 控制事件分发（细节见 event.Config）。
type DispatchConfig struct {
	// Workers 是并发度 / 会话分片数。零值使用 runtime.NumCPU()。
	Workers int
	// Buffer 是事件队列容量。零值默认 1024。
	Buffer int
	// Mode 是顺序策略。零值等价于 DispatchConcurrent。
	Mode DispatchMode
	// Backpressure 是队列满时的策略。零值等价于 BackpressureDropNewest。
	Backpressure Backpressure
	// HandlerTimeout 是单条事件的处理超时。零值不限。
	HandlerTimeout time.Duration
	// KeyFunc 用于按会话有序模式的会话划分；零值使用 event.DefaultKeyFunc。
	KeyFunc func(types.Event) string
}

// DispatchMode 是事件分发顺序策略（别名，定义在 event 包）。
type DispatchMode = event.Mode

// 分发顺序策略。
const (
	DispatchConcurrent        = event.ModeConcurrent
	DispatchSequential        = event.ModeSequential
	DispatchPerPeerSequential = event.ModePerPeerSequential
)

// Backpressure 是事件队列满时的策略（别名，定义在 event 包）。
type Backpressure = event.Backpressure

// 背压策略。
const (
	BackpressureDropNewest = event.BackpressureDropNewest
	BackpressureBlock      = event.BackpressureBlock
)

// Middleware 是事件中间件（别名，定义在 event 包）。
type Middleware = event.Middleware

// RetryConfig 控制自动重试。
//
// OneBot 没有幂等键，默认只重试 get_* / can_send_* 等只读动作；
// send_* 等有副作用的动作需要显式开启 RetryNonIdempotent。
type RetryConfig struct {
	// MaxAttempts 是最大尝试次数（含首次）。零值默认 3；<=1 表示不重试。
	MaxAttempts int

	// BaseDelay 是首次重试等待，零值默认 200ms。
	BaseDelay time.Duration

	// MaxDelay 是退避上限，零值默认 3s。
	MaxDelay time.Duration

	// RetryNonIdempotent 为 true 时，有副作用的动作也会重试（可能重复发送）。
	RetryNonIdempotent bool
}

// RateLimitConfig 是令牌桶限流配置。
type RateLimitConfig struct {
	// QPS 是每秒补充的令牌数。<=0 表示关闭限流。
	QPS float64

	// Burst 是桶容量。零值默认 max(1, ceil(QPS))。
	Burst int
}

func (c *Config) setDefaults() {
	if c.Transport == "" {
		c.Transport = TransportHTTP
	}
	if c.HTTP.APIBaseURL == "" && (c.Transport == TransportHTTP || c.Transport == TransportNone) {
		c.HTTP.APIBaseURL = "http://127.0.0.1:5700"
	}
	if c.APITimeout == 0 {
		c.APITimeout = 15 * time.Second
	}
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BaseDelay <= 0 {
		c.Retry.BaseDelay = 200 * time.Millisecond
	}
	if c.Retry.MaxDelay <= 0 {
		c.Retry.MaxDelay = 3 * time.Second
	}
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = 16 << 20
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if c.UserAgent == "" {
		c.UserAgent = "YukiChoco/" + Version + " (OneBot v11)"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}
	if c.RateLimit.QPS > 0 && c.RateLimit.Burst <= 0 {
		burst := int(c.RateLimit.QPS)
		if burst < 1 {
			burst = 1
		}
		c.RateLimit.Burst = burst
	}

	f := &c.ForwardWS
	if f.DialTimeout <= 0 {
		f.DialTimeout = 10 * time.Second
	}
	if f.IdleTimeout <= 0 {
		f.IdleTimeout = 90 * time.Second
	}
	if f.PingInterval <= 0 {
		f.PingInterval = 30 * time.Second
	}
	if f.Reconnect.InitialDelay <= 0 {
		f.Reconnect.InitialDelay = time.Second
	}
	if f.Reconnect.MaxDelay <= 0 {
		f.Reconnect.MaxDelay = 60 * time.Second
	}
	if f.Reconnect.Multiplier <= 0 {
		f.Reconnect.Multiplier = 2
	}
	if f.Reconnect.Jitter == 0 {
		f.Reconnect.Jitter = 0.2
	}

	if c.ReverseWS.Listen == "" {
		c.ReverseWS.Listen = "127.0.0.1:8080"
	}
	if c.ReverseWS.Path == "" {
		c.ReverseWS.Path = "/onebot/v11/ws"
	}
	if c.ReverseWS.IdleTimeout <= 0 {
		c.ReverseWS.IdleTimeout = 90 * time.Second
	}
	if c.ReverseWS.PingInterval <= 0 {
		c.ReverseWS.PingInterval = 30 * time.Second
	}
	if c.AccessToken != "" {
		c.Webhook.VerifyToken = true
	}
}

func (c *Config) validate() error {
	if c.AccessToken != "" && strings.ContainsAny(c.AccessToken, "\r\n") {
		return errors.New("yukichoco: Config.AccessToken 不能包含换行")
	}

	if base := strings.TrimSpace(c.HTTP.APIBaseURL); base != "" {
		u, err := url.Parse(base)
		if err != nil {
			return fmt.Errorf("yukichoco: HTTP.APIBaseURL 不是合法 URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("yukichoco: HTTP.APIBaseURL 的 scheme 必须是 http(s)，实际为 %q", u.Scheme)
		}
		if u.Host == "" {
			return errors.New("yukichoco: HTTP.APIBaseURL 缺少主机名")
		}
		c.HTTP.APIBaseURL = strings.TrimRight(base, "/")
	}

	switch c.Transport {
	case TransportHTTP, TransportNone:
		if c.HTTP.APIBaseURL == "" {
			return fmt.Errorf("yukichoco: Transport=%s 时 HTTP.APIBaseURL 不能为空", c.Transport)
		}
	case TransportForwardWS:
		f := &c.ForwardWS
		f.URL = strings.TrimSpace(f.URL)
		if f.URL == "" {
			return errors.New("yukichoco: Transport=forward-ws 时 ForwardWS.URL 不能为空")
		}
		if err := validateWSURL("ForwardWS.URL", f.URL); err != nil {
			return err
		}
		f.EventURL = strings.TrimSpace(f.EventURL)
		if f.EventURL == "" {
			if derived, ok := deriveEventURL(f.URL); ok {
				f.EventURL = derived
			}
		}
		if f.EventURL != "" {
			if err := validateWSURL("ForwardWS.EventURL", f.EventURL); err != nil {
				return err
			}
			if f.EventURL == f.URL {
				// 显式写成同一个地址等价于通用模式。
				f.EventURL = ""
			}
		}
		if f.EventURL == "" && c.HTTP.APIBaseURL == "" {
			if u, err := url.Parse(f.URL); err == nil &&
				strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/event") &&
				!strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/api") {
				return errors.New("yukichoco: ForwardWS.URL 指向 /event 时，请同时设置 ForwardWS.EventURL 或 HTTP.APIBaseURL")
			}
		}
	case TransportReverseWS:
		if strings.TrimSpace(c.ReverseWS.Listen) == "" {
			return errors.New("yukichoco: Transport=reverse-ws 时 ReverseWS.Listen 不能为空")
		}
		if c.HTTP.APIBaseURL != "" {
			// 允许混合：HTTP 调 API，反向 WS 收事件。
			c.Logger.Debug("yukichoco: 反向 WS 模式同时配置了 HTTP API，API 调用将走 HTTP")
		}
	default:
		return fmt.Errorf("yukichoco: 不支持的 Transport %q", c.Transport)
	}
	return nil
}

func validateWSURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("yukichoco: %s 不是合法 URL: %w", name, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return fmt.Errorf("yukichoco: %s 的 scheme 必须是 ws 或 wss，实际为 %q", name, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("yukichoco: %s 缺少主机名", name)
	}
	return nil
}

// deriveEventURL 在 URL 指向 /api 时推导同主机的 /event。
//
// 其它路径（含根路径）视为通用单连接模式，返回 false。
func deriveEventURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/api") {
		u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api") + "/event"
		u.RawQuery = ""
		return u.String(), true
	}
	return "", false
}

// String 返回脱敏后的配置摘要，可安全写日志。
func (c Config) String() string {
	return fmt.Sprintf("Config{Transport: %s, APIBaseURL: %s, ForwardWS.URL: %s, ReverseWS.Listen: %s, AccessToken: %s, APITimeout: %s, Retry.MaxAttempts: %d, RateLimit.QPS: %g, Dispatch.Mode: %s}",
		c.Transport, safeURL(c.HTTP.APIBaseURL), safeURL(c.ForwardWS.URL), c.ReverseWS.Listen,
		redactSecret(c.AccessToken), c.APITimeout, c.Retry.MaxAttempts, c.RateLimit.QPS, c.Dispatch.Mode)
}

// LogValue 让 slog 输出脱敏后的配置。
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

// safeURL 去掉 URL 中的 userinfo 与常见 token 查询参数，避免日志泄漏。
func safeURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	// 直接去掉 userinfo，避免把 token 写进日志。
	u.User = nil
	q := u.Query()
	for _, key := range []string{"access_token", "token", "secret"} {
		if q.Has(key) {
			q.Set(key, "***")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func redactSecret(s string) string {
	if s == "" {
		return "(未设置)"
	}
	return "***"
}
