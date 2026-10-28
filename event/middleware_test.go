package event_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestTimeoutMiddleware(t *testing.T) {
	var expired int32
	h := event.Timeout(20 * time.Millisecond)(func(ctx context.Context, _ types.Event) error {
		<-ctx.Done()
		atomic.AddInt32(&expired, 1)
		return ctx.Err()
	})
	err := h(context.Background(), recallEvent(t, 1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if atomic.LoadInt32(&expired) != 1 {
		t.Fatal("handler 未收到 ctx 取消")
	}

	// 非正数表示不限时。
	h = event.Timeout(0)(func(context.Context, types.Event) error { return nil })
	if err := h(context.Background(), recallEvent(t, 1)); err != nil {
		t.Fatalf("不加超时时应正常返回: %v", err)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := event.Logging(logger, slog.LevelInfo)(func(context.Context, types.Event) error {
		return errors.New("boom")
	})
	if err := h(context.Background(), recallEvent(t, 1)); err == nil {
		t.Fatal("应透传错误")
	}
	out := buf.String()
	if out == "" {
		t.Fatal("未输出日志")
	}
	for _, want := range []string{"message_recall", "elapsed_ms", "boom"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("日志缺少 %q: %s", want, out)
		}
	}
}

func TestDefaultKeyFuncVariants(t *testing.T) {
	// 群消息
	if got := event.DefaultKeyFunc(groupEvent(t, 42, 200, "hi", types.GroupMemberRoleMember, false)); got != "group:42" {
		t.Errorf("群消息 key = %q", got)
	}
	// 私聊
	if got := event.DefaultKeyFunc(friendEvent(t)); got != "friend:200" {
		t.Errorf("私聊 key = %q", got)
	}
	// 其它事件按类型分组
	if got := event.DefaultKeyFunc(recallEvent(t, 1)); got != "*types.MessageRecallEvent" {
		t.Errorf("其它事件 key = %q", got)
	}
	// 非 message_receive 的 message 事件（未知 scene）走 self 分支
	raw := []byte(`{"time":1,"self_id":9,"event_type":"message_receive","data":{"message_scene":"temp","peer_id":1,"message_seq":1,"sender_id":2,"time":1,"segments":[]}}`)
	ev, err := types.UnmarshalEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := event.DefaultKeyFunc(ev); got != "temp:1" {
		t.Errorf("临时会话 key = %q", got)
	}
}

func TestGroupRouterMentionsAll(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	var hits int64
	event.RouteGroup(d).MentionsAll().Handle(func(context.Context, *types.GroupMessage) error {
		atomic.AddInt64(&hits, 1)
		return nil
	})

	// 构造一条含 mention_all 的群消息
	raw := `{"time":1,"self_id":100,"event_type":"message_receive","data":{
		"message_scene":"group","peer_id":1,"message_seq":1,"sender_id":2,"time":1,
		"segments":[{"type":"mention_all","data":{}},{"type":"text","data":{"text":"通知"}}],
		"group":{"group_id":1,"group_name":"g","member_count":1,"max_member_count":1},
		"group_member":{"user_id":2,"nickname":"n","sex":"unknown","group_id":1,
			"card":"","title":"","level":1,"role":"admin","join_time":1,"last_sent_time":1}}}`
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	d.Dispatch(context.Background(), ev)
	d.Dispatch(context.Background(), groupEvent(t, 1, 2, "普通消息", types.GroupMemberRoleMember, false))
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if hits != 1 {
		t.Fatalf("MentionsAll 命中 %d 次, want 1", hits)
	}
}
