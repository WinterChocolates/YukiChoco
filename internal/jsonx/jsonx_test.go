package jsonx_test

import (
	"errors"
	"testing"

	"github.com/WinterChocolates/YukiChoco/internal/jsonx"
)

func TestMarshalTagged(t *testing.T) {
	type textData struct {
		Text string `json:"text"`
	}
	raw, err := jsonx.MarshalTagged("text", textData{Text: "hi"})
	if err != nil {
		t.Fatalf("MarshalTagged: %v", err)
	}
	if string(raw) != `{"type":"text","data":{"text":"hi"}}` {
		t.Fatalf("MarshalTagged = %s", raw)
	}

	// nil payload 视为空对象。
	raw, err = jsonx.MarshalTagged("mention_all", nil)
	if err != nil {
		t.Fatalf("MarshalTagged(nil): %v", err)
	}
	if string(raw) != `{"type":"mention_all","data":{}}` {
		t.Fatalf("MarshalTagged(nil) = %s", raw)
	}

	// 非对象 payload 必须报错，避免生成非法消息段。
	if _, err := jsonx.MarshalTagged("x", "string-payload"); err == nil {
		t.Fatal("非对象 payload 应报错")
	}
}

func TestTaggedPeek(t *testing.T) {
	got, err := jsonx.TaggedPeek([]byte(`{"type":"text","data":{}}`), "type")
	if err != nil {
		t.Fatalf("TaggedPeek: %v", err)
	}
	if got != "text" {
		t.Fatalf("TaggedPeek = %q", got)
	}

	if _, err := jsonx.TaggedPeek([]byte(`{"data":{}}`), "type"); err == nil {
		t.Fatal("缺少字段应报错")
	}
	if _, err := jsonx.TaggedPeek([]byte(`{"type":1}`), "type"); err == nil {
		t.Fatal("非字符串字段应报错")
	}
	if _, err := jsonx.TaggedPeek([]byte(`not json`), "type"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestTaggedData(t *testing.T) {
	data, err := jsonx.TaggedData([]byte(`{"type":"text","data":{"text":"hi"}}`))
	if err != nil {
		t.Fatalf("TaggedData: %v", err)
	}
	if string(data) != `{"text":"hi"}` {
		t.Fatalf("TaggedData = %s", data)
	}

	// data 缺失或为 null 时返回空对象，兼容省略空数据的实现。
	for _, in := range []string{`{"type":"mention_all"}`, `{"type":"mention_all","data":null}`} {
		data, err := jsonx.TaggedData([]byte(in))
		if err != nil {
			t.Fatalf("TaggedData(%s): %v", in, err)
		}
		if string(data) != `{}` {
			t.Fatalf("TaggedData(%s) = %s, want {}", in, data)
		}
	}

	if _, err := jsonx.TaggedData([]byte(`[]`)); err == nil {
		t.Fatal("非对象 JSON 应报错")
	}
}

func TestDecodeInt64(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{`123`, 123, false},
		{`-9`, -9, false},
		{`"456"`, 456, false},
		{`""`, 0, false},
		{`null`, 0, false},
		{``, 0, false},
		{`1.5`, 0, true},
		{`"1.5"`, 0, true},
		{`"abc"`, 0, true},
		{`{"a":1}`, 0, true},
	}
	for _, tc := range cases {
		got, err := jsonx.DecodeInt64([]byte(tc.in))
		if tc.wantErr {
			if err == nil {
				t.Errorf("DecodeInt64(%s) 期望报错", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("DecodeInt64(%s): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("DecodeInt64(%s) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestEncodeInt64(t *testing.T) {
	if got := string(jsonx.EncodeInt64(-42)); got != "-42" {
		t.Fatalf("EncodeInt64 = %s", got)
	}
}

func TestDecodeErrorHook(t *testing.T) {
	type record struct {
		err error
		ctx string
	}
	var got []record
	jsonx.SetDecodeErrorHook(func(err error, ctx string) {
		got = append(got, record{err: err, ctx: ctx})
	})
	t.Cleanup(func() { jsonx.SetDecodeErrorHook(nil) })

	jsonx.Report(errors.New("boom"), "IncomingSegments[3]")
	jsonx.Report(nil, "should-be-ignored")

	if len(got) != 1 {
		t.Fatalf("hook 调用次数 = %d, want 1", len(got))
	}
	if got[0].ctx != "IncomingSegments[3]" || got[0].err == nil {
		t.Fatalf("hook 参数错误: %+v", got[0])
	}
}
