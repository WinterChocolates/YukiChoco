// Package jsonx 提供 OneBot v11 编解码所需的容错工具。
//
// OneBot 实现之间对数字/字符串、消息段 data 类型、可选字段的处理并不一致，
// SDK 的解码路径必须宽容：未知字段忽略、非法数组元素跳过、未知联合类型保留原始 JSON。
package jsonx

import (
	"encoding/json"
	"fmt"
	"math"
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

// DecodeInt64 解码 JSON 整数，同时容忍字符串形式的数字与整数值浮点数。
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
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("jsonx: %q 不是合法的整数: %w", s, err)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, fmt.Errorf("jsonx: %q 不是整数", s)
	}
	return int64(f), nil
}

// EncodeInt64 以 JSON 数字形式编码整数。
func EncodeInt64(v int64) []byte {
	return strconv.AppendInt(nil, v, 10)
}

// DecodeString 解码字符串，同时容忍数字与布尔（转成其字面量形式）。
func DecodeString(b []byte) (string, error) {
	s := strings.TrimSpace(string(b))
	switch s {
	case "", "null":
		return "", nil
	}
	if s[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return "", fmt.Errorf("jsonx: 解析字符串失败: %w", err)
		}
		return v, nil
	}
	if s[0] == '{' || s[0] == '[' {
		return "", fmt.Errorf("jsonx: 期望标量，实际为 %s", s)
	}
	return s, nil
}

// EncodeString 以 JSON 字符串形式编码。
func EncodeString(s string) ([]byte, error) { return json.Marshal(s) }

// DecodeArray 逐个解码数组元素，跳过非法元素而不是整体失败。
//
// context 仅用于错误上报，便于定位是哪个字段的数据有问题。
func DecodeArray[T any](b []byte, context string) ([]T, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(b, &raws); err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raws))
	for i, raw := range raws {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			Report(fmt.Errorf("jsonx: 第 %d 个元素解码失败: %w", i, err), context)
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
