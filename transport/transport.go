// Package transport 是 Milky 的事件传输层。
//
// 它只负责「建立一次连接 + 按条读出事件」，不包含重连与分发策略：
// 断线后是否重连、以什么节奏重连，由上层（Client 的连接状态机）决定。
// 这样 SSE、WebHook 等其它事件源可以复用同一套重连与分发逻辑。
package transport

import (
	"context"
	"errors"
	"fmt"
)

// 事件源名称。
const (
	// SourceWS 是 WebSocket 事件源（GET /event 升级）。
	SourceWS = "ws"
	// SourceSSE 是 Server-Sent Events 事件源（M3 实现）。
	SourceSSE = "sse"
	// SourceWebHook 是 WebHook 事件源（M3 实现）。
	SourceWebHook = "webhook"
)

// 传输层错误。
var (
	// ErrClosed 表示会话已被本地关闭。
	ErrClosed = errors.New("milky/transport: 会话已关闭")

	// ErrUnauthorized 表示事件通道鉴权失败（HTTP 401）。
	ErrUnauthorized = errors.New("milky/transport: 事件通道鉴权失败")
)

// Session 是一条已建立的事件连接。
//
// 实现需保证：Close 可重复调用；Next 在 ctx 结束或 Close 之后尽快返回错误。
type Session interface {
	// Next 阻塞返回下一条事件的原始 JSON。
	Next(ctx context.Context) ([]byte, error)
	// Close 关闭连接并释放资源。
	Close() error
}

// Source 是可建立事件连接的事件源。
type Source interface {
	// Name 返回事件源名称，例如 transport.SourceWS。
	Name() string
	// Connect 建立一次连接。失败时返回错误，由上层决定是否重连。
	Connect(ctx context.Context) (Session, error)
}

// StatusError 表示握手阶段的非预期 HTTP 状态码。
type StatusError struct {
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("milky/transport: 握手失败: HTTP %s", e.Status)
}
