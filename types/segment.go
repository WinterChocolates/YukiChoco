package types

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SegmentType 是消息段类型。
type SegmentType string

// OneBot v11 标准消息段与常见扩展。
const (
	SegmentText      SegmentType = "text"
	SegmentFace      SegmentType = "face"
	SegmentImage     SegmentType = "image"
	SegmentRecord    SegmentType = "record"
	SegmentVideo     SegmentType = "video"
	SegmentAt        SegmentType = "at"
	SegmentRPS       SegmentType = "rps"
	SegmentDice      SegmentType = "dice"
	SegmentShake     SegmentType = "shake"
	SegmentPoke      SegmentType = "poke"
	SegmentAnonymous SegmentType = "anonymous"
	SegmentShare     SegmentType = "share"
	SegmentContact   SegmentType = "contact"
	SegmentLocation  SegmentType = "location"
	SegmentMusic     SegmentType = "music"
	SegmentReply     SegmentType = "reply"
	SegmentForward   SegmentType = "forward"
	SegmentNode      SegmentType = "node"
	SegmentXML       SegmentType = "xml"
	SegmentJSON      SegmentType = "json"
	SegmentCard      SegmentType = "card"
	SegmentTTS       SegmentType = "tts"
	SegmentMFace     SegmentType = "mface"
	SegmentFile      SegmentType = "file"
	SegmentMarkdown  SegmentType = "markdown"
	SegmentKeyboard  SegmentType = "keyboard"
)

// Segment 是一个消息段。
//
// Data 使用 map[string]any 而不是 map[string]string，原因是 OneBot 实现之间
// 对 data 值的类型并不一致（user_id 可能是数字，node 的 content 是数组）；
// 保留原始类型才能无损失地转发。取值请用 String / Int64 / Bool 等助手。
type Segment struct {
	Type SegmentType `json:"type"`
	Data SegmentData `json:"data"`
}

// SegmentData 是消息段数据。
type SegmentData map[string]any

// NewSegment 构造一个消息段。
func NewSegment(t SegmentType, data map[string]any) Segment {
	return Segment{Type: t, Data: data}
}

// Get 返回原始值。
func (s Segment) Get(key string) (any, bool) {
	v, ok := s.Data[key]
	return v, ok
}

// String 返回字符串值；数字与布尔会被转换成字符串。
func (s Segment) String(key string) string {
	switch v := s.Data[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// Int64 返回整数值；无法转换时返回 0。
func (s Segment) Int64(key string) int64 {
	v := s.Data[key]
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int:
		return int64(n)
	case int64:
		return n
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	default:
		return 0
	}
}

// Bool 返回布尔值；无法转换时返回 false。
func (s Segment) Bool(key string) bool {
	switch v := s.Data[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	case float64:
		return v != 0
	default:
		return false
	}
}

// Is 报告是否是某种消息段。
func (s Segment) Is(t SegmentType) bool { return s.Type == t }

// MarshalJSON 保证 data 字段始终存在（部分协议端对缺失 data 会报参数错误）。
func (s Segment) MarshalJSON() ([]byte, error) {
	type alias Segment
	if s.Data == nil {
		s.Data = SegmentData{}
	}
	return json.Marshal(alias(s))
}

// UnmarshalJSON 兼容 data 缺失或为 null 的消息段。
func (s *Segment) UnmarshalJSON(b []byte) error {
	var raw struct {
		Type SegmentType     `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	s.Type = raw.Type
	s.Data = SegmentData{}
	if len(raw.Data) == 0 || string(raw.Data) == "null" {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return err
	}
	if data != nil {
		s.Data = data
	}
	return nil
}
