package event

import (
	"context"
	"sync"

	"github.com/WinterChocolates/YukiChoco/types"
)

// On 订阅某一具体事件类型，返回取消订阅函数。
//
//	off := event.On(dispatcher, func(ctx context.Context, ev *types.GroupMessageEvent) error { ... })
//
// 类型参数 T 取事件的指针类型，事件类型由 T 的零值推导。
func On[T types.Event](d *Dispatcher, h func(context.Context, T) error) func() {
	var zero T
	return d.SubscribeType(zero.EventType(), func(ctx context.Context, ev types.Event) error {
		typed, ok := ev.(T)
		if !ok {
			return nil
		}
		return h(ctx, typed)
	})
}

// Once 与 On 相同，但只触发一次（触发后自动取消订阅）。
func Once[T types.Event](d *Dispatcher, h func(context.Context, T) error) func() {
	var (
		mu     sync.Mutex
		cancel func()
		fired  bool
	)

	off := On(d, func(ctx context.Context, ev T) error {
		mu.Lock()
		if fired {
			mu.Unlock()
			return nil
		}
		fired = true
		c := cancel
		mu.Unlock()

		if c != nil {
			defer c()
		}
		return h(ctx, ev)
	})

	mu.Lock()
	cancel = off
	alreadyFired := fired
	mu.Unlock()
	if alreadyFired {
		off()
	}

	return func() {
		mu.Lock()
		c := cancel
		mu.Unlock()
		if c != nil {
			c()
		}
	}
}
