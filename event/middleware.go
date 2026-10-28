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
func Recover(logger *slog.Logger, counter *atomic.Uint64) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, ev types.Event) (err error) {
			defer func() {
				if r := recover(); r != nil {
					if counter != nil {
						counter.Add(1)
					}
					logger.Error("yukichoco: 事件处理器 panic",
						"event_type", ev.EventType(),
						"panic", fmt.Sprint(r),
						"stack", string(debug.Stack()),
					)
					err = fmt.Errorf("yukichoco: 事件处理器 panic: %v", r)
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
			logger.Log(ctx, level, "yukichoco: 事件处理完成",
				"event_type", ev.EventType(),
				"self_id", ev.EventSelfID().Int64(),
				"elapsed_ms", time.Since(started).Milliseconds(),
				"err", err,
			)
			return err
		}
	}
}

// Filter 只把匹配的事件传给后续链（不匹配时直接返回 nil）。
func Filter(pred func(types.Event) bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, ev types.Event) error {
			if pred != nil && !pred(ev) {
				return nil
			}
			return next(ctx, ev)
		}
	}
}
