package event_test

import (
	"context"
	"testing"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func BenchmarkDispatch(b *testing.B) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 4096})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	ev := newGroupEvent(1, 2, "/echo hi", 10000)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	_ = d.Close(ctx)
}
