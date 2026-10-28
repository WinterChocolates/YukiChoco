package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/WinterChocolates/YukiChoco/internal/jsonx"
)

// Message 是一条消息，由若干消息段组成。
//
// 解码时同时支持 OneBot 的数组格式与 CQ 码字符串格式；编码始终输出数组格式。
// 需要 CQ 码时用 CQString。
type Message []Segment

// MarshalJSON 以数组格式编码（nil 输出空数组，避免协议端把 null 当非法参数）。
func (m Message) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Segment(m))
}

// UnmarshalJSON 兼容数组格式、单个段对象与 CQ 码字符串。
func (m *Message) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	switch {
	case len(trimmed) == 0 || string(trimmed) == "null":
		*m = nil
		return nil
	case trimmed[0] == '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		parsed, err := ParseCQ(s)
		if err != nil {
			return err
		}
		*m = parsed
		return nil
	case trimmed[0] == '[':
		segs, err := jsonx.DecodeArray[Segment](trimmed, "types.Message")
		if err != nil {
			return err
		}
		*m = segs
		return nil
	case trimmed[0] == '{':
		// 个别实现把单段消息直接序列化成对象。
		var seg Segment
		if err := json.Unmarshal(trimmed, &seg); err != nil {
			return err
		}
		*m = Message{seg}
		return nil
	default:
		return fmt.Errorf("types: 无法解析消息（首字节 %q）", trimmed[0])
	}
}

// PlainText 返回所有 text 段拼接后的文本（忽略其它段）。
func (m Message) PlainText() string {
	if len(m) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range m {
		if seg.Type == SegmentText {
			b.WriteString(seg.String("text"))
		}
	}
	return b.String()
}

// String 返回纯文本内容，等价于 PlainText。
func (m Message) String() string { return m.PlainText() }

// OfType 返回指定类型的消息段。
func (m Message) OfType(t SegmentType) []Segment {
	var out []Segment
	for _, seg := range m {
		if seg.Type == t {
			out = append(out, seg)
		}
	}
	return out
}

// First 返回第一个指定类型的消息段。
func (m Message) First(t SegmentType) (Segment, bool) {
	for _, seg := range m {
		if seg.Type == t {
			return seg, true
		}
	}
	return Segment{}, false
}

// Contains 报告是否包含指定类型的消息段。
func (m Message) Contains(t SegmentType) bool {
	_, ok := m.First(t)
	return ok
}

// Mentioned 报告是否 @ 了指定用户（qq=all 视为 @全体成员，不计入）。
func (m Message) Mentioned(userID UserID) bool {
	for _, seg := range m.OfType(SegmentAt) {
		if seg.String("qq") == userID.String() {
			return true
		}
	}
	return false
}

// MentionsAll 报告是否 @ 了全体成员。
func (m Message) MentionsAll() bool {
	for _, seg := range m.OfType(SegmentAt) {
		if seg.String("qq") == "all" {
			return true
		}
	}
	return false
}

// ReplyID 返回 reply 段引用的消息 ID；没有则返回 0。
func (m Message) ReplyID() MessageID {
	seg, ok := m.First(SegmentReply)
	if !ok {
		return 0
	}
	return MessageID(seg.Int64("id"))
}
