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

	"github.com/WinterChocolates/YukiChoco/types"
)

// Config 是客户端的配置。零值字段会由 New 填充默认值。
type Config struct {
	// BaseURL 是协议端的 HTTP 地址，例如 "http://127.0.0.1:6700"。
	//
	// 必填。末尾的斜杠会被自动去掉。
	BaseURL string

	// AccessToken 是协议端的 access_token（对应 Authorization: Bearer）。
	//
	// 可选；为空表示协议端未开启鉴权。该值不会出现在日志与错误信息中。
	AccessToken string

	// HTTPClient 可注入自定义的 HTTP 客户端（代理、TLS、HTTP/2、连接池等）。
	//
	// 为零值时使用内置客户端（连接复用、总超时由 APITimeout 控制）。
	HTTPClient *http.Client

	// APITimeout 是单次 API 调用的超时（含重试链之外的每一次尝试）。
	//
	// 零值默认 15s；负值表示不额外设置超时（完全交给 ctx）。
	APITimeout time.Duration

	// Retry 控制重试策略。
	Retry RetryConfig

	// RateLimit 控制客户端限流。
	RateLimit RateLimitConfig

	// Logger 是结构化日志器。零值时丢弃全部日志。
	Logger *slog.Logger

	// UserAgent 是请求的 User-Agent。零值时使用 "YukiChoco/<版本> (Milky <协议版本>)"。
	UserAgent string

	// DebugDump 非空时，每次调用后写入一行脱敏后的调试信息。
	DebugDump io.Writer

	// MaxResponseBytes 限制单次响应体大小，防止异常响应耗尽内存。
	//
	// 零值默认 16 MiB。
	MaxResponseBytes int64
}

// RetryConfig 控制自动重试。
//
// 只有幂等端点（get_/set_/mark_/delete_/move_/rename_/accept_/reject_/quit_/persist_）
// 会默认重试：Milky 没有幂等键，重试 send_* 会造成重复消息。
type RetryConfig struct {
	// MaxAttempts 是幂等端点的最大尝试次数（含首次）。零值默认 3；<=1 表示不重试。
	MaxAttempts int

	// BaseDelay 是首次重试前的等待时间。零值默认 200ms。
	BaseDelay time.Duration

	// MaxDelay 是退避上限。零值默认 3s。
	MaxDelay time.Duration

	// RetryNonIdempotent 为 true 时，send_* 等非幂等端点也会重试（可能重复发送）。
	RetryNonIdempotent bool
}

// RateLimitConfig 是令牌桶限流配置。
type RateLimitConfig struct {
	// QPS 是每秒补充的令牌数。<=0 表示关闭限流。
	QPS float64

	// Burst 是桶容量（允许的突发量）。零值默认 max(1, ceil(QPS))。
	Burst int
}

// setDefaults 填充零值字段的默认值。
func (c *Config) setDefaults() {
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
		c.UserAgent = "YukiChoco/" + Version + " (Milky " + types.ProtocolVersion + ")"
	}
	if c.RateLimit.QPS > 0 && c.RateLimit.Burst <= 0 {
		burst := int(c.RateLimit.QPS)
		if burst < 1 {
			burst = 1
		}
		c.RateLimit.Burst = burst
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}
}

// validate 校验配置并规范化 BaseURL。
func (c *Config) validate() error {
	raw := strings.TrimSpace(c.BaseURL)
	if raw == "" {
		return errors.New("milky: Config.BaseURL 不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("milky: Config.BaseURL 不是合法 URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("milky: Config.BaseURL 的 scheme 必须是 http 或 https，实际为 %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("milky: Config.BaseURL 缺少主机名")
	}
	c.BaseURL = strings.TrimRight(raw, "/")
	return nil
}

// String 返回脱敏后的配置摘要，可安全写日志。
func (c Config) String() string {
	return fmt.Sprintf("Config{BaseURL: %s, AccessToken: %s, APITimeout: %s, Retry.MaxAttempts: %d, RateLimit.QPS: %g}",
		c.BaseURL, redactSecret(c.AccessToken), c.APITimeout, c.Retry.MaxAttempts, c.RateLimit.QPS)
}

// LogValue 让 slog 输出脱敏后的配置。
func (c Config) LogValue() slog.Value {
	return slog.StringValue(c.String())
}

// redactSecret 把非空密钥替换为固定占位符，避免长度泄露。
func redactSecret(s string) string {
	if s == "" {
		return "(未设置)"
	}
	return "***"
}
