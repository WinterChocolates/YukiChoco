package YukiChoco

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/transport"
	"github.com/WinterChocolates/YukiChoco/types"
)

// EventSourceKind 是事件通道类型。
type EventSourceKind string

const (
	// EventSourceNone 表示不接收事件，只做 API 调用。
	EventSourceNone EventSourceKind = "none"
	// EventSourceWS 表示通过 WebSocket（GET /event）接收事件。
	EventSourceWS EventSourceKind = "ws"
)

// stateHook 与 rawHook 是回调注册记录。
type stateHook struct {
	id uint64
	fn func(State)
}

type rawHook struct {
	id uint64
	fn func(context.Context, []byte) error
}

// Events 返回底层事件分发器，用于高级用法（自定义中间件、统计等）。
func (c *Client) Events() *event.Dispatcher { return c.events }

// Use 追加事件中间件，例如 event.Timeout(3*time.Second)。
func (c *Client) Use(mw ...event.Middleware) { c.events.Use(mw...) }

// OnEvent 订阅所有事件（已解析为 types.Event）。
func (c *Client) OnEvent(h func(context.Context, types.Event) error) func() {
	return c.events.SubscribeAll(h)
}

// On 订阅某一具体事件类型。
//
//	YukiChoco.On(c, func(ctx context.Context, ev *types.MessageReceiveEvent) error { ... })
func On[T types.Event](c *Client, h func(context.Context, T) error) func() {
	return event.On(c.events, h)
}

// Once 与 On 相同，但只触发一次。
func Once[T types.Event](c *Client, h func(context.Context, T) error) func() {
	return event.Once(c.events, h)
}

// OnRaw 注册原始事件 JSON 钩子。
//
// 钩子在事件解码之前、事件循环内同步调用，必须尽快返回（适合打日志/落盘），
// 耗时逻辑请放进 OnEvent。返回的钩子不会再收到任何数据。
func (c *Client) OnRaw(h func(ctx context.Context, raw []byte) error) func() {
	c.mu.Lock()
	id := c.nextRawID.Add(1)
	c.rawHooks = append(c.rawHooks, rawHook{id: id, fn: h})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		out := c.rawHooks[:0]
		for _, hook := range c.rawHooks {
			if hook.id != id {
				out = append(out, hook)
			}
		}
		c.rawHooks = out
	}
}

// State 返回当前事件连接状态。
func (c *Client) State() State { return State(c.state.Load()) }

// Connected 报告事件通道是否已连接。
func (c *Client) Connected() bool { return c.State() == StateConnected }

// OnStateChange 注册状态变化回调，返回注销函数。
//
// 回调由单一 goroutine 按状态迁移顺序串行执行（不会阻塞事件循环），
// 因此回调顺序与 State() 的取值变化一致，可以安全地在回调里调用 State()。
func (c *Client) OnStateChange(h func(State)) func() {
	c.mu.Lock()
	id := c.nextStateID.Add(1)
	c.stateHooks = append(c.stateHooks, stateHook{id: id, fn: h})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		out := c.stateHooks[:0]
		for _, hook := range c.stateHooks {
			if hook.id != id {
				out = append(out, hook)
			}
		}
		c.stateHooks = out
	}
}

// WaitReady 等待事件通道首次就绪（或 ctx 结束、客户端关闭）。
func (c *Client) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-c.closedCh:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start 启动事件循环；配合 Shutdown 使用。
//
// Start 只在启动前检查 ctx 是否已取消，事件循环的生命周期由 Shutdown 控制，
// 因此不要用带短超时的 ctx 调用 Start。Start 重复调用返回 ErrAlreadyStarted。
func (c *Client) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	if c.started {
		c.mu.Unlock()
		return ErrAlreadyStarted
	}
	c.started = true
	src := c.eventSource
	done := make(chan struct{})
	c.loopDone = done
	c.mu.Unlock()

	if src == nil {
		// 纯 API 模式：没有事件通道，但客户端立即可用。
		c.setState(StateConnected)
		c.markReady()
		close(done)
		return nil
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.loopCancel = cancel
	c.mu.Unlock()

	go func() {
		defer close(done)
		c.runEventLoop(loopCtx, src)
	}()
	return nil
}

// Shutdown 停止事件循环、排空事件队列并关闭客户端。
//
// 超时由 ctx 控制；返回 ctx.Err() 表示等待超时（可能有 handler 仍在执行）。
// 可重复调用。
func (c *Client) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	first := !c.shutdown
	c.shutdown = true
	cancel := c.loopCancel
	done := c.loopDone
	c.mu.Unlock()

	// 重复调用时仍会继续等待事件队列排空（上一次可能因 ctx 超时提前返回）。
	if first && cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := c.events.Close(ctx); err != nil {
		return err
	}
	c.setState(StateClosed)
	return c.Close()
}

// runEventLoop 是连接状态机：拨号 → 消费 → 断开 → 退避重连。
func (c *Client) runEventLoop(ctx context.Context, src transport.Source) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if attempt == 0 {
			c.setState(StateDialing)
		}

		sess, err := src.Connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			attempt++
			c.logger.Warn("milky: 事件通道连接失败",
				"source", src.Name(), "attempt", attempt, "err", err)
			if !c.canRetry(attempt) {
				c.logger.Error("milky: 已达最大重连次数，停止事件循环", "attempts", attempt)
				c.setState(StateDisconnected)
				return
			}
			c.setState(StateReconnecting)
			if err := sleepContext(ctx, c.reconnectDelay(attempt)); err != nil {
				return
			}
			continue
		}

		attempt = 0
		c.setState(StateConnected)
		c.markReady()
		c.logger.Info("milky: 事件通道已连接", "source", src.Name())

		err = c.consumeEvents(ctx, sess)
		_ = sess.Close()
		if ctx.Err() != nil {
			return
		}

		c.logger.Warn("milky: 事件通道已断开", "source", src.Name(), "err", err)
		attempt++
		if !c.canRetry(attempt) {
			c.logger.Error("milky: 已达最大重连次数，停止事件循环", "attempts", attempt)
			c.setState(StateDisconnected)
			return
		}
		c.setState(StateReconnecting)
		if err := sleepContext(ctx, c.reconnectDelay(attempt)); err != nil {
			return
		}
	}
}

// consumeEvents 读取并派发事件，直到连接出错或 ctx 结束。
func (c *Client) consumeEvents(ctx context.Context, sess transport.Session) error {
	for {
		raw, err := sess.Next(ctx)
		if err != nil {
			return err
		}
		c.deliverRaw(ctx, raw)

		ev, err := types.UnmarshalEvent(raw)
		if err != nil {
			// 单条事件解析失败不应中断连接（兼容性契约 C4）。
			c.logger.Warn("milky: 无法解析事件，已跳过", "err", err)
			continue
		}
		c.events.Dispatch(ctx, ev)
	}
}

// deliverRaw 同步调用原始 JSON 钩子，并隔离其 panic。
func (c *Client) deliverRaw(ctx context.Context, raw []byte) {
	c.mu.RLock()
	hooks := append([]rawHook(nil), c.rawHooks...)
	c.mu.RUnlock()
	for _, hook := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("milky: 原始事件钩子 panic", "panic", fmt.Sprint(r))
				}
			}()
			if err := hook.fn(ctx, raw); err != nil {
				c.logger.Warn("milky: 原始事件钩子返回错误", "err", err)
			}
		}()
	}
}

// canRetry 判断是否允许继续重连。
func (c *Client) canRetry(attempt int) bool {
	cfg := c.cfg.Reconnect
	if cfg.Disable {
		return false
	}
	return cfg.MaxAttempts <= 0 || attempt <= cfg.MaxAttempts
}

// reconnectDelay 计算第 attempt 次重连前的等待时间（指数退避 + 抖动）。
func (c *Client) reconnectDelay(attempt int) time.Duration {
	cfg := c.cfg.Reconnect
	delay := cfg.InitialDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * cfg.Multiplier)
		if delay >= cfg.MaxDelay {
			delay = cfg.MaxDelay
			break
		}
	}
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	if cfg.Jitter > 0 {
		// [1-jitter, 1+jitter] 区间内的均匀抖动
		delta := cfg.Jitter * (rand.Float64()*2 - 1)
		delay = time.Duration(float64(delay) * (1 + delta))
	}
	if delay < 0 {
		delay = 0
	}
	return delay
}

// setState 更新状态并把通知排入队列。
//
// 通知由一个独立的 goroutine 按顺序消费，既不阻塞事件循环，也保证回调顺序与
// 状态迁移一致。队列满时丢弃通知（状态本身可用 State() 查询）。
func (c *Client) setState(s State) {
	if prev := State(c.state.Swap(int32(s))); prev == s {
		return
	}
	select {
	case c.stateCh <- s:
	default:
	}
}

// notifyStates 是状态通知的单一消费者。
func (c *Client) notifyStates() {
	defer close(c.stateDone)
	for {
		select {
		case <-c.closedCh:
			// 关闭前把已排队的状态通知发完。
			for {
				select {
				case s := <-c.stateCh:
					c.fireState(s)
				default:
					return
				}
			}
		case s := <-c.stateCh:
			c.fireState(s)
		}
	}
}

// fireState 依次调用状态回调，并隔离 panic。
func (c *Client) fireState(s State) {
	c.mu.RLock()
	hooks := append([]stateHook(nil), c.stateHooks...)
	c.mu.RUnlock()
	for _, hook := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("milky: 状态回调 panic", "panic", fmt.Sprint(r))
				}
			}()
			hook.fn(s)
		}()
	}
}

// markReady 关闭 ready 通道（仅一次）。
func (c *Client) markReady() { c.readyOnce.Do(func() { close(c.ready) }) }
