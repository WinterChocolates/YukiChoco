package event_test

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestMiddlewareChain(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	d.Use(
		event.Filter(func(ev types.Event) bool { return ev.EventType() == types.EventTypeGroupMessage }),
		event.Timeout(50*time.Millisecond),
		event.Logging(logger, slog.LevelDebug),
	)
	var hits atomic.Int32
	d.SubscribeAll(func(context.Context, types.Event) error {
		hits.Add(1)
		return nil
	})
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "a", 0))
	d.Dispatch(context.Background(), &types.LifecycleEvent{MetaEvent: types.MetaEvent{
		EventBase:     types.EventBase{SelfID: 1, PostType: types.PostTypeMetaEvent},
		MetaEventType: "lifecycle",
	}})
	waitFor(t, time.Second, func() bool { return hits.Load() == 1 })
}

func TestDefaultKeyFunc(t *testing.T) {
	group := newGroupEvent(5, 6, "x", 0)
	if got := event.DefaultKeyFunc(group); got != "group:5" {
		t.Errorf("group key=%s", got)
	}
	priv := &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{UserID: 7}}
	if got := event.DefaultKeyFunc(priv); got != "private:7" {
		t.Errorf("private key=%s", got)
	}
	if got := event.DefaultKeyFunc(&types.LifecycleEvent{MetaEvent: types.MetaEvent{EventBase: types.EventBase{SelfID: 9}}}); got != "event:meta.lifecycle:9" {
		t.Errorf("meta key=%s", got)
	}
}

func TestGenericRouter(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var mu sync.Mutex
	var hits int
	event.NewRouter(d).
		Type(types.EventTypeGroupMessage).
		Where(func(ev types.Event) bool { return ev.EventSelfID() == 10000 }).
		Handle(func(context.Context, types.Event) error {
			mu.Lock()
			hits++
			mu.Unlock()
			return nil
		})
	d.Dispatch(context.Background(), newGroupEvent(1, 2, "x", 0))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return hits == 1
	})
}

func TestGroupRouterPredicates(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 32})
	defer d.Close(context.Background())

	var suffix, contains, atAll, admin, owner, regex int32
	event.RouteGroup(d).Suffix("end").Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&suffix, 1)
		return nil
	})
	event.RouteGroup(d).Contains("mid").Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&contains, 1)
		return nil
	})
	event.RouteGroup(d).AtAll().Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&atAll, 1)
		return nil
	})
	event.RouteGroup(d).Admin().Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&admin, 1)
		return nil
	})
	event.RouteGroup(d).Owner().Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&owner, 1)
		return nil
	})
	event.RouteGroup(d).Regex(regexp.MustCompile(`^/re`)).Handle(func(context.Context, *types.GroupMessageEvent) error {
		atomic.AddInt32(&regex, 1)
		return nil
	})

	ev := newGroupEvent(1, 2, "a mid end", 0)
	d.Dispatch(context.Background(), ev)
	atAllEv := newGroupEvent(1, 2, "all", 0)
	atAllEv.Message = append(atAllEv.Message, types.Segment{Type: types.SegmentAt, Data: types.SegmentData{"qq": "all"}})
	d.Dispatch(context.Background(), atAllEv)
	adminEv := newGroupEvent(1, 2, "/re admin", 0)
	adminEv.Sender.Role = types.RoleAdmin
	d.Dispatch(context.Background(), adminEv)
	ownerEv := newGroupEvent(1, 2, "/re owner", 0)
	ownerEv.Sender.Role = types.RoleOwner
	d.Dispatch(context.Background(), ownerEv)

	waitFor(t, time.Second, func() bool {
		return atomic.LoadInt32(&suffix) == 1 && atomic.LoadInt32(&contains) == 1 &&
			atomic.LoadInt32(&atAll) == 1 && atomic.LoadInt32(&admin) == 2 &&
			atomic.LoadInt32(&owner) == 1 && atomic.LoadInt32(&regex) == 2
	})
}

func TestPrivateRouterPredicates(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8})
	defer d.Close(context.Background())

	var contains, regex int32
	event.RoutePrivate(d).Contains("ping").Handle(func(context.Context, *types.PrivateMessageEvent) error {
		atomic.AddInt32(&contains, 1)
		return nil
	})
	event.RoutePrivate(d).Regex(regexp.MustCompile(`^/cmd`)).Handle(func(context.Context, *types.PrivateMessageEvent) error {
		atomic.AddInt32(&regex, 1)
		return nil
	})
	d.Dispatch(context.Background(), &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{
		Message: types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "ping"}}},
	}})
	d.Dispatch(context.Background(), &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{
		Message: types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "/cmd x"}}},
	}})
	waitFor(t, time.Second, func() bool {
		return atomic.LoadInt32(&contains) == 1 && atomic.LoadInt32(&regex) == 1
	})
}
