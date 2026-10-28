package event_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

// groupEvent 构造一条群消息事件。
func groupEvent(t *testing.T, groupID int64, sender int64, text string, role types.GroupMemberRole, mentionSelf bool) types.Event {
	t.Helper()
	segments := fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, text)
	if mentionSelf {
		segments = fmt.Sprintf(`[{"type":"mention","data":{"user_id":100,"name":"bot"}},{"type":"text","data":{"text":%q}}]`, text)
	}
	raw := fmt.Sprintf(`{"time":1,"self_id":100,"event_type":"message_receive","data":{
		"message_scene":"group","peer_id":%d,"message_seq":1,"sender_id":%d,"time":1,
		"segments":%s,
		"group":{"group_id":%d,"group_name":"g","member_count":1,"max_member_count":1},
		"group_member":{"user_id":%d,"nickname":"n","sex":"unknown","group_id":%d,
			"card":"","title":"","level":1,"role":%q,"join_time":1,"last_sent_time":1}}}`,
		groupID, sender, segments, groupID, sender, groupID, role)
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		t.Fatalf("构造事件失败: %v", err)
	}
	return ev
}

func TestGroupRouterPredicates(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 32, Logger: discardLogger()})
	base := event.RouteGroup(d)

	var atMeGroup1, allAdmin, prefix int64
	base.Group(1).AtMe(100).Handle(func(context.Context, *types.GroupMessage) error {
		atomic.AddInt64(&atMeGroup1, 1)
		return nil
	})
	base.Admin().Handle(func(context.Context, *types.GroupMessage) error {
		atomic.AddInt64(&allAdmin, 1)
		return nil
	})
	base.Prefix("/cmd").Handle(func(context.Context, *types.GroupMessage) error {
		atomic.AddInt64(&prefix, 1)
		return nil
	})

	// 命中：群 1 + @bot
	d.Dispatch(context.Background(), groupEvent(t, 1, 200, "hi", types.GroupMemberRoleMember, true))
	// 不命中：群 2 + @bot
	d.Dispatch(context.Background(), groupEvent(t, 2, 200, "hi", types.GroupMemberRoleMember, true))
	// 命中 admin（群主）
	d.Dispatch(context.Background(), groupEvent(t, 2, 300, "hi", types.GroupMemberRoleOwner, false))
	// 命中 prefix
	d.Dispatch(context.Background(), groupEvent(t, 3, 200, "/cmd now", types.GroupMemberRoleMember, false))
	// 私聊消息不应进入 GroupRouter
	d.Dispatch(context.Background(), friendEvent(t))

	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := atomic.LoadInt64(&atMeGroup1); got != 1 {
		t.Errorf("Group(1).AtMe 命中 %d 次, want 1", got)
	}
	if got := atomic.LoadInt64(&allAdmin); got != 1 {
		t.Errorf("Admin 命中 %d 次, want 1", got)
	}
	if got := atomic.LoadInt64(&prefix); got != 1 {
		t.Errorf("Prefix 命中 %d 次, want 1", got)
	}
}

func TestGroupRouterUnsubscribe(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	var calls int64
	off := event.RouteGroup(d).Handle(func(context.Context, *types.GroupMessage) error {
		atomic.AddInt64(&calls, 1)
		return nil
	})
	off()
	d.Dispatch(context.Background(), groupEvent(t, 1, 200, "hi", types.GroupMemberRoleMember, false))
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Fatal("注销后不应再回调")
	}
}

func TestGroupRouterHandleEvent(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	var gotText string
	event.RouteGroup(d).Group(7).HandleEvent(func(_ context.Context, recv *types.MessageReceiveEvent, msg *types.GroupMessage) error {
		gotText = msg.PlainText() + "|" + recv.EventType().String()
		return nil
	})
	d.Dispatch(context.Background(), groupEvent(t, 7, 200, "hello", types.GroupMemberRoleMember, false))
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if gotText != "hello|message_receive" {
		t.Fatalf("HandleEvent got %q", gotText)
	}
}

func TestRouterTypeAndWhere(t *testing.T) {
	d := event.New(event.Config{Mode: event.ModeSequential, Buffer: 8, Logger: discardLogger()})
	var recallCount, whereCount int64
	event.NewRouter(d).Type(types.EventMessageRecall).Handle(func(context.Context, types.Event) error {
		atomic.AddInt64(&recallCount, 1)
		return nil
	})
	event.NewRouter(d).Where(func(ev types.Event) bool {
		return ev.EventType() == types.EventMessageReceive
	}).Handle(func(context.Context, types.Event) error {
		atomic.AddInt64(&whereCount, 1)
		return nil
	})

	d.Dispatch(context.Background(), recallEvent(t, 1))
	d.Dispatch(context.Background(), groupEvent(t, 1, 200, "hi", types.GroupMemberRoleMember, false))
	if err := d.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if recallCount != 1 || whereCount != 1 {
		t.Fatalf("recall=%d where=%d", recallCount, whereCount)
	}
}

// friendEvent 构造一条私聊事件。
func friendEvent(t *testing.T) types.Event {
	t.Helper()
	raw := `{"time":1,"self_id":100,"event_type":"message_receive","data":{
		"message_scene":"friend","peer_id":200,"message_seq":1,"sender_id":200,"time":1,
		"segments":[{"type":"text","data":{"text":"hi"}}],
		"friend":{"user_id":200,"nickname":"n","sex":"unknown","qid":"","remark":"",
			"category":{"category_id":0,"category_name":""}}}}`
	ev, err := types.UnmarshalEvent([]byte(raw))
	if err != nil {
		t.Fatalf("构造私聊事件失败: %v", err)
	}
	return ev
}
