package event_test

import (
	"context"
	"testing"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

// benchEvent 预构造一条事件，避免把解码开销算进分发基准。
func benchEvent(b *testing.B) types.Event {
	b.Helper()
	raw := `{"time":1,"self_id":100,"event_type":"message_receive","data":{
		"message_scene":"group","peer_id":200,"message_seq":7,"sender_id":300,"time":1,
		"segments":[{"type":"text","data":{"text":"hi"}}],
		"group":{"group_id":200,"group_name":"g","member_count":1,"max_member_count":1},
		"group_member":{"user_id":300,"nickname":"n","sex":"unknown","group_id":200,
			"card":"","title":"","level":1,"role":"member","join_time":1,"last_sent_time":1}}}`
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		b.Fatalf("构造事件失败: %v", err)
	}
	return ev
}

// BenchmarkDispatchConcurrent 测并发模式的端到端吞吐（投递 + 分发 + 空 handler）。
func BenchmarkDispatchConcurrent(b *testing.B) {
	ev := benchEvent(b)
	d := event.New(event.Config{
		Mode:         event.ModeConcurrent,
		Workers:      8,
		Buffer:       1024,
		Backpressure: event.BackpressureBlock,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	if err := d.Close(context.Background()); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

// BenchmarkDispatchSequential 测全局有序模式的吞吐。
func BenchmarkDispatchSequential(b *testing.B) {
	ev := benchEvent(b)
	d := event.New(event.Config{
		Mode:         event.ModeSequential,
		Buffer:       1024,
		Backpressure: event.BackpressureBlock,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	if err := d.Close(context.Background()); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

// BenchmarkDispatchPerPeer 测按会话有序模式（8 分片）。
func BenchmarkDispatchPerPeer(b *testing.B) {
	ev := benchEvent(b)
	d := event.New(event.Config{
		Mode:         event.ModePerPeerSequential,
		Workers:      8,
		Buffer:       1024,
		Backpressure: event.BackpressureBlock,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	if err := d.Close(context.Background()); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

// BenchmarkDispatchDropWhenFull 测队列满时的丢弃路径开销。
func BenchmarkDispatchDropWhenFull(b *testing.B) {
	ev := benchEvent(b)
	block := make(chan struct{})
	d := event.New(event.Config{
		Mode:         event.ModeSequential,
		Buffer:       1,
		Backpressure: event.BackpressureDropNewest,
		Logger:       discardLogger(),
	})
	d.SubscribeAll(func(context.Context, types.Event) error {
		<-block
		return nil
	})
	d.Dispatch(context.Background(), ev) // 占住唯一的 worker 与队列位
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	close(block)
	if err := d.Close(context.Background()); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

func BenchmarkDefaultKeyFunc(b *testing.B) {
	ev := benchEvent(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = event.DefaultKeyFunc(ev)
	}
}
