// Package jsonx 提供 Milky 协议编解码所需的容错工具。
//
// Milky 的兼容性契约允许协议端在响应中增加字段、在数组中放入 SDK 尚未识别的元素，
// 因此 SDK 的解码路径必须"宽容"：未知字段忽略、非法数组元素跳过、未知联合类型原样保留。
// 该包集中实现这些规则，避免各处重复。
package jsonx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// OnDecodeError 在解码过程中遇到"可跳过"的非法数据时被调用。
//
// 默认 nil，表示静默跳过（事件流不因单个坏元素中断）。
// 上层可通过 SetDecodeErrorHook 接入 slog 等日志设施。
var decodeErrorHook func(err error, context string)

// SetDecodeErrorHook 安装解码错误回调。
func SetDecodeErrorHook(fn func(err error, context string)) { decodeErrorHook = fn }

// Report 上报一个可跳过的解码错误（未安装回调时静默）。
func Report(err error, context string) {
	if decodeErrorHook != nil && err != nil {
		decodeErrorHook(err, context)
	}
}

// MarshalTagged 将 payload 包装为 {"type": tag, "data": payload}。
//
// payload 必须序列化为 JSON 对象；无字段的消息段（如 mention_all）传空结构体即可。
func MarshalTagged(tag string, payload any) ([]byte, error) {
	if payload == nil {
		payload = struct{}{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("jsonx: 序列化 %q 的消息段数据失败: %w", tag, err)
	}
	// 无字段的消息段（如 mention_all）可能以 nil 指针传入。
	if string(data) == "null" {
		data = []byte("{}")
	}
	if len(data) == 0 || data[0] != '{' {
		return nil, fmt.Errorf("jsonx: %q 的消息段数据必须是 JSON 对象，实际为 %s", tag, data)
	}
	tagJSON, err := json.Marshal(tag)
	if err != nil {
		return nil, fmt.Errorf("jsonx: 序列化标记 %q 失败: %w", tag, err)
	}
	out := make([]byte, 0, len(tagJSON)+len(data)+24)
	out = append(out, `{"type":`...)
	out = append(out, tagJSON...)
	out = append(out, `,"data":`...)
	out = append(out, data...)
	out = append(out, '}')
	return out, nil
}

// TaggedPeek 读取 JSON 对象中 tagField 的字符串值（用于判别联合类型）。
func TaggedPeek(b []byte, tagField string) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return "", fmt.Errorf("jsonx: 解析 %s 字段所在的 JSON 失败: %w", tagField, err)
	}
	raw, ok := m[tagField]
	if !ok {
		return "", fmt.Errorf("jsonx: 缺少判别字段 %q", tagField)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("jsonx: 判别字段 %q 不是字符串: %w", tagField, err)
	}
	return s, nil
}

// TaggedData 取出 JSON 对象中的 data 子对象。
//
// data 缺失或为 null 时返回空对象 {}，以兼容省略空数据的协议端实现。
func TaggedData(b []byte) (json.RawMessage, error) {
	var holder struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &holder); err != nil {
		return nil, fmt.Errorf("jsonx: 提取 data 失败: %w", err)
	}
	if len(holder.Data) == 0 || string(holder.Data) == "null" {
		return json.RawMessage("{}"), nil
	}
	return holder.Data, nil
}

// DecodeInt64 解码 JSON 整数，同时容忍以字符串形式给出的数字（如 "123456789"）。
//
// 浮点数（如 12345.0）视为非法：Milky 规范中 ID 与 seq 均为整数。
func DecodeInt64(b []byte) (int64, error) {
	s := strings.TrimSpace(string(b))
	switch s {
	case "", "null":
		return 0, nil
	}
	if s[0] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return 0, fmt.Errorf("jsonx: 解析整数字符串失败: %w", err)
		}
		s = strings.TrimSpace(unquoted)
		if s == "" {
			return 0, nil
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("jsonx: %q 不是合法的整数: %w", s, err)
	}
	return v, nil
}

// EncodeInt64 以 JSON 数字形式编码整数。
func EncodeInt64(v int64) []byte {
	return strconv.AppendInt(nil, v, 10)
}
