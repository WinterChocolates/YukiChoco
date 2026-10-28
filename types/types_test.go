package types_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

// 官方文档中的事件示例（Milky 1.3 字段名为 segments）。
const docMessageReceive = `{
  "time": 1234567890,
  "self_id": 123456789,
  "event_type": "message_receive",
  "data": {
    "message_scene": "group",
    "peer_id": 987654321,
    "message_seq": 23333,
    "sender_id": 111222333,
    "time": 1234567890,
    "segments": [
      {"type": "reply", "data": {"message_seq": 23332, "sender_id": 111222333, "time": 1234567889, "segments": [{"type": "text", "data": {"text": "原消息"}}]}},
      {"type": "mention", "data": {"user_id": 123456789, "name": "机器人"}},
      {"type": "text", "data": {"text": " 你好"}},
      {"type": "image", "data": {"resource_id": "res-1", "temp_url": "https://example.com/a.png", "width": 100, "height": 200, "summary": "[图片]", "sub_type": "normal"}}
    ],
    "group": {
      "group_id": 987654321,
      "group_name": "测试群",
      "member_count": 3,
      "max_member_count": 200
    },
    "group_member": {
      "user_id": 111222333,
      "nickname": "群友",
      "sex": "unknown",
      "group_id": 987654321,
      "card": "群名片",
      "title": "",
      "level": 1,
      "role": "member",
      "join_time": 1700000000,
      "last_sent_time": 1700000001
    }
  }
}`

func TestUnmarshalEventMessageReceive(t *testing.T) {
	ev, err := types.UnmarshalEvent([]byte(docMessageReceive))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	if got, want := ev.EventType(), types.EventMessageReceive; got != want {
		t.Fatalf("EventType = %q, want %q", got, want)
	}
	if ev.Meta().SelfID != 123456789 {
		t.Fatalf("SelfID = %d", ev.Meta().SelfID)
	}

	recv, ok := ev.(*types.MessageReceiveEvent)
	if !ok {
		t.Fatalf("事件类型 = %T, want *types.MessageReceiveEvent", ev)
	}
	msg, ok := recv.GroupMessage()
	if !ok {
		t.Fatalf("Data 类型 = %T, want *types.GroupMessage", recv.Data)
	}
	if msg.Group.GroupName != "测试群" {
		t.Fatalf("GroupName = %q", msg.Group.GroupName)
	}
	if msg.GroupMember.Role != types.GroupMemberRoleMember {
		t.Fatalf("Role = %q", msg.GroupMember.Role)
	}
	if got := msg.PlainText(); got != " 你好" {
		t.Fatalf("PlainText = %q", got)
	}
	if !msg.Segments.Mentioned(123456789) {
		t.Fatal("Mentioned(123456789) = false")
	}
	if seq, ok := msg.Segments.ReplySeq(); !ok || seq != 23332 {
		t.Fatalf("ReplySeq = %d, %v", seq, ok)
	}

	// reply 段内嵌的 segments 也必须被正确解码。
	reply, ok := msg.Segments[0].(*types.IncomingReplySegment)
	if !ok {
		t.Fatalf("segments[0] = %T", msg.Segments[0])
	}
	if len(reply.Segments) != 1 {
		t.Fatalf("reply.Segments 长度 = %d", len(reply.Segments))
	}
	if txt, ok := reply.Segments[0].(*types.IncomingTextSegment); !ok || txt.Text != "原消息" {
		t.Fatalf("reply.Segments[0] = %#v", reply.Segments[0])
	}
}

func TestEventMarshalRoundTrip(t *testing.T) {
	ev, err := types.UnmarshalEvent([]byte(docMessageReceive))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var envelope struct {
		EventType string `json:"event_type"`
		SelfID    int64  `json:"self_id"`
		Time      int64  `json:"time"`
		Data      struct {
			MessageScene string          `json:"message_scene"`
			Segments     json.RawMessage `json:"segments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal(round-trip): %v\n%s", err, raw)
	}
	if envelope.EventType != "message_receive" || envelope.Data.MessageScene != "group" {
		t.Fatalf("信封错误: %s", raw)
	}
	if len(envelope.Data.Segments) == 0 {
		t.Fatalf("segments 丢失: %s", raw)
	}
}

func TestUnknownEventPassThrough(t *testing.T) {
	in := `{"time":1,"self_id":2,"event_type":"future_event","data":{"whatever":[1,2,3]}}`
	ev, err := types.UnmarshalEvent([]byte(in))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	unknown, ok := ev.(*types.UnknownEvent)
	if !ok {
		t.Fatalf("事件类型 = %T, want *types.UnknownEvent", ev)
	}
	if unknown.Type != "future_event" || unknown.Meta().SelfID != 2 {
		t.Fatalf("未知事件元信息错误: %+v", unknown)
	}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != in {
		t.Fatalf("未知事件未原样保留:\n got=%s\nwant=%s", out, in)
	}
}

func TestSegmentRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		seg  types.OutgoingSegment
		want string
	}{
		{"text", &types.OutgoingTextSegment{Text: "hi"}, `{"type":"text","data":{"text":"hi"}}`},
		{"mention", &types.OutgoingMentionSegment{UserID: 42}, `{"type":"mention","data":{"user_id":42}}`},
		{"mention_all", &types.OutgoingMentionAllSegment{}, `{"type":"mention_all","data":{}}`},
		{"face", &types.OutgoingFaceSegment{FaceID: "1", IsLarge: true}, `{"type":"face","data":{"face_id":"1","is_large":true}}`},
		{"reply", &types.OutgoingReplySegment{MessageSeq: 7}, `{"type":"reply","data":{"message_seq":7}}`},
		{"image", &types.OutgoingImageSegment{URI: "file:///a.png"}, `{"type":"image","data":{"uri":"file:///a.png"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.seg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(raw) != tc.want {
				t.Fatalf("Marshal =\n %s\nwant\n %s", raw, tc.want)
			}
			back, err := types.UnmarshalOutgoingSegment(raw)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if back.SegmentType() != tc.seg.SegmentType() {
				t.Fatalf("SegmentType = %q, want %q", back.SegmentType(), tc.seg.SegmentType())
			}
		})
	}
}

func TestUnknownSegmentPassThrough(t *testing.T) {
	in := `{"type":"future_segment","data":{"x":1}}`
	seg, err := types.UnmarshalIncomingSegment([]byte(in))
	if err != nil {
		t.Fatalf("UnmarshalIncomingSegment: %v", err)
	}
	unknown, ok := seg.(*types.UnknownIncomingSegment)
	if !ok {
		t.Fatalf("段类型 = %T, want *types.UnknownIncomingSegment", seg)
	}
	if unknown.SegmentType() != "future_segment" {
		t.Fatalf("SegmentType = %q", unknown.SegmentType())
	}
	out, err := json.Marshal(seg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(out) != in {
		t.Fatalf("未知段未原样保留: %s", out)
	}
}

func TestSegmentsSkipInvalidElement(t *testing.T) {
	// 第二个元素缺少数值化 user_id（object），应被跳过而不是整体失败。
	in := `[{"type":"text","data":{"text":"a"}},{"type":"mention","data":{"user_id":{"oops":1}}},{"type":"text","data":{"text":"b"}}]`
	var segs types.IncomingSegments
	if err := json.Unmarshal([]byte(in), &segs); err != nil {
		t.Fatalf("容错解码失败: %v", err)
	}
	if got := segs.PlainText(); got != "ab" {
		t.Fatalf("PlainText = %q, want %q", got, "ab")
	}
	if len(segs) != 2 {
		t.Fatalf("段数量 = %d, want 2", len(segs))
	}
}

func TestIDFlexibleDecode(t *testing.T) {
	var v struct {
		A types.UserID     `json:"a"`
		B types.GroupID    `json:"b"`
		C types.Uin        `json:"c"`
		D types.MessageSeq `json:"d"`
	}
	// 字符串形式的数字必须被接受（兼容把 ID 序列化为字符串的实现）。
	if err := json.Unmarshal([]byte(`{"a":"123","b":456,"c":"789","d":10}`), &v); err != nil {
		t.Fatalf("宽容解码失败: %v", err)
	}
	if v.A != 123 || v.B != 456 || v.C != 789 || v.D != 10 {
		t.Fatalf("解码结果错误: %+v", v)
	}
	// 浮点数必须被拒绝。
	if err := json.Unmarshal([]byte(`{"a":1.5}`), &v); err == nil {
		t.Fatal("浮点数应被拒绝")
	}
	// 编码始终是 JSON 数字。
	out, err := json.Marshal(v.A)
	if err != nil || string(out) != "123" {
		t.Fatalf("Marshal = %s, %v", out, err)
	}
}

func TestRequestOmitOptionalAndDefaulted(t *testing.T) {
	// parent_folder_id 默认值为 "/"：零值应被省略，交由协议端应用默认值。
	raw, err := json.Marshal(types.UploadGroupFileRequest{
		GroupID:  1,
		FileURI:  "file:///tmp/a.txt",
		FileName: "a.txt",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "parent_folder_id") {
		t.Fatalf("默认值字段不应出现在请求体中: %s", raw)
	}
	if !strings.Contains(string(raw), `"file_uri":"file:///tmp/a.txt"`) {
		t.Fatalf("必需字段缺失: %s", raw)
	}

	// 显式设置时应正常序列化。
	parent := "/sub"
	raw, err = json.Marshal(types.UploadGroupFileRequest{GroupID: 1, ParentFolderID: "/sub", FileURI: "x", FileName: "y"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"parent_folder_id":"/sub"`) {
		t.Fatalf("显式默认值字段应被序列化: %s", raw)
	}
	_ = parent

	// 可选字段为 nil 时不出现。
	raw, err = json.Marshal(types.GetHistoryMessagesRequest{MessageScene: types.MessageSceneGroup, PeerID: 42})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "start_message_seq") || strings.Contains(string(raw), "limit") {
		t.Fatalf("可选字段应被省略: %s", raw)
	}
}

func TestEnumValid(t *testing.T) {
	if !types.SegmentText.Valid() || types.SegmentType("nope").Valid() {
		t.Fatal("SegmentType.Valid 行为错误")
	}
	if !types.MessageSceneGroup.Valid() || types.MessageScene("x").Valid() {
		t.Fatal("MessageScene.Valid 行为错误")
	}
	if got := types.GroupMemberRoleOwner.String(); got != "owner" {
		t.Fatalf("String = %q", got)
	}
}

func TestProtocolMetadata(t *testing.T) {
	if len(types.AllEndpoints) != 65 {
		t.Fatalf("端点数量 = %d, want 65", len(types.AllEndpoints))
	}
	if len(types.EndpointService) != len(types.AllEndpoints) {
		t.Fatalf("EndpointService 覆盖不全: %d vs %d", len(types.EndpointService), len(types.AllEndpoints))
	}
	if types.ProtocolVersion != "1.3" {
		t.Fatalf("ProtocolVersion = %q", types.ProtocolVersion)
	}
	for ep, since := range types.APISince {
		if since == "" {
			t.Fatalf("端点 %q 的 since 为空", ep)
		}
		if _, ok := types.EndpointService[ep]; !ok {
			t.Fatalf("APISince 中的端点 %q 未登记到 EndpointService", ep)
		}
	}
}
