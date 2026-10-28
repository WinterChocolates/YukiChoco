package YukiChoco

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/types"
)

func TestIdempotentActions(t *testing.T) {
	c := &Client{}
	for _, action := range []string{"get_status", "get_group_list", "can_send_image"} {
		if !c.idempotent(action) {
			t.Errorf("%s 应可重试", action)
		}
	}
	for _, action := range []string{"send_group_msg", "delete_msg", "set_group_ban", "_send_group_notice"} {
		if c.idempotent(action) {
			t.Errorf("%s 不应默认重试", action)
		}
	}
}

func TestRetryDelayAndBackoff(t *testing.T) {
	c := &Client{cfg: Config{
		Retry: RetryConfig{BaseDelay: 100 * time.Millisecond, MaxDelay: 500 * time.Millisecond},
		ForwardWS: ForwardWSConfig{Reconnect: ReconnectConfig{
			InitialDelay: 100 * time.Millisecond, MaxDelay: 400 * time.Millisecond,
			Multiplier: 2, Jitter: -1,
		}},
	}}
	if got := c.retryDelay(1); got != 100*time.Millisecond {
		t.Errorf("retryDelay(1)=%v", got)
	}
	if got := c.retryDelay(3); got != 400*time.Millisecond {
		t.Errorf("retryDelay(3)=%v", got)
	}
	if got := c.retryDelay(10); got != 500*time.Millisecond {
		t.Errorf("retryDelay(10)=%v", got)
	}
	if got := c.backoff(1); got != 100*time.Millisecond {
		t.Errorf("backoff(1)=%v", got)
	}
	if got := c.backoff(10); got != 400*time.Millisecond {
		t.Errorf("backoff(10)=%v", got)
	}
}

func TestMarshalParams(t *testing.T) {
	for _, in := range []any{nil, (*struct{})(nil), struct{}{}} {
		b, err := marshalParams(in)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "{}" {
			t.Errorf("%T: %s", in, b)
		}
	}
	b, err := marshalParams(map[string]any{"x": 1})
	if err != nil || string(b) != `{"x":1}` {
		t.Fatalf("%s %v", b, err)
	}
}

func TestSafeURL(t *testing.T) {
	cases := map[string]string{
		"ws://user:pass@host:1/ws?access_token=secret&x=1": "ws://host:1/ws?access_token=%2A%2A%2A&x=1",
		"http://host/a?token=abc":                          "http://host/a?token=%2A%2A%2A",
		"":                                                 "",
	}
	for in, want := range cases {
		if got := safeURL(in); got != want {
			t.Errorf("safeURL(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet([]byte("  short  ")); got != "short" {
		t.Errorf("snippet=%q", got)
	}
	long := strings.Repeat("x", 600)
	if len(snippet([]byte(long))) != 515 {
		t.Errorf("长响应截断长度=%d", len(snippet([]byte(long))))
	}
}

func TestCheckEnvelope(t *testing.T) {
	if err := checkEnvelope("x", envelope{Status: "ok", Retcode: 0}, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkEnvelope("x", envelope{Status: "async", Retcode: 1}, nil); err != nil {
		t.Fatal(err)
	}
	err := checkEnvelope("x", envelope{Status: "failed", Retcode: 103, Data: json.RawMessage("null")}, []byte("{}"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Action != "x" || apiErr.Snippet != "{}" {
		t.Fatalf("apiErr=%+v", apiErr)
	}
}

func TestDecodeData(t *testing.T) {
	var out types.SendMsgResult
	if err := decodeData("x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if err := decodeData("x", json.RawMessage(`{"message_id":3}`), &out); err != nil || out.MessageID != 3 {
		t.Fatalf("%+v %v", out, err)
	}
	if err := decodeData("x", json.RawMessage(`bad`), &out); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
