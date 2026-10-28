package types

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// CQ 码转义规则（OneBot v11）：
//
//	&  -> &amp;
//	[  -> &#91;
//	]  -> &#93;
//	,  -> &#44;
func cqEscape(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '[':
			b.WriteString("&#91;")
		case ']':
			b.WriteString("&#93;")
		case ',':
			b.WriteString("&#44;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cqUnescape(s string) string {
	if s == "" || !strings.Contains(s, "&") {
		return s
	}
	// 顺序很重要：先解实体码，最后解 &amp;，避免二次解码。
	s = strings.ReplaceAll(s, "&#44;", ",")
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

// CQCode 把消息段编码为 CQ 码。
//
// 非标量 data（例如 node 段的 content 数组）会以 JSON 形式写入参数值，
// 这类段通常只在数组格式下有意义。
func (s Segment) CQCode() string {
	if s.Type == SegmentText {
		return cqEscape(s.String("text"))
	}
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("[CQ:")
	b.WriteString(string(s.Type))
	for _, k := range keys {
		b.WriteString(",")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(cqEscape(cqValue(s.Data[k])))
	}
	b.WriteString("]")
	return b.String()
}

func cqValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		return x.String()
	case float64:
		// 避免 1234567890123 被格式化成科学计数法。
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

// CQString 把整条消息编码为 CQ 码字符串。
func (m Message) CQString() string {
	var b strings.Builder
	for _, seg := range m {
		b.WriteString(seg.CQCode())
	}
	return b.String()
}

// ParseCQ 解析 CQ 码字符串。
//
// 解析是宽容的：无法识别的 `[CQ:...]` 片段会作为文本保留，不会中断整条消息。
func ParseCQ(s string) (Message, error) {
	if s == "" {
		return nil, nil
	}
	var msg Message
	for len(s) > 0 {
		idx := strings.Index(s, "[CQ:")
		if idx < 0 {
			msg = appendText(msg, cqUnescape(s))
			break
		}
		if idx > 0 {
			msg = appendText(msg, cqUnescape(s[:idx]))
		}
		rest := s[idx+len("[CQ:"):]
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			// 没有闭合，按普通文本处理。
			msg = appendText(msg, cqUnescape(s[idx:]))
			break
		}
		body := rest[:end]
		seg, ok := parseCQBody(body)
		if !ok {
			msg = appendText(msg, cqUnescape(s[idx:idx+len("[CQ:")+end+1]))
		} else {
			msg = append(msg, seg)
		}
		s = rest[end+1:]
	}
	return msg, nil
}

func appendText(msg Message, text string) Message {
	if text == "" {
		return msg
	}
	return append(msg, Segment{Type: SegmentText, Data: SegmentData{"text": text}})
}

func parseCQBody(body string) (Segment, bool) {
	if body == "" {
		return Segment{}, false
	}
	typeName := body
	params := ""
	if i := strings.IndexByte(body, ','); i >= 0 {
		typeName = body[:i]
		params = body[i+1:]
	}
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return Segment{}, false
	}
	data := SegmentData{}
	for params != "" {
		var pair string
		if i := strings.IndexByte(params, ','); i >= 0 {
			pair = params[:i]
			params = params[i+1:]
		} else {
			pair = params
			params = ""
		}
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		data[strings.TrimSpace(k)] = cqUnescape(v)
	}
	// text 段统一成 text 字段。
	if SegmentType(typeName) == SegmentText {
		if _, ok := data["text"]; !ok {
			data["text"] = ""
		}
	}
	return Segment{Type: SegmentType(typeName), Data: data}, true
}
