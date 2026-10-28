package event

import (
	"context"
	"sync"

	"github.com/WinterChocolates/YukiChoco/types"
)

// On 订阅某一具体事件类型，返回取消订阅函数。
//
//	off := event.On(dispatcher, func(ctx context.Context, ev *types.MessageReceiveEvent) error { ... })
//
// 类型参数 T 取事件的指针类型（生成的事件类型都以指针实现 types.Event），
// 事件类型由 T 的零值推导，因此不需要手写 event_type 字符串。
func On[T types.Event](d *Dispatcher, h func(context.Context, T) error) func() {
	var zero T
	return d.SubscribeType(zero.EventType(), func(ctx context.Context, ev types.Event) error {
		typed, ok := ev.(T)
		if !ok {
			// 理论上不会发生：event_type 与具体类型一一对应。
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
	// 订阅注册前如果已经触发过（并发分发场景），立即注销，避免残留空订阅。
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
