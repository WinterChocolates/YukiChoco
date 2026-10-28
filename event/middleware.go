package event

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/WinterChocolates/YukiChoco/types"
)

// Recover 捕获处理链中的 panic，记录日志并计入统计。
//
// Dispatcher 总会把它装在链的最外层，因此用户中间件里的 panic 同样不会
// 影响事件循环；这里导出是为了在自定义链（见 Chain）中复用。
func Recover(logger *slog.Logger, counter *atomic.Uint64) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, ev types.Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					if counter != nil {
						counter.Add(1)
					}
					logger.Error("milky: 事件处理器 panic",
						"event_type", ev.EventType(),
						"panic", fmt.Sprint(r),
						"stack", string(debug.Stack()),
					)
					err = fmt.Errorf("milky: 事件处理器 panic: %v", r)
				}
			}()
			return next(ctx, ev)
		}
	}
}

// Timeout 为单条事件的处理设置超时。
//
// handler 必须自行响应 ctx（例如把 ctx 传给 API 调用），否则超时只体现为
// ctx 被取消而不会强制中断 handler。
func Timeout(d time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, ev types.Event) error {
			if d <= 0 {
				return next(ctx, ev)
			}
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, ev)
		}
	}
}

// Logging 记录每条事件的处理耗时与结果。
func Logging(logger *slog.Logger, level slog.Level) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, ev types.Event) error {
			started := time.Now()
			err := next(ctx, ev)
			attrs := []any{
				"event_type", ev.EventType(),
				"elapsed_ms", time.Since(started).Milliseconds(),
			}
			if err != nil {
				attrs = append(attrs, "err", err)
			}
			logger.Log(ctx, level, "milky: 处理事件", attrs...)
			return err
		}
	}
}
