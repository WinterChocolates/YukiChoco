// Package event 负责 OneBot 事件的订阅与分发。
//
// 分发策略有三档：并发、全局有序、按会话有序；队列满时可丢弃或阻塞。
// 无论用户是否安装中间件，panic 都会被捕获，单个 handler 不会拖垮事件循环。
package event

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/WinterChocolates/YukiChoco/types"
)

// Handler 处理一条事件。返回的 error 只会被记录，不会中断分发。
type Handler func(ctx context.Context, ev types.Event) error

// Middleware 包装 Handler，用于横切关注点（日志、限流、鉴权等）。
type Middleware func(next Handler) Handler

// Mode 是事件分发顺序策略。
type Mode string

const (
	// ModeConcurrent 多 worker 并发处理，不保证顺序（默认）。
	ModeConcurrent Mode = "concurrent"
	// ModeSequential 单 worker 串行处理，全局有序。
	ModeSequential Mode = "sequential"
	// ModePerPeerSequential 按会话分片：同一会话内有序，不同会话并行。
	ModePerPeerSequential Mode = "per-peer"
)

// Backpressure 是队列满时的策略。
type Backpressure string

const (
	// BackpressureDropNewest 丢弃新事件并计数（默认）。
	BackpressureDropNewest Backpressure = "drop-newest"
	// BackpressureBlock 阻塞投递方。
	//
	// 事件源会停止读取 socket，长时间阻塞可能被协议端判定为超时断开。
	BackpressureBlock Backpressure = "block"
)

// Config 是 Dispatcher 的配置。
type Config struct {
	// Workers 是并发度 / 分片数。零值使用 runtime.NumCPU()。
	Workers int
	// Buffer 是队列容量。零值使用 1024。
	Buffer int
	// Mode 是顺序策略。零值等价于 ModeConcurrent。
	Mode Mode
	// Backpressure 是队列满时的策略。零值等价于 BackpressureDropNewest。
	Backpressure Backpressure
	// HandlerTimeout 是单个事件的处理超时。零值不限。
	HandlerTimeout time.Duration
	// KeyFunc 用于 ModePerPeerSequential 的会话划分。
	KeyFunc func(types.Event) string
	// Logger 用于记录 handler 错误、panic 与丢弃。零值丢弃日志。
	Logger *slog.Logger
}

func (c *Config) setDefaults() {
	if c.Workers <= 0 {
		c.Workers = runtime.NumCPU()
	}
	if c.Buffer <= 0 {
		c.Buffer = 1024
	}
	if c.Mode == "" {
		c.Mode = ModeConcurrent
	}
	if c.Backpressure == "" {
		c.Backpressure = BackpressureDropNewest
	}
	if c.KeyFunc == nil {
		c.KeyFunc = DefaultKeyFunc
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// DefaultKeyFunc 按会话划分事件：
// 群消息按群号，私聊按对方 QQ 号，其它按事件类型 + self_id。
func DefaultKeyFunc(ev types.Event) string {
	switch e := ev.(type) {
	case *types.GroupMessageEvent:
		return "group:" + e.GroupID.String()
	case *types.PrivateMessageEvent:
		return "private:" + e.UserID.String()
	default:
		return "event:" + string(ev.EventType()) + ":" + ev.EventSelfID().String()
	}
}

type subscription struct {
	id      uint64
	handler Handler
}

// Dispatcher 负责把事件投递到队列，并按策略调用订阅者。
type Dispatcher struct {
	cfg     Config
	logger  *slog.Logger
	queues  []chan types.Event
	workers int
	wg      sync.WaitGroup

	handled  atomic.Uint64
	failed   atomic.Uint64
	dropped  atomic.Uint64
	panicked atomic.Uint64

	senders sync.RWMutex

	mu         sync.RWMutex
	closed     bool
	all        []*subscription
	byType     map[types.EventType][]*subscription
	middleware []Middleware
	chain      Handler

	nextID    atomic.Uint64
	closeOnce sync.Once
}

// New 创建并启动分发器。
func New(cfg Config) *Dispatcher {
	cfg.setDefaults()
	d := &Dispatcher{
		cfg:    cfg,
		logger: cfg.Logger,
		byType: make(map[types.EventType][]*subscription),
	}
	d.rebuildChain()

	switch cfg.Mode {
	case ModeSequential:
		d.workers = 1
		q := make(chan types.Event, cfg.Buffer)
		d.queues = []chan types.Event{q}
		d.startWorkers(q, 1)
	case ModePerPeerSequential:
		d.workers = cfg.Workers
		d.queues = make([]chan types.Event, cfg.Workers)
		for i := range d.queues {
			q := make(chan types.Event, cfg.Buffer)
			d.queues[i] = q
			d.startWorkers(q, 1)
		}
	default:
		d.workers = cfg.Workers
		q := make(chan types.Event, cfg.Buffer)
		d.queues = []chan types.Event{q}
		d.startWorkers(q, cfg.Workers)
	}
	return d
}

func (d *Dispatcher) startWorkers(q chan types.Event, n int) {
	for i := 0; i < n; i++ {
		d.wg.Add(1)
		go d.worker(q)
	}
}

// Use 追加中间件。可在运行期调用；已注册的中间件从下一个事件起生效。
func (d *Dispatcher) Use(mw ...Middleware) {
	if len(mw) == 0 {
		return
	}
	d.mu.Lock()
	d.middleware = append(d.middleware, mw...)
	d.rebuildChainLocked()
	d.mu.Unlock()
}

func (d *Dispatcher) rebuildChainLocked() {
	chain := Handler(func(ctx context.Context, ev types.Event) error {
		return d.invokeSubscribers(ctx, ev)
	})
	for i := len(d.middleware) - 1; i >= 0; i-- {
		chain = d.middleware[i](chain)
	}
	d.chain = Recover(d.logger, &d.panicked)(chain)
}

func (d *Dispatcher) rebuildChain() {
	d.mu.Lock()
	d.rebuildChainLocked()
	d.mu.Unlock()
}

// SubscribeAll 订阅所有事件，返回取消订阅函数。
func (d *Dispatcher) SubscribeAll(h Handler) func() { return d.subscribe("", h) }

// SubscribeType 订阅指定事件类型，返回取消订阅函数。
func (d *Dispatcher) SubscribeType(t types.EventType, h Handler) func() {
	return d.subscribe(t, h)
}

func (d *Dispatcher) subscribe(t types.EventType, h Handler) func() {
	sub := &subscription{id: d.nextID.Add(1), handler: h}
	d.mu.Lock()
	if t == "" {
		d.all = append(d.all, sub)
	} else {
		d.byType[t] = append(d.byType[t], sub)
	}
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if t == "" {
			d.all = removeSubscription(d.all, sub.id)
			return
		}
		d.byType[t] = removeSubscription(d.byType[t], sub.id)
	}
}

func removeSubscription(list []*subscription, id uint64) []*subscription {
	out := list[:0]
	for _, s := range list {
		if s.id != id {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Dispatch 把事件投入队列。已关闭或队列满（丢弃策略）时立即返回。
func (d *Dispatcher) Dispatch(ctx context.Context, ev types.Event) {
	if ev == nil {
		return
	}
	d.senders.RLock()
	defer d.senders.RUnlock()

	d.mu.RLock()
	closed := d.closed
	d.mu.RUnlock()
	if closed {
		return
	}

	q := d.queueFor(ev)
	if d.cfg.Backpressure == BackpressureBlock {
		select {
		case q <- ev:
		case <-ctx.Done():
		}
		return
	}
	select {
	case q <- ev:
	default:
		d.dropped.Add(1)
		d.logger.Warn("yukichoco: 事件队列已满，丢弃事件",
			"event_type", ev.EventType(), "mode", string(d.cfg.Mode), "buffer", d.cfg.Buffer)
	}
}

func (d *Dispatcher) queueFor(ev types.Event) chan types.Event {
	if d.cfg.Mode != ModePerPeerSequential || len(d.queues) == 1 {
		return d.queues[0]
	}
	key := d.cfg.KeyFunc(ev)
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return d.queues[int(h.Sum32())%len(d.queues)]
}

func (d *Dispatcher) worker(q <-chan types.Event) {
	defer d.wg.Done()
	for ev := range q {
		d.handle(ev)
	}
}

func (d *Dispatcher) handle(ev types.Event) {
	ctx := context.Background()
	if d.cfg.HandlerTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.cfg.HandlerTimeout)
		defer cancel()
	}

	d.mu.RLock()
	chain := d.chain
	d.mu.RUnlock()

	if err := chain(ctx, ev); err != nil {
		d.failed.Add(1)
		d.logger.Error("yukichoco: 事件处理失败", "event_type", ev.EventType(), "err", err)
		return
	}
	d.handled.Add(1)
}

func (d *Dispatcher) invokeSubscribers(ctx context.Context, ev types.Event) error {
	d.mu.RLock()
	all := append([]*subscription(nil), d.all...)
	typed := append([]*subscription(nil), d.byType[ev.EventType()]...)
	d.mu.RUnlock()

	var errs []error
	run := func(list []*subscription) {
		for _, s := range list {
			func() {
				defer func() {
					if r := recover(); r != nil {
						d.panicked.Add(1)
						d.logger.Error("yukichoco: 事件订阅者 panic",
							"event_type", ev.EventType(),
							"panic", fmt.Sprint(r),
							"stack", string(debug.Stack()),
						)
						errs = append(errs, fmt.Errorf("yukichoco: 事件订阅者 panic: %v", r))
					}
				}()
				if err := s.handler(ctx, ev); err != nil {
					errs = append(errs, err)
				}
			}()
		}
	}
	run(all)
	run(typed)
	return errors.Join(errs...)
}

// Close 停止接收新事件，等待队列排空与 handler 结束。
//
// 超时由 ctx 控制；超时返回 ctx.Err()。
func (d *Dispatcher) Close(ctx context.Context) error {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		d.mu.Unlock()

		d.senders.Lock()
		for _, q := range d.queues {
			close(q)
		}
		d.senders.Unlock()
	})

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats 是分发器的运行统计。
type Stats struct {
	Handled  uint64
	Failed   uint64
	Dropped  uint64
	Panicked uint64
	Queued   int
	Workers  int
	Mode     Mode
}

// Stats 返回当前统计快照。
func (d *Dispatcher) Stats() Stats {
	queued := 0
	for _, q := range d.queues {
		queued += len(q)
	}
	return Stats{
		Handled:  d.handled.Load(),
		Failed:   d.failed.Load(),
		Dropped:  d.dropped.Load(),
		Panicked: d.panicked.Load(),
		Queued:   queued,
		Workers:  d.workers,
		Mode:     d.cfg.Mode,
	}
}
