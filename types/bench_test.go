package types_test

import (
	"encoding/json"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

const benchEventJSON = `{"time":1,"self_id":100,"event_type":"message_receive","data":{
	"message_scene":"group","peer_id":987654321,"message_seq":23333,"sender_id":111222333,"time":1700000000,
	"segments":[
		{"type":"reply","data":{"message_seq":23332,"sender_id":111222333,"time":1699999999,"segments":[{"type":"text","data":{"text":"原消息"}}]}},
		{"type":"mention","data":{"user_id":100,"name":"bot"}},
		{"type":"text","data":{"text":" 你好，这是一条用于基准测试的消息"}},
		{"type":"image","data":{"resource_id":"r","temp_url":"https://example.com/a.png","width":100,"height":200,"summary":"[图片]","sub_type":"normal"}}
	],
	"group":{"group_id":987654321,"group_name":"测试群","member_count":10,"max_member_count":200},
	"group_member":{"user_id":111222333,"nickname":"群友","sex":"unknown","group_id":987654321,
		"card":"名片","title":"","level":3,"role":"member","join_time":1700000000,"last_sent_time":1700000001}}}`

func BenchmarkUnmarshalEvent(b *testing.B) {
	raw := []byte(benchEventJSON)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for i := 0; i < b.N; i++ {
		ev, err := types.UnmarshalEvent(raw)
		if err != nil {
			b.Fatalf("UnmarshalEvent: %v", err)
		}
		if ev.EventType() != types.EventMessageReceive {
			b.Fatal("事件类型错误")
		}
	}
}

func BenchmarkUnmarshalEventAndRead(b *testing.B) {
	raw := []byte(benchEventJSON)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ev, err := types.UnmarshalEvent(raw)
		if err != nil {
			b.Fatal(err)
		}
		recv := ev.(*types.MessageReceiveEvent)
		msg, ok := recv.GroupMessage()
		if !ok || !msg.Segments.Mentioned(100) {
			b.Fatal("事件内容错误")
		}
		_ = msg.PlainText()
	}
}

func BenchmarkMarshalOutgoingSegments(b *testing.B) {
	segs := types.OutgoingSegments{
		&types.OutgoingReplySegment{MessageSeq: 23332},
		&types.OutgoingMentionSegment{UserID: 100},
		&types.OutgoingTextSegment{Text: "你好，这是一条用于基准测试的消息"},
		&types.OutgoingImageSegment{URI: "file:///tmp/a.png"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(segs); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalIncomingSegments(b *testing.B) {
	raw := []byte(`[{"type":"reply","data":{"message_seq":1,"sender_id":2,"time":3,"segments":[]}},
		{"type":"mention","data":{"user_id":100,"name":"bot"}},
		{"type":"text","data":{"text":"你好"}},
		{"type":"image","data":{"resource_id":"r","temp_url":"u","width":1,"height":1,"summary":"","sub_type":"normal"}}]`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var segs types.IncomingSegments
		if err := json.Unmarshal(raw, &segs); err != nil {
			b.Fatal(err)
		}
		if len(segs) != 4 {
			b.Fatalf("段数量 = %d", len(segs))
		}
	}
}

func BenchmarkIDSemanticTypes(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var id types.UserID
		if err := json.Unmarshal([]byte(`"123456789"`), &id); err != nil {
			b.Fatal(err)
		}
	}
}
