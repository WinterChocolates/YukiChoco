// Package transport 提供 OneBot v11 的 WebSocket 传输能力。
//
// 它只负责「一条 JSON 连接的读写」与「反向 WebSocket 接入」，不包含事件路由、
// echo 关联或重连策略，这些由上层 Client 负责。
package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// 传输层错误。
var (
	// ErrClosed 表示连接已被本地关闭。
	ErrClosed = errors.New("transport: 连接已关闭")

	// ErrUnauthorized 表示握手鉴权失败（HTTP 401）。
	ErrUnauthorized = errors.New("transport: 鉴权失败")

	// ErrNotConnected 表示当前没有可用的连接。
	ErrNotConnected = errors.New("transport: 没有可用连接")
)

// StatusError 表示握手阶段的非预期 HTTP 状态码。
type StatusError struct {
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("transport: 握手失败: HTTP %s", e.Status)
}

// IsUnauthorized 判断错误是否由鉴权失败导致。
func IsUnauthorized(err error) bool {
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.StatusCode == http.StatusUnauthorized
	}
	return false
}

// contextError 优先返回 ctx 的错误，避免把关闭包装成网络错误。
func contextError(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	return err
}
