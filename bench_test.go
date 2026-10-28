package YukiChoco

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/mock"
	"github.com/WinterChocolates/YukiChoco/types"
)

// BenchmarkAPICallMeasure 测一次完整 API 调用的框架开销（含 loopback HTTP）。
func BenchmarkAPICallMeasure(b *testing.B) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("get_login_info", func(json.RawMessage) (any, error) {
		return map[string]any{"uin": 100, "nickname": "bot"}, nil
	})

	c, err := New(Config{
		BaseURL:     srv.URL(),
		AccessToken: "secret-token",
		EventSource: EventSourceNone,
	})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.System().GetLoginInfo(ctx); err != nil {
			b.Fatalf("GetLoginInfo: %v", err)
		}
	}
}

// BenchmarkCallEscapeHatch 测泛化调用（map 入参 + 结构体出参）的开销。
func BenchmarkCallEscapeHatch(b *testing.B) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("get_friend_list", func(json.RawMessage) (any, error) {
		return map[string]any{"friends": []any{}}, nil
	})

	c, err := New(Config{BaseURL: srv.URL(), AccessToken: "secret-token", EventSource: EventSourceNone})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer c.Close()

	var out struct {
		Friends []types.FriendEntity `json:"friends"`
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Call(ctx, "get_friend_list", map[string]any{"no_cache": true}, &out); err != nil {
			b.Fatalf("Call: %v", err)
		}
	}
}

// BenchmarkSendGroupMessage 测带消息段的发送调用（含段构造与序列化）。
func BenchmarkSendGroupMessage(b *testing.B) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("send_group_message", func(json.RawMessage) (any, error) {
		return map[string]any{"message_seq": 1, "time": 2}, nil
	})

	c, err := New(Config{BaseURL: srv.URL(), AccessToken: "secret-token", EventSource: EventSourceNone})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer c.Close()

	segs := []types.OutgoingSegment{
		&types.OutgoingReplySegment{MessageSeq: 1},
		&types.OutgoingMentionSegment{UserID: 100},
		&types.OutgoingTextSegment{Text: "hello"},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Group().SendMessage(ctx, 200, segs...); err != nil {
			b.Fatalf("SendMessage: %v", err)
		}
	}
}

// BenchmarkDecodeAndDispatch 测「解码 + 分发」的完整链路吞吐（NFR-1 指标）。
func BenchmarkDecodeAndDispatch(b *testing.B) {
	raw := []byte(groupMessageJSON)
	d := event.New(event.Config{
		Mode:         event.ModeSequential,
		Buffer:       1024,
		Backpressure: event.BackpressureBlock,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	d.SubscribeAll(func(context.Context, types.Event) error { return nil })
	ctx := context.Background()

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev, err := types.UnmarshalEvent(raw)
		if err != nil {
			b.Fatalf("UnmarshalEvent: %v", err)
		}
		d.Dispatch(ctx, ev)
	}
	b.StopTimer()
	if err := d.Close(context.Background()); err != nil {
		b.Fatalf("Close: %v", err)
	}
}
