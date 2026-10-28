package YukiChoco

import (
	"errors"
	"fmt"
)

// 哨兵错误，可用 errors.Is 判别。
var (
	// ErrClosed 表示客户端已关闭，不再接受调用。
	ErrClosed = errors.New("milky: 客户端已关闭")

	// ErrUnauthorized 对应 HTTP 401：access_token 缺失或不匹配。
	ErrUnauthorized = errors.New("milky: 鉴权失败（access_token 缺失或不匹配）")

	// ErrEndpointNotFound 对应 HTTP 404：协议端不存在该 API。
	ErrEndpointNotFound = errors.New("milky: 协议端不存在该 API")

	// ErrUnsupportedMediaType 对应 HTTP 415：协议端不接受该 Content-Type。
	ErrUnsupportedMediaType = errors.New("milky: 协议端不接受该 Content-Type")

	// ErrUnexpectedStatus 表示协议端返回了非预期的 HTTP 状态码（如 5xx）。
	ErrUnexpectedStatus = errors.New("milky: 协议端返回了非预期的 HTTP 状态码")

	// ErrInvalidResponse 表示响应体不是合法的 Milky 信封。
	ErrInvalidResponse = errors.New("milky: 协议端响应不是合法的 Milky 信封")

	// ErrBadRequest 对应 retcode -400：请求参数被协议端拒绝。
	ErrBadRequest = errors.New("milky: 请求参数被协议端拒绝")

	// ErrNotLoggedIn 对应 retcode -403：协议端未处于登录状态。
	ErrNotLoggedIn = errors.New("milky: 协议端未处于登录状态")

	// ErrNotFound 对应 retcode -404：资源不存在（好友、群、消息等）。
	ErrNotFound = errors.New("milky: 资源不存在")

	// ErrProtocolFailure 表示 status=failed 但 retcode 未在规范中定义。
	ErrProtocolFailure = errors.New("milky: 协议端返回失败")
)

// 协议规范中定义的 retcode。
const (
	// RetcodeOK 表示成功。
	RetcodeOK = 0
	// RetcodeBadRequest 表示请求参数解析或校验失败。
	RetcodeBadRequest = -400
	// RetcodeNotLoggedIn 表示协议端未登录。
	RetcodeNotLoggedIn = -403
	// RetcodeNotFound 表示资源不存在。
	RetcodeNotFound = -404
)

// APIError 是协议层失败：HTTP 200 但信封中 status=failed 或 retcode≠0。
//
// 不要依赖 Message 的文案做判断（协议端可自由措辞），请用 Retcode 或
// errors.Is(err, ErrNotLoggedIn) 之类的哨兵错误。
type APIError struct {
	// Endpoint 是被调用的端点名，例如 "send_group_message"。
	Endpoint string
	// Retcode 是协议端返回的错误码，0 表示成功（此时不会产生 APIError）。
	Retcode int
	// Message 是协议端返回的错误描述。
	Message string
	// Snippet 是响应体的脱敏截断，仅用于排障；不会包含 access_token。
	Snippet string
}

// Error 实现 error 接口。输出中不含请求头与 token。
func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "协议端未提供错误信息"
	}
	s := fmt.Sprintf("milky: %s 调用失败: retcode=%d: %s", e.Endpoint, e.Retcode, msg)
	if e.Snippet != "" && e.Message == "" {
		s += "（响应片段: " + e.Snippet + "）"
	}
	return s
}

// Unwrap 让 errors.Is 能按 retcode 归类到哨兵错误。
func (e *APIError) Unwrap() error {
	switch e.Retcode {
	case RetcodeBadRequest:
		return ErrBadRequest
	case RetcodeNotLoggedIn:
		return ErrNotLoggedIn
	case RetcodeNotFound:
		return ErrNotFound
	case RetcodeOK:
		return ErrProtocolFailure
	default:
		return nil
	}
}

// IsRetryable 报告该错误是否值得重试。
//
// 协议层的失败都是确定性的（参数错误、未登录、资源不存在），重试没有意义；
// 网络错误与 5xx 才由调用方按幂等性决定是否重试（见 apicall.go）。
func (e *APIError) IsRetryable() bool { return false }
