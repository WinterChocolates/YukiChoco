package types_test

import (
	"encoding/json"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

func TestIDsTolerantDecode(t *testing.T) {
	var v struct {
		User  types.UserID    `json:"user_id"`
		Group types.GroupID   `json:"group_id"`
		Msg   types.MessageID `json:"message_id"`
		Flag  types.Flag      `json:"flag"`
	}
	raw := `{"user_id":"10001","group_id":20002,"message_id":"-3","flag":12345}`
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if v.User != 10001 || v.Group != 20002 || v.Msg != -3 || v.Flag.String() != "12345" {
		t.Fatalf("got %+v", v)
	}
}

func TestMessageDecodeArray(t *testing.T) {
	var msg types.Message
	raw := `[{"type":"text","data":{"text":"hi"}},{"type":"at","data":{"qq":123}},{"type":"image","data":{"file":"a.png","sub_type":1}}]`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg) != 3 {
		t.Fatalf("len=%d", len(msg))
	}
	if msg.PlainText() != "hi" {
		t.Errorf("PlainText=%q", msg.PlainText())
	}
	if !msg.Mentioned(123) {
		t.Error("应 @ 123")
	}
	seg := msg[2]
	if seg.String("file") != "a.png" || seg.Int64("sub_type") != 1 {
		t.Errorf("segment=%+v", seg)
	}
}

func TestMessageDecodeCQString(t *testing.T) {
	var msg types.Message
	raw := `"[CQ:at,qq=123] hello [CQ:image,file=http://a/b.png]"`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg) != 3 {
		t.Fatalf("len=%d: %+v", len(msg), msg)
	}
	if msg[0].Type != types.SegmentAt || msg[0].String("qq") != "123" {
		t.Errorf("at=%+v", msg[0])
	}
	if msg.PlainText() != " hello " {
		t.Errorf("text=%q", msg.PlainText())
	}
	if msg[2].Type != types.SegmentImage {
		t.Errorf("image=%+v", msg[2])
	}
}

func TestMessageDecodeSkipsBadSegment(t *testing.T) {
	var msg types.Message
	raw := `[{"type":"text","data":{"text":"a"}},42,{"type":"text","data":{"text":"b"}}]`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg) != 2 || msg.PlainText() != "ab" {
		t.Fatalf("got %+v", msg)
	}
}

func TestMessageEncodeIsArray(t *testing.T) {
	msg := types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "hi"}}}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[{"type":"text","data":{"text":"hi"}}]` {
		t.Fatalf("got %s", raw)
	}

	var nilMsg types.Message
	raw, err = json.Marshal(nilMsg)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `[]` {
		t.Fatalf("nil message = %s", raw)
	}
}

func TestCQCodec(t *testing.T) {
	msg := types.Message{
		{Type: types.SegmentText, Data: types.SegmentData{"text": "a[b],c&d"}},
		{Type: types.SegmentAt, Data: types.SegmentData{"qq": "42"}},
		{Type: types.SegmentImage, Data: types.SegmentData{"file": "http://a/b.png"}},
	}
	cq := msg.CQString()
	want := `a&#91;b&#93;&#44;c&amp;d[CQ:at,qq=42][CQ:image,file=http://a/b.png]`
	if cq != want {
		t.Fatalf("cq=%q\nwant=%q", cq, want)
	}
	back, err := types.ParseCQ(cq)
	if err != nil {
		t.Fatal(err)
	}
	if back.PlainText() != "a[b],c&d" {
		t.Errorf("text=%q", back.PlainText())
	}
	if len(back) != 3 || back[1].String("qq") != "42" {
		t.Errorf("back=%+v", back)
	}
}

func TestParseCQTolerant(t *testing.T) {
	// 未闭合的 CQ 码按文本保留，不报错。
	msg, err := types.ParseCQ("x[CQ:at,qq=1 y")
	if err != nil {
		t.Fatal(err)
	}
	if msg.PlainText() != "x[CQ:at,qq=1 y" {
		t.Fatalf("got %q", msg.PlainText())
	}
	// 残缺参数被忽略。
	msg, err = types.ParseCQ("[CQ:at,qq=1,broken]")
	if err != nil {
		t.Fatal(err)
	}
	if len(msg) != 1 || msg[0].String("qq") != "1" {
		t.Fatalf("got %+v", msg)
	}
}

func TestMessageHelpers(t *testing.T) {
	msg := types.Message{
		{Type: types.SegmentReply, Data: types.SegmentData{"id": "99"}},
		{Type: types.SegmentAt, Data: types.SegmentData{"qq": "all"}},
	}
	if msg.ReplyID() != 99 {
		t.Errorf("ReplyID=%d", msg.ReplyID())
	}
	if !msg.MentionsAll() {
		t.Error("MentionsAll=false")
	}
	if msg.Mentioned(1) {
		t.Error("Mentioned(1)=true")
	}
}

func TestSegmentMarshalAlwaysHasData(t *testing.T) {
	seg := types.Segment{Type: types.SegmentDice}
	raw, err := json.Marshal(seg)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"type":"dice","data":{}}` {
		t.Fatalf("got %s", raw)
	}
}
