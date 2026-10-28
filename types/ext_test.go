package types_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

func TestIDSemantics(t *testing.T) {
	id := types.UserID(123456789)
	if id.Int64() != 123456789 || id.String() != "123456789" || id.IsZero() {
		t.Fatalf("UserID 行为异常: %d %s %v", id.Int64(), id.String(), id.IsZero())
	}
	var zero types.UserID
	if !zero.IsZero() || zero.String() != "0" {
		t.Fatalf("零值 UserID 行为异常: %s %v", zero.String(), zero.IsZero())
	}

	raw, err := json.Marshal(types.GroupID(-1))
	if err != nil || string(raw) != "-1" {
		t.Fatalf("GroupID Marshal = %s, %v", raw, err)
	}
	raw, err = json.Marshal(types.MessageSeq(42))
	if err != nil || string(raw) != "42" {
		t.Fatalf("MessageSeq Marshal = %s, %v", raw, err)
	}
	raw, err = json.Marshal(types.Uin(7))
	if err != nil || string(raw) != "7" {
		t.Fatalf("Uin Marshal = %s, %v", raw, err)
	}
}

func TestUnknownMessageScene(t *testing.T) {
	raw := `{"message_scene":"future_scene","peer_id":1,"message_seq":2,"sender_id":3,"time":4,"segments":[]}`
	msg, err := types.UnmarshalIncomingMessage([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalIncomingMessage: %v", err)
	}
	unknown, ok := msg.(*types.UnknownIncomingMessage)
	if !ok {
		t.Fatalf("类型 = %T, want *types.UnknownIncomingMessage", msg)
	}
	if unknown.Tag != "future_scene" {
		t.Errorf("Tag = %q", unknown.Tag)
	}
	// 未知场景的访问器返回零值，不应 panic。
	if msg.Scene() != "" || msg.Peer() != 0 || msg.Seq() != 0 || msg.Sender() != 0 || msg.MessageSegments() != nil {
		t.Errorf("未知场景访问器应返回零值")
	}
	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != raw {
		t.Errorf("未知消息未原样保留:\n got=%s\nwant=%s", out, raw)
	}
}

func TestGroupNotificationVariants(t *testing.T) {
	join := `{"type":"join_request","group_id":1,"notification_seq":2,"is_filtered":false,"initiator_id":3,"state":"pending","comment":"hi"}`
	var n types.GroupNotification
	n, err := types.UnmarshalGroupNotification([]byte(join))
	if err != nil {
		t.Fatalf("UnmarshalGroupNotification: %v", err)
	}
	req, ok := n.(*types.GroupJoinRequestNotification)
	if !ok {
		t.Fatalf("类型 = %T", n)
	}
	if req.Group() != 1 || req.Kind() != types.GroupNotificationJoinRequest {
		t.Errorf("访问器错误: group=%d kind=%s", req.Group(), req.Kind())
	}
	if req.OperatorID != nil {
		t.Errorf("可选字段 operator_id 应为 nil")
	}

	// 未识别通知类型同样透传。
	unknownRaw := `{"type":"future_notification","group_id":9}`
	n, err = types.UnmarshalGroupNotification([]byte(unknownRaw))
	if err != nil {
		t.Fatalf("UnmarshalGroupNotification(unknown): %v", err)
	}
	if _, ok := n.(*types.UnknownGroupNotification); !ok {
		t.Fatalf("类型 = %T", n)
	}
	out, _ := json.Marshal(n)
	if string(out) != unknownRaw {
		t.Errorf("未知通知未原样保留: %s", out)
	}
}

func TestSegmentHelpers(t *testing.T) {
	var segs types.IncomingSegments
	raw := `[
		{"type":"reply","data":{"message_seq":11,"sender_id":1,"time":2,"segments":[]}},
		{"type":"mention","data":{"user_id":100,"name":"bot"}},
		{"type":"mention_all","data":{}},
		{"type":"text","data":{"text":"hi "}},
		{"type":"image","data":{"resource_id":"r","temp_url":"u","width":1,"height":1,"summary":"","sub_type":"normal"}},
		{"type":"text","data":{"text":"there"}},
		{"type":"file","data":{"file_id":"f","file_name":"a.txt","file_size":1}}
	]`
	if err := json.Unmarshal([]byte(raw), &segs); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := segs.PlainText(); got != "hi there" {
		t.Errorf("PlainText = %q", got)
	}
	if !segs.Mentioned(100) || segs.Mentioned(101) {
		t.Error("Mentioned 行为异常")
	}
	if !segs.MentionsAll() {
		t.Error("MentionsAll 应为 true")
	}
	if seq, ok := segs.ReplySeq(); !ok || seq != 11 {
		t.Errorf("ReplySeq = %d, %v", seq, ok)
	}
	if imgs := segs.Images(); len(imgs) != 1 || imgs[0].ResourceID != "r" {
		t.Errorf("Images = %+v", imgs)
	}
	if kinds := segs.Types(); len(kinds) != 7 || kinds[0] != types.SegmentReply {
		t.Errorf("Types = %v", kinds)
	}

	rendered := segs.String()
	for _, want := range []string{"[reply:11]", "@100", "@全体成员", "hi ", "[image]", "there", "[file:a.txt]"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("String() 缺少 %q: %s", want, rendered)
		}
	}

	// 无 reply 时返回 ok=false。
	var plain types.IncomingSegments
	if _, ok := plain.ReplySeq(); ok {
		t.Error("空消息不应有 reply")
	}
	if plain.Mentioned(1) || plain.MentionsAll() || plain.PlainText() != "" || plain.Images() != nil {
		t.Error("空消息的辅助方法应返回零值")
	}

	// 发送侧 PlainText
	out := types.OutgoingSegments{
		&types.OutgoingTextSegment{Text: "a"},
		&types.OutgoingMentionSegment{UserID: 1},
		&types.OutgoingTextSegment{Text: "b"},
	}
	if got := out.PlainText(); got != "ab" {
		t.Errorf("OutgoingSegments.PlainText = %q", got)
	}
}

func TestMessageExtHelpers(t *testing.T) {
	in := `{"time":1,"self_id":100,"event_type":"message_receive","data":{
		"message_scene":"group","peer_id":200,"message_seq":7,"sender_id":300,"time":1,
		"segments":[{"type":"text","data":{"text":"hi"}}],
		"group":{"group_id":200,"group_name":"g","member_count":1,"max_member_count":1},
		"group_member":{"user_id":300,"nickname":"n","sex":"unknown","group_id":200,
			"card":"","title":"","level":1,"role":"member","join_time":1,"last_sent_time":1}}}`
	ev, err := types.UnmarshalEvent([]byte(in))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	recv := ev.(*types.MessageReceiveEvent)
	msg, ok := recv.GroupMessage()
	if !ok {
		t.Fatalf("GroupMessage 失败: %T", recv.Data)
	}
	if msg.PlainText() != "hi" {
		t.Errorf("PlainText = %q", msg.PlainText())
	}
	if _, ok := recv.FriendMessage(); ok {
		t.Error("FriendMessage 不应命中群消息")
	}
	if _, ok := recv.TempMessage(); ok {
		t.Error("TempMessage 不应命中群消息")
	}

	// DisplayName：群名片优先，其次昵称；好友备注优先。
	member := &types.GroupMemberEntity{Nickname: "nick", Card: ""}
	if got := member.DisplayName(); got != "nick" {
		t.Errorf("DisplayName = %q", got)
	}
	member.Card = "card"
	if got := member.DisplayName(); got != "card" {
		t.Errorf("DisplayName = %q", got)
	}
	friend := &types.FriendEntity{Nickname: "nick", Remark: ""}
	if got := friend.DisplayName(); got != "nick" {
		t.Errorf("DisplayName = %q", got)
	}
	friend.Remark = "remark"
	if got := friend.DisplayName(); got != "remark" {
		t.Errorf("DisplayName = %q", got)
	}
}

func TestEnumString(t *testing.T) {
	cases := map[string]string{
		types.SegmentText.String():                  "text",
		types.SegmentMentionAll.String():            "mention_all",
		types.EventMessageReceive.String():          "message_receive",
		types.GroupMemberRoleAdmin.String():         "admin",
		types.RequestStatePending.String():          "pending",
		types.ImageSubTypeSticker.String():          "sticker",
		types.GroupNotificationJoinRequest.String(): "join_request",
		types.QQProtocolTypeAndroidPhone.String():   "android_phone",
		types.GroupNotificationAdminChange.String(): "admin_change",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("枚举 String = %q, want %q", got, want)
		}
	}
	if types.MessageScene("weird").Valid() {
		t.Error("未知枚举值不应 Valid")
	}
}

func TestIDInt64Accessors(t *testing.T) {
	if got := types.GroupID(5).Int64(); got != 5 {
		t.Errorf("GroupID.Int64 = %d", got)
	}
	if got := types.Uin(6).Int64(); got != 6 {
		t.Errorf("Uin.Int64 = %d", got)
	}
	if got := types.MessageSeq(7).Int64(); got != 7 {
		t.Errorf("MessageSeq.Int64 = %d", got)
	}
	if types.GroupID(0).IsZero() != true || types.Uin(1).IsZero() != false {
		t.Error("IsZero 行为异常")
	}
	if types.MessageSeq(8).String() != "8" {
		t.Error("MessageSeq.String 行为异常")
	}
}
