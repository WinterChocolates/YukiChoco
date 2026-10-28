package YukiChoco

import (
	"errors"
	"fmt"
	"strings"
)

// 哨兵错误，可用 errors.Is 判别。
var (
	// ErrClosed 表示客户端已关闭，不再接受调用。
	ErrClosed = errors.New("yukichoco: 客户端已关闭")

	// ErrAlreadyStarted 表示 Start 被重复调用。
	ErrAlreadyStarted = errors.New("yukichoco: 客户端已启动")

	// ErrNotConnected 表示当前没有可用的 API 通道或事件连接。
	ErrNotConnected = errors.New("yukichoco: 没有可用的连接（尚未 Start 或协议端未连接）")

	// ErrUnauthorized 表示鉴权失败（HTTP 401 / retcode 104）。
	ErrUnauthorized = errors.New("yukichoco: 鉴权失败（access_token 不正确）")

	// ErrActionNotFound 表示协议端不支持该 action（HTTP 404）。
	ErrActionNotFound = errors.New("yukichoco: 协议端不支持该 action")

	// ErrUnexpectedStatus 表示协议端返回了非预期的 HTTP 状态码（如 5xx）。
	ErrUnexpectedStatus = errors.New("yukichoco: 协议端返回了非预期的 HTTP 状态码")

	// ErrInvalidResponse 表示响应体不是合法的 OneBot 响应。
	ErrInvalidResponse = errors.New("yukichoco: 协议端响应格式非法")

	// ErrBadRequest 对应 retcode 100/102：请求参数被拒绝。
	ErrBadRequest = errors.New("yukichoco: 请求参数被协议端拒绝")

	// ErrNotFound 对应 retcode 103：资源不存在。
	ErrNotFound = errors.New("yukichoco: 资源不存在")

	// ErrForbidden 对应 retcode 200 段：权限不足。
	ErrForbidden = errors.New("yukichoco: 权限不足或操作被拒绝")

	// ErrActionFailed 是未归类的 status=failed / async 失败。
	ErrActionFailed = errors.New("yukichoco: 协议端返回失败")

	// ErrNotSupported 表示当前配置不支持该能力。
	ErrNotSupported = errors.New("yukichoco: 当前配置不支持该能力")
)

// OneBot v11 常用 retcode。
const (
	// RetcodeOK 表示成功。
	RetcodeOK = 0
	// RetcodeAsync 表示请求已受理（异步执行）。
	RetcodeAsync = 1
	// RetcodeBadRequest 表示请求格式错误。
	RetcodeBadRequest = 100
	// RetcodeBadParam 表示参数错误。
	RetcodeBadParam = 102
	// RetcodeNotFound 表示资源不存在。
	RetcodeNotFound = 103
	// RetcodeCredentialsInvalid 表示凭证失效。
	RetcodeCredentialsInvalid = 104
	// RetcodeForbidden 表示权限不足。
	RetcodeForbidden = 200
	// RetcodeAsyncFailed 表示异步任务失败。
	RetcodeAsyncFailed = 201
)

// APIError 是协议层失败：HTTP 200 但 status 非 ok，或 retcode 非 0/1。
//
// 不要依赖 Message 文案做判断，请用 Retcode 或 errors.Is。
type APIError struct {
	// Action 是被调用的动作名，例如 "send_group_msg"。
	Action string
	// Status 是响应中的 status（ok / failed / async）。
	Status string
	// Retcode 是协议端错误码；0 与 1 不会产生 APIError。
	Retcode int
	// Message 是协议端返回的错误描述。
	Message string
	// Wording 是部分实现提供的补充描述。
	Wording string
	// Snippet 是响应体的脱敏截断，仅用于排障。
	Snippet string
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Wording
	}
	if msg == "" {
		msg = "协议端未提供错误信息"
	}
	s := fmt.Sprintf("yukichoco: %s 调用失败: status=%s retcode=%d: %s", e.Action, e.Status, e.Retcode, msg)
	if e.Snippet != "" && e.Message == "" && e.Wording == "" {
		s += "（响应片段: " + e.Snippet + "）"
	}
	return s
}

// Unwrap 让 errors.Is 能按 retcode 归类。
func (e *APIError) Unwrap() error {
	switch e.Retcode {
	case RetcodeBadRequest, RetcodeBadParam:
		return ErrBadRequest
	case RetcodeNotFound:
		return ErrNotFound
	case RetcodeCredentialsInvalid:
		return ErrUnauthorized
	case RetcodeForbidden:
		return ErrForbidden
	default:
		return ErrActionFailed
	}
}

// IsRetryable 报告该错误是否值得重试。协议层失败是确定性的，不重试。
func (e *APIError) IsRetryable() bool { return false }

// snippet 截断响应体，避免日志被大响应淹没。
func snippet(b []byte) string {
	const max = 512
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
