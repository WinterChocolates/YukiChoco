package types_test

import (
	"encoding/json"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

func BenchmarkParseCQ(b *testing.B) {
	raw := `[CQ:at,qq=123456] hello [CQ:image,file=http://example.com/a.png] [CQ:face,id=14]`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := types.ParseCQ(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageUnmarshalArray(b *testing.B) {
	raw := []byte(`[{"type":"text","data":{"text":"hello"}},{"type":"at","data":{"qq":"123"}},{"type":"image","data":{"file":"http://example.com/a.png"}}]`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var msg types.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseEvent(b *testing.B) {
	raw := []byte(`{"time":1700000000,"self_id":10000,"post_type":"message","message_type":"group","sub_type":"normal","message_id":1,"group_id":20001,"user_id":30001,"sender":{"user_id":30001,"nickname":"n","role":"member"},"message":[{"type":"text","data":{"text":"/echo hi"}}],"raw_message":"/echo hi","font":0}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := types.ParseEvent(raw); err != nil {
			b.Fatal(err)
		}
	}
}
