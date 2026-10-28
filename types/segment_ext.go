package types

import "strings"

// PlainText 拼接所有 text 段的内容，忽略其它类型。
func (s IncomingSegments) PlainText() string {
	var b strings.Builder
	for _, seg := range s {
		if t, ok := seg.(*IncomingTextSegment); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// PlainText 拼接所有 text 段的内容，忽略其它类型。
func (s OutgoingSegments) PlainText() string {
	var b strings.Builder
	for _, seg := range s {
		if t, ok := seg.(*OutgoingTextSegment); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// Mentioned 报告消息是否提及了 uid。
func (s IncomingSegments) Mentioned(uid UserID) bool {
	for _, seg := range s {
		if m, ok := seg.(*IncomingMentionSegment); ok && m.UserID == uid {
			return true
		}
	}
	return false
}

// MentionsAll 报告消息是否 @ 了全体成员。
func (s IncomingSegments) MentionsAll() bool {
	for _, seg := range s {
		if _, ok := seg.(*IncomingMentionAllSegment); ok {
			return true
		}
	}
	return false
}

// ReplySeq 返回第一个 reply 段所引用的消息序列号。
func (s IncomingSegments) ReplySeq() (MessageSeq, bool) {
	for _, seg := range s {
		if r, ok := seg.(*IncomingReplySegment); ok {
			return r.MessageSeq, true
		}
	}
	return 0, false
}

// Images 返回消息中的所有图片段。
func (s IncomingSegments) Images() []*IncomingImageSegment {
	var out []*IncomingImageSegment
	for _, seg := range s {
		if img, ok := seg.(*IncomingImageSegment); ok {
			out = append(out, img)
		}
	}
	return out
}

// Types 返回消息段的类型序列，用于调试与路由判断。
func (s IncomingSegments) Types() []SegmentType {
	out := make([]SegmentType, 0, len(s))
	for _, seg := range s {
		out = append(out, seg.SegmentType())
	}
	return out
}

// String 以紧凑形式描述消息段列表，便于日志排查（不保证可逆）。
func (s IncomingSegments) String() string {
	parts := make([]string, 0, len(s))
	for _, seg := range s {
		switch t := seg.(type) {
		case *IncomingTextSegment:
			parts = append(parts, t.Text)
		case *IncomingMentionSegment:
			parts = append(parts, "@"+t.UserID.String())
		case *IncomingMentionAllSegment:
			parts = append(parts, "@全体成员")
		case *IncomingReplySegment:
			parts = append(parts, "[reply:"+t.MessageSeq.String()+"]")
		case *IncomingImageSegment:
			parts = append(parts, "[image]")
		case *IncomingRecordSegment:
			parts = append(parts, "[record]")
		case *IncomingVideoSegment:
			parts = append(parts, "[video]")
		case *IncomingFileSegment:
			parts = append(parts, "[file:"+t.FileName+"]")
		case *IncomingForwardSegment:
			parts = append(parts, "[forward]")
		default:
			parts = append(parts, "["+seg.SegmentType().String()+"]")
		}
	}
	return strings.Join(parts, "")
}
