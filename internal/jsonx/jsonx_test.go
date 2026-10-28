package jsonx_test

import (
	"encoding/json"
	"testing"

	"github.com/WinterChocolates/YukiChoco/internal/jsonx"
)

func TestDecodeInt64(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{`123`, 123},
		{`"123"`, 123},
		{`-42`, -42},
		{`123.0`, 123},
		{`null`, 0},
		{`""`, 0},
	}
	for _, c := range cases {
		got, err := jsonx.DecodeInt64([]byte(c.in))
		if err != nil {
			t.Fatalf("DecodeInt64(%s): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("DecodeInt64(%s) = %d, want %d", c.in, got, c.want)
		}
	}
	if _, err := jsonx.DecodeInt64([]byte(`1.5`)); err == nil {
		t.Error("1.5 应被拒绝")
	}
	if _, err := jsonx.DecodeInt64([]byte(`"abc"`)); err == nil {
		t.Error("abc 应被拒绝")
	}
}

func TestDecodeString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"abc"`, "abc"},
		{`123`, "123"},
		{`true`, "true"},
		{`null`, ""},
	}
	for _, c := range cases {
		got, err := jsonx.DecodeString([]byte(c.in))
		if err != nil {
			t.Fatalf("DecodeString(%s): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("DecodeString(%s) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := jsonx.DecodeString([]byte(`{"a":1}`)); err == nil {
		t.Error("对象应被拒绝")
	}
}

func TestDecodeArraySkipsInvalid(t *testing.T) {
	type item struct {
		N int `json:"n"`
	}
	got, err := jsonx.DecodeArray[item]([]byte(`[{"n":1},{"n":"x"},{"n":3}]`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].N != 1 || got[1].N != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestEncode(t *testing.T) {
	if string(jsonx.EncodeInt64(42)) != "42" {
		t.Error("EncodeInt64")
	}
	b, err := jsonx.EncodeString("a\"b")
	if err != nil {
		t.Fatal(err)
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil || s != "a\"b" {
		t.Fatalf("EncodeString: %s %v", b, err)
	}
}
