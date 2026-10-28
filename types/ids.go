package types

import (
	"strconv"

	"github.com/WinterChocolates/YukiChoco/internal/jsonx"
)

// UserID 是 QQ 号。
//
// OneBot v11 规定为整数，但部分实现会序列化成字符串，解码时两种形式都接受。
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

// Int64 返回整数值。
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

// Int64 返回整数值。
func (id GroupID) Int64() int64 { return int64(id) }

// String 返回十进制字符串。
func (id GroupID) String() string { return strconv.FormatInt(int64(id), 10) }

// IsZero 报告是否为零值。
func (id GroupID) IsZero() bool { return id == 0 }

// MessageID 是消息 ID。
//
// 不同协议端的取值可能是负数、正数或很大的整数，统一用 int64 承载。
type MessageID int64

// UnmarshalJSON 实现宽容解码（数字或数字字符串）。
func (id *MessageID) UnmarshalJSON(b []byte) error {
	v, err := jsonx.DecodeInt64(b)
	if err != nil {
		return err
	}
	*id = MessageID(v)
	return nil
}

// MarshalJSON 以 JSON 数字形式编码。
func (id MessageID) MarshalJSON() ([]byte, error) { return jsonx.EncodeInt64(int64(id)), nil }

// Int64 返回整数值。
func (id MessageID) Int64() int64 { return int64(id) }

// String 返回十进制字符串。
func (id MessageID) String() string { return strconv.FormatInt(int64(id), 10) }

// IsZero 报告是否为零值。
func (id MessageID) IsZero() bool { return id == 0 }

// Flag 是请求事件的标识（set_friend_add_request / set_group_add_request 使用）。
//
// 协议规定为字符串，但个别实现返回数字，解码时统一转成字符串。
type Flag string

// UnmarshalJSON 实现宽容解码（字符串或数字）。
func (f *Flag) UnmarshalJSON(b []byte) error {
	s, err := jsonx.DecodeString(b)
	if err != nil {
		return err
	}
	*f = Flag(s)
	return nil
}

// MarshalJSON 以 JSON 字符串形式编码。
func (f Flag) MarshalJSON() ([]byte, error) { return jsonx.EncodeString(string(f)) }

// String 返回字符串值。
func (f Flag) String() string { return string(f) }
