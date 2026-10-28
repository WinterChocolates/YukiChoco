package event_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

// msgEvent 构造一条群消息事件，seq 用于验证顺序。
func msgEvent(t *testing.T, groupID, seq int64) types.Event {
	t.Helper()
	raw := fmt.Sprintf(`{
		"time": 1, "self_id": 1, "event_type": "message_receive",
		"data": {
			"message_scene": "group", "peer_id": %d, "message_seq": %d, "sender_id": 2, "time": 1,
			"segments": [{"type":"text","data":{"text":"x"}}],
			"group": {"group_id": %d, "group_name": "g", "member_count": 1, "max_member_count": 1},
			"group_member": {"user_id": 2, "nickname": "n", "sex": "unknown", "group_id": %d,
				"card": "", "title": "", "level": 1, "role": "member", "join_time": 1, "last_sent_time": 1}
		}
	}`, groupID, seq, groupID, groupID)
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		t.Fatalf("构造事件失败: %v", err)
	}
	return ev
}

func recallEvent(t *testing.T, seq int64) types.Event {
	t.Helper()
	raw := fmt.Sprintf(`{"time":1,"self_id":1,"event_type":"message_recall","data":{
		"message_scene":"group","peer_id":9,"message_seq":%d,"sender_id":2,"operator_id":2,"display_suffix":""}}`, seq)
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		t.Fatalf("构造事件失败: %v", err)
	}
	return ev
}

// waitFor 轮询等待条件成立。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", msg)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
}

func TestSequentialModePreservesOrder(t *testing.T) {
	d := event.New(event.Config{
		Mode:   event.ModeSequential,
		Buffer: 64,
		Logger: discardLogger(),
	})
	var mu sync.Mutex
	var seqs []int64
	d.SubscribeType(types.EventMessageReceive, func(_ context.Context, ev types.Event) error {
		m, ok := ev.(*types.MessageReceiveEvent)
		if !ok {
			return nil
		}
		gm, ok := m.GroupMessage()
		if !ok {
			return nil
		}
		mu.Lock()
		seqs = append(seqs, gm.MessageSeq.Int64())
		mu.Unlock()
		return nil
	})

	const total = 100
	for i := 0; i < total; i++ {
		d.Dispatch(context.Background(), msgEvent(t, 1, int64(i+1)))
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seqs) != total {
		t.Fatalf("处理事件数 = %d, want %d", len(seqs), total)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("顺序被破坏: seqs[%d]=%d <= seqs[%d]=%d", i, seqs[i], i-1, seqs[i-1])
		}
	}
}

func TestPerPeerModeOrdersWithinPeerOnly(t *testing.T) {
	d := event.New(event.Config{
		Mode:    event.ModePerPeerSequential,
		Workers: 4,
		Buffer:  128,
		Logger:  discardLogger(),
	})

	var inFlight, maxInFlight int64
	perPeer := map[int64][]int64{}
	var mu sync.Mutex

	d.SubscribeType(types.EventMessageReceive, func(_ context.Context, ev types.Event) error {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			max := atomic.LoadInt64(&maxInFlight)
			if cur <= max || atomic.CompareAndSwapInt64(&maxInFlight, max, cur) {
				break
			}
		}
		defer atomic.AddInt64(&inFlight, -1)

		m := ev.(*types.MessageReceiveEvent)
		gm, _ := m.GroupMessage()
		time.Sleep(time.Millisecond)
		mu.Lock()
		perPeer[gm.Group.GroupID.Int64()] = append(perPeer[gm.Group.GroupID.Int64()], gm.MessageSeq.Int64())
		mu.Unlock()
		return nil
	})

	const groups = 4
	const perGroup = 25
	for i := 0; i < perGroup; i++ {
		for g := int64(1); g <= groups; g++ {
			d.Dispatch(context.Background(), msgEvent(t, g, int64(i+1)))
		}
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(perPeer) != groups {
		t.Fatalf("会话数 = %d, want %d", len(perPeer), groups)
	}
	for gid, seqs := range perPeer {
		if len(seqs) != perGroup {
			t.Fatalf("群 %d 处理数 = %d, want %d", gid, len(seqs), perGroup)
		}
		for i := 1; i < len(seqs); i++ {
			if seqs[i] <= seqs[i-1] {
				t.Fatalf("群 %d 顺序被破坏: %v", gid, seqs)
			}
		}
	}
	if atomic.LoadInt64(&maxInFlight) < 2 {
		t.Error("不同会话应当可以并行处理")
	}
}

func TestConcurrentModeUsesMultipleWorkers(t *testing.T) {
	d := event.New(event.Config{
		Mode:    event.ModeConcurrent,
		Workers: 4,
		Buffer:  128,
		Logger:  discardLogger(),
	})
	var inFlight, maxInFlight int64
	d.SubscribeAll(func(context.Context, types.Event) error {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			max := atomic.LoadInt64(&maxInFlight)
			if cur <= max || atomic.CompareAndSwapInt64(&maxInFlight, max, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return nil
	})
	for i := 0; i < 32; i++ {
		d.Dispatch(context.Background(), msgEvent(t, 1, int64(i+1)))
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt64(&maxInFlight); got < 2 {
		t.Fatalf("并发度 = %d, want >= 2", got)
	}
}

func TestDropNewestBackpressure(t *testing.T) {
	release := make(chan struct{})
	d := event.New(event.Config{
		Mode:         event.ModeSequential,
		Buffer:       1,
		Backpressure: event.BackpressureDropNewest,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error {
		<-release
		return nil
	})

	for i := 0; i < 20; i++ {
		d.Dispatch(context.Background(), msgEvent(t, 1, int64(i+1)))
	}
	stats := d.Stats()
	if stats.Dropped == 0 {
		t.Fatalf("队列满时应丢弃事件，stats = %+v", stats)
	}
	close(release)
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBlockBackpressure(t *testing.T) {
	release := make(chan struct{})
	d := event.New(event.Config{
		Mode:         event.ModeSequential,
		Buffer:       1,
		Backpressure: event.BackpressureBlock,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error {
		<-release
		return nil
	})

	// 队列满后阻塞投递，ctx 超时应让投递返回而不是丢弃。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	for i := 0; i < 5; i++ {
		d.Dispatch(ctx, msgEvent(t, 1, int64(i+1)))
	}
	if got := d.Stats().Dropped; got != 0 {
		t.Fatalf("Block 策略不应丢弃事件，dropped = %d", got)
	}
	close(release)
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestPanicIsIsolated(t *testing.T) {
	var healthy int64
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	d.SubscribeAll(func(context.Context, types.Event) error {
		panic("boom")
	})
	d.SubscribeAll(func(context.Context, types.Event) error {
		atomic.AddInt64(&healthy, 1)
		return nil
	})
	for i := 0; i < 3; i++ {
		d.Dispatch(context.Background(), recallEvent(t, int64(i+1)))
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	stats := d.Stats()
	if stats.Panicked != 3 {
		t.Errorf("Panicked = %d, want 3", stats.Panicked)
	}
	if got := atomic.LoadInt64(&healthy); got != 3 {
		t.Errorf("panic 之后其它 handler 仍应执行，healthy = %d", got)
	}
}

func TestMiddlewareOrderAndShortCircuit(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	t.Run("顺序", func(t *testing.T) {
		d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 4, Logger: discardLogger()})
		d.Use(
			func(next event.Handler) event.Handler {
				return func(ctx context.Context, ev types.Event) error {
					record("mw1-in")
					err := next(ctx, ev)
					record("mw1-out")
					return err
				}
			},
			func(next event.Handler) event.Handler {
				return func(ctx context.Context, ev types.Event) error {
					record("mw2-in")
					err := next(ctx, ev)
					record("mw2-out")
					return err
				}
			},
		)
		d.SubscribeAll(func(context.Context, types.Event) error {
			record("handler")
			return nil
		})
		d.Dispatch(context.Background(), recallEvent(t, 1))
		if err := d.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
		mu.Lock()
		got := append([]string(nil), order...)
		mu.Unlock()
		want := []string{"mw1-in", "mw2-in", "handler", "mw2-out", "mw1-out"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("中间件顺序 = %v, want %v", got, want)
		}
	})

	t.Run("短路", func(t *testing.T) {
		var called int64
		d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 4, Logger: discardLogger()})
		d.Use(func(event.Handler) event.Handler {
			return func(context.Context, types.Event) error {
				return errors.New("被拦截")
			}
		})
		d.SubscribeAll(func(context.Context, types.Event) error {
			atomic.AddInt64(&called, 1)
			return nil
		})
		d.Dispatch(context.Background(), recallEvent(t, 1))
		if err := d.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if atomic.LoadInt64(&called) != 0 {
			t.Error("中间件短路后 handler 不应执行")
		}
		if d.Stats().Failed != 1 {
			t.Errorf("Failed = %d, want 1", d.Stats().Failed)
		}
	})
}

func TestUnsubscribeAndOnce(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 16, Logger: discardLogger()})

	var counted int64
	off := d.SubscribeType(types.EventMessageRecall, func(context.Context, types.Event) error {
		atomic.AddInt64(&counted, 1)
		return nil
	})
	off() // 立即注销
	d.Dispatch(context.Background(), recallEvent(t, 1))

	var onceCount int64
	event.Once(d, func(context.Context, *types.MessageRecallEvent) error {
		atomic.AddInt64(&onceCount, 1)
		return nil
	})
	d.Dispatch(context.Background(), recallEvent(t, 2))
	d.Dispatch(context.Background(), recallEvent(t, 3))

	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if atomic.LoadInt64(&counted) != 0 {
		t.Errorf("注销后仍收到 %d 次事件", counted)
	}
	if got := atomic.LoadInt64(&onceCount); got != 1 {
		t.Errorf("Once 触发次数 = %d, want 1", got)
	}
}

func TestTypedSubscriptionOnlySeesOwnType(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 16, Logger: discardLogger()})

	var msgCount, recallCount int64
	event.On(d, func(context.Context, *types.MessageReceiveEvent) error {
		atomic.AddInt64(&msgCount, 1)
		return nil
	})
	event.On(d, func(context.Context, *types.MessageRecallEvent) error {
		atomic.AddInt64(&recallCount, 1)
		return nil
	})

	d.Dispatch(context.Background(), msgEvent(t, 1, 1))
	d.Dispatch(context.Background(), recallEvent(t, 2))
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if msgCount != 1 || recallCount != 1 {
		t.Fatalf("typed 订阅错配: msg=%d recall=%d", msgCount, recallCount)
	}
}

func TestCloseDrainsQueue(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeConcurrent, Workers: 2, Buffer: 64, Logger: discardLogger()})
	var handled int64
	d.SubscribeAll(func(context.Context, types.Event) error {
		atomic.AddInt64(&handled, 1)
		return nil
	})
	for i := 0; i < 20; i++ {
		d.Dispatch(context.Background(), recallEvent(t, int64(i+1)))
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt64(&handled); got != 20 {
		t.Errorf("Close 应排空队列: handled = %d, want 20", got)
	}
	// 关闭后投递应被忽略且不 panic。
	d.Dispatch(context.Background(), recallEvent(t, 99))
	if d.Stats().Dropped != 0 {
		t.Errorf("关闭后的投递不应计入 dropped")
	}
}

func TestCloseHonorsContext(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 4, Logger: discardLogger()})
	d.SubscribeAll(func(context.Context, types.Event) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	})
	d.Dispatch(context.Background(), recallEvent(t, 1))
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := d.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close err = %v, want DeadlineExceeded", err)
	}
	close(release)
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("handler 结束后 Close 应成功: %v", err)
	}
}

func TestHandlerTimeout(t *testing.T) {
	d := event.New(event.Config{
		Mode:           event.ModeSequential,
		Buffer:         4,
		HandlerTimeout: 20 * time.Millisecond,
		Logger:         discardLogger(),
	})
	errCh := make(chan error, 1)
	d.SubscribeAll(func(ctx context.Context, _ types.Event) error {
		<-ctx.Done()
		errCh <- ctx.Err()
		return nil
	})
	d.Dispatch(context.Background(), recallEvent(t, 1))
	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("handler 的 ctx err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler 未在超时后收到 ctx 取消")
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestStats(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	d.Dispatch(context.Background(), recallEvent(t, 1))
	waitFor(t, time.Second, func() bool { return d.Stats().Handled == 1 }, "handled 计数")
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	stats := d.Stats()
	if stats.Mode != event.ModeSequential || stats.Workers != 1 {
		t.Errorf("stats = %+v", stats)
	}
}
