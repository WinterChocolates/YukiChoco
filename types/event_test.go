package types_test

import (
	"encoding/json"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

func TestParseGroupMessageEvent(t *testing.T) {
	raw := `{
		"time": 1700000000, "self_id": 10000, "post_type": "message",
		"message_type": "group", "sub_type": "normal", "message_id": 123,
		"group_id": 20001, "user_id": 30001,
		"sender": {"user_id": 30001, "nickname": "n", "card": "c", "role": "admin"},
		"message": [{"type":"at","data":{"qq":"10000"}},{"type":"text","data":{"text":" /echo hi"}}],
		"raw_message": "[CQ:at,qq=10000] /echo hi", "font": 0
	}`
	ev, err := types.ParseEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := ev.(*types.GroupMessageEvent)
	if !ok {
		t.Fatalf("type = %T", ev)
	}
	if msg.EventType() != types.EventTypeGroupMessage {
		t.Errorf("EventType=%s", msg.EventType())
	}
	if msg.GroupID != 20001 || msg.UserID != 30001 || msg.MessageID != 123 {
		t.Errorf("fields=%+v", msg)
	}
	if msg.Sender.Role != types.RoleAdmin {
		t.Errorf("role=%s", msg.Sender.Role)
	}
	if !msg.AtMe(10000) || msg.AtAll() {
		t.Error("AtMe/AtAll 判断错误")
	}
	if msg.PlainText() != " /echo hi" {
		t.Errorf("text=%q", msg.PlainText())
	}
	if msg.EventSelfID() != 10000 {
		t.Errorf("SelfID=%d", msg.EventSelfID())
	}
	if msg.OccurredAt().Unix() != 1700000000 {
		t.Errorf("time=%v", msg.OccurredAt())
	}
	if len(msg.RawJSON()) == 0 {
		t.Error("RawJSON 为空")
	}
}

func TestParsePrivateMessageCQ(t *testing.T) {
	raw := `{"time":1,"self_id":2,"post_type":"message","message_type":"private",
		"sub_type":"friend","message_id":3,"user_id":4,
		"message":"[CQ:face,id=14]hi","raw_message":"[CQ:face,id=14]hi","font":0}`
	ev, err := types.ParseEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := ev.(*types.PrivateMessageEvent)
	if !ok {
		t.Fatalf("type=%T", ev)
	}
	if msg.MessageType != types.MessageTypePrivate || len(msg.Message) != 2 {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.PlainText() != "hi" {
		t.Errorf("text=%q", msg.PlainText())
	}
}

func TestParseNoticeEvents(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		want  types.EventType
		check func(t *testing.T, ev types.Event)
	}{
		{
			name: "group_upload",
			raw: `{"post_type":"notice","notice_type":"group_upload","group_id":1,"user_id":2,
				"file":{"id":"f","name":"a.txt","size":10,"busid":1}}`,
			want: types.EventTypeGroupUpload,
			check: func(t *testing.T, ev types.Event) {
				e := ev.(*types.GroupUploadEvent)
				if e.File.Name != "a.txt" || e.File.Size != 10 {
					t.Errorf("file=%+v", e.File)
				}
			},
		},
		{
			name: "group_ban",
			raw:  `{"post_type":"notice","notice_type":"group_ban","sub_type":"ban","group_id":1,"user_id":2,"operator_id":3,"duration":60}`,
			want: types.EventTypeGroupBan,
			check: func(t *testing.T, ev types.Event) {
				e := ev.(*types.GroupBanEvent)
				if e.Duration != 60 || e.SubType != "ban" || e.OperatorID != 3 {
					t.Errorf("ev=%+v", e)
				}
			},
		},
		{
			name: "notify_poke",
			raw:  `{"post_type":"notice","notice_type":"notify","sub_type":"poke","group_id":1,"user_id":2,"target_id":3}`,
			want: types.EventTypeNotify,
			check: func(t *testing.T, ev types.Event) {
				e := ev.(*types.NotifyEvent)
				if !e.IsPoke() || e.TargetID != 3 {
					t.Errorf("ev=%+v", e)
				}
			},
		},
		{
			name: "group_recall",
			raw:  `{"post_type":"notice","notice_type":"group_recall","group_id":1,"user_id":2,"operator_id":3,"message_id":9}`,
			want: types.EventTypeGroupRecall,
			check: func(t *testing.T, ev types.Event) {
				if ev.(*types.GroupRecallEvent).MessageID != 9 {
					t.Error("message_id")
				}
			},
		},
		{
			name: "essence",
			raw:  `{"post_type":"notice","notice_type":"essence","sub_type":"add","group_id":1,"message_id":9,"operator_id":3}`,
			want: types.EventTypeEssence,
			check: func(t *testing.T, ev types.Event) {
				if ev.(*types.EssenceEvent).SubType != "add" {
					t.Error("sub_type")
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, err := types.ParseEvent([]byte(c.raw))
			if err != nil {
				t.Fatal(err)
			}
			if ev.EventType() != c.want {
				t.Fatalf("EventType=%s, want %s", ev.EventType(), c.want)
			}
			c.check(t, ev)
		})
	}
}

func TestParseRequestsAndMeta(t *testing.T) {
	friend, err := types.ParseEvent([]byte(`{"post_type":"request","request_type":"friend","user_id":1,"comment":"hi","flag":123}`))
	if err != nil {
		t.Fatal(err)
	}
	if friend.EventType() != types.EventTypeFriendRequest || friend.(*types.FriendRequestEvent).Flag.String() != "123" {
		t.Errorf("friend=%+v", friend)
	}

	group, err := types.ParseEvent([]byte(`{"post_type":"request","request_type":"group","sub_type":"invite","group_id":2,"user_id":3,"comment":"","flag":"f"}`))
	if err != nil {
		t.Fatal(err)
	}
	if group.EventType() != types.EventTypeGroupRequest || group.(*types.GroupRequestEvent).SubType != "invite" {
		t.Errorf("group=%+v", group)
	}

	hb, err := types.ParseEvent([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","time":1,"self_id":2,"interval":5000,"status":{"online":true,"good":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	h := hb.(*types.HeartbeatEvent)
	if h.EventType() != types.EventTypeHeartbeat || h.Status == nil || !h.Status.Online || h.Interval != 5000 {
		t.Errorf("heartbeat=%+v", h)
	}

	lc, err := types.ParseEvent([]byte(`{"post_type":"meta_event","meta_event_type":"lifecycle","sub_type":"connect","self_id":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if lc.EventType() != types.EventTypeLifecycle || lc.(*types.LifecycleEvent).SubType != "connect" {
		t.Errorf("lifecycle=%+v", lc)
	}
}

func TestParseUnknownEvent(t *testing.T) {
	raw := `{"post_type":"notice","notice_type":"brand_new","user_id":1,"extra":{"a":1}}`
	ev, err := types.ParseEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	unknown, ok := ev.(*types.UnknownEvent)
	if !ok {
		t.Fatalf("type=%T", ev)
	}
	if unknown.EventType() != types.EventTypeUnknown {
		t.Errorf("EventType=%s", unknown.EventType())
	}
	var payload map[string]any
	if err := json.Unmarshal(unknown.Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["notice_type"] != "brand_new" {
		t.Errorf("raw=%s", unknown.Raw)
	}
	if unknown.PostType != types.PostTypeNotice {
		t.Errorf("PostType=%s", unknown.PostType)
	}
}

func TestParseEventInvalid(t *testing.T) {
	if _, err := types.ParseEvent([]byte(`{`)); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}
