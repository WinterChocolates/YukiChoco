package types

import (
	"strconv"

	"github.com/WinterChocolates/YukiChoco/internal/jsonx"
)

// UserID 是 QQ 号。
//
// 协议规定为 int64。解码时额外容忍以字符串形式给出的数字，以兼容把 ID
// 序列化为字符串的协议端实现；编码始终输出 JSON 数字。
type UserID int64

// UnmarshalJSON 实现宽容解码（数字或数字字符串）。
func (id *UserID) UnmarshalJSON(b []byte) error {
	v, err := jsonx.DecodeInt64(b)
	if err != nil {
		return err
	}
	*id = UserID(v)
	return nil
}

// MarshalJSON 以 JSON 数字形式编码。
func (id UserID) MarshalJSON() ([]byte, error) { return jsonx.EncodeInt64(int64(id)), nil }

// Int64 返回其整数值。
func (id UserID) Int64() int64 { return int64(id) }

// String 返回十进制字符串。
func (id UserID) String() string { return strconv.FormatInt(int64(id), 10) }

// IsZero 报告是否为零值。
func (id UserID) IsZero() bool { return id == 0 }

// GroupID 是群号。
type GroupID int64

// UnmarshalJSON 实现宽容解码（数字或数字字符串）。
func (id *GroupID) UnmarshalJSON(b []byte) error {
	v, err := jsonx.DecodeInt64(b)
	if err != nil {
		return err
	}
	*id = GroupID(v)
	return nil
}

// MarshalJSON 以 JSON 数字形式编码。
func (id GroupID) MarshalJSON() ([]byte, error) { return jsonx.EncodeInt64(int64(id)), nil }

// Int64 返回其整数值。
func (id GroupID) Int64() int64 { return int64(id) }

// String 返回十进制字符串。
func (id GroupID) String() string { return strconv.FormatInt(int64(id), 10) }

// IsZero 报告是否为零值。
func (id GroupID) IsZero() bool { return id == 0 }

// Uin 是语义上不明确的号码（协议中同时可能是 QQ 号或群号，例如 peer_id）。
type Uin int64

// UnmarshalJSON 实现宽容解码（数字或数字字符串）。
func (u *Uin) UnmarshalJSON(b []byte) error {
	v, err := jsonx.DecodeInt64(b)
	if err != nil {
		return err
	}
	*u = Uin(v)
	return nil
}

// MarshalJSON 以 JSON 数字形式编码。
func (u Uin) MarshalJSON() ([]byte, error) { return jsonx.EncodeInt64(int64(u)), nil }

// Int64 返回其整数值。
func (u Uin) Int64() int64 { return int64(u) }

// String 返回十进制字符串。
func (u Uin) String() string { return strconv.FormatInt(int64(u), 10) }

// IsZero 报告是否为零值。
func (u Uin) IsZero() bool { return u == 0 }

// MessageSeq 是消息序列号（同一会话内单调递增）。
type MessageSeq int64

// UnmarshalJSON 实现宽容解码（数字或数字字符串）。
func (s *MessageSeq) UnmarshalJSON(b []byte) error {
	v, err := jsonx.DecodeInt64(b)
	if err != nil {
		return err
	}
	*s = MessageSeq(v)
	return nil
}

// MarshalJSON 以 JSON 数字形式编码。
func (s MessageSeq) MarshalJSON() ([]byte, error) { return jsonx.EncodeInt64(int64(s)), nil }

// Int64 返回其整数值。
func (s MessageSeq) Int64() int64 { return int64(s) }

// String 返回十进制字符串。
func (s MessageSeq) String() string { return strconv.FormatInt(int64(s), 10) }

// IsZero 报告是否为零值。
func (s MessageSeq) IsZero() bool { return s == 0 }
