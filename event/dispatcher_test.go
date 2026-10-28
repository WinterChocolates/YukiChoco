package event_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func newGroupEvent(gid types.GroupID, uid types.UserID, text string, at types.UserID) *types.GroupMessageEvent {
	msg := types.Message{}
	if at != 0 {
		msg = append(msg, types.Segment{Type: types.SegmentAt, Data: types.SegmentData{"qq": at.String()}})
	}
	msg = append(msg, types.Segment{Type: types.SegmentText, Data: types.SegmentData{"text": text}})
	return &types.GroupMessageEvent{MessageEvent: types.MessageEvent{
		EventBase: types.EventBase{
			Time: 1, SelfID: 10000, PostType: types.PostTypeMessage,
		},
		MessageType: types.MessageTypeGroup,
		MessageID:   1,
		GroupID:     gid,
		UserID:      uid,
		Message:     msg,
		Sender:      types.Sender{UserID: uid, Role: types.RoleMember},
	}}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func TestDispatcherSubscribeAndStats(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var mu sync.Mutex
	var got []types.EventType
	var all int
	off := d.SubscribeType(types.EventTypeGroupMessage, func(_ context.Context, ev types.Event) error {
		mu.Lock()
		got = append(got, ev.EventType())
		mu.Unlock()
		return nil
	})
	d.SubscribeAll(func(context.Context, types.Event) error {
		mu.Lock()
		all++
		mu.Unlock()
		return nil
	})
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "hi", 0))

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1 && all == 1
	})
	off()
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "hi2", 0))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return all == 2
	})

	stats := d.Stats()
	if stats.Handled != 2 || stats.Workers != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestDispatcherOrder(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 64})
	defer d.Close(context.Background())

	var mu sync.Mutex
	var order []string
	d.SubscribeType(types.EventTypeGroupMessage, func(_ context.Context, ev types.Event) error {
		mu.Lock()
		order = append(order, ev.(*types.GroupMessageEvent).PlainText())
		mu.Unlock()
		return nil
	})
	for i := 0; i < 20; i++ {
		d.Dispatch(context.Background(), newGroupEvent(1, 2, fmt.Sprint(i), 0))
	}
	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 20
	})
	mu.Lock()
	defer mu.Unlock()
	for i, v := range order {
		if v != fmt.Sprint(i) {
			t.Fatalf("order[%d]=%s", i, v)
		}
	}
}

func TestDispatcherPanicIsolation(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var called atomic.Int32
	d.SubscribeType(types.EventTypeGroupMessage, func(context.Context, types.Event) error {
		panic("boom")
	})
	d.SubscribeType(types.EventTypeGroupMessage, func(context.Context, types.Event) error {
		called.Add(1)
		return nil
	})
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "hi", 0))
	waitFor(t, time.Second, func() bool { return called.Load() == 1 })
	waitFor(t, time.Second, func() bool { return d.Stats().Panicked == 1 })
}

func TestRouterGroup(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var mu sync.Mutex
	var hits int
	event.RouteGroup(d).Group(1).AtMe(10000).Prefix("/cmd").
		Handle(func(context.Context, *types.GroupMessageEvent) error {
			mu.Lock()
			hits++
			mu.Unlock()
			return nil
		})

	d.Dispatch(context.Background(), newGroupEvent(1, 2, "/cmd run", 10000))
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "/cmd run", 0))
	d.Dispatch(context.Background(), newGroupEvent(2, 2, "/cmd run", 10000))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return hits == 1
	})
}

func TestRouterPrivate(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var hits atomic.Int32
	event.RoutePrivate(d).From(7).Prefix("hello").
		Handle(func(context.Context, *types.PrivateMessageEvent) error {
			hits.Add(1)
			return nil
		})

	ev := &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{
		EventBase:   types.EventBase{SelfID: 1, PostType: types.PostTypeMessage},
		MessageType: types.MessageTypePrivate,
		UserID:      7,
		Message:     types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "hello world"}}},
	}}
	d.Dispatch(context.Background(), ev)
	waitFor(t, time.Second, func() bool { return hits.Load() == 1 })
}

func TestOnAndOnce(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var once, on atomic.Int32
	event.Once(d, func(context.Context, *types.GroupMessageEvent) error {
		once.Add(1)
		return nil
	})
	event.On(d, func(context.Context, *types.GroupMessageEvent) error {
		on.Add(1)
		return nil
	})
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "a", 0))
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "b", 0))
	waitFor(t, time.Second, func() bool { return on.Load() == 2 })
	if once.Load() != 1 {
		t.Fatalf("once=%d", once.Load())
	}
}

func TestCloseDrains(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 16})
	var handled atomic.Int32
	d.SubscribeAll(func(context.Context, types.Event) error {
		time.Sleep(5 * time.Millisecond)
		handled.Add(1)
		return nil
	})
	for i := 0; i < 5; i++ {
		d.Dispatch(context.Background(), newGroupEvent(1, 2, "x", 0))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if handled.Load() != 5 {
		t.Fatalf("handled=%d", handled.Load())
	}
	// 关闭后 Dispatch 不应 panic。
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "y", 0))
}
