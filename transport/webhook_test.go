package transport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/transport"
)

// postEvent 向 WebHook handler 投递一个事件。
func postEvent(t *testing.T, url string, body string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestWebHookReceivesEvents(t *testing.T) {
	wh := transport.NewWebHook(transport.WebHookConfig{})
	srv := httptest.NewServer(wh.Handler())
	defer srv.Close()

	sess, err := wh.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer sess.Close()

	resp := postEvent(t, srv.URL, `{"event_type":"bot_offline"}`, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := sess.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if string(raw) != `{"event_type":"bot_offline"}` {
		t.Fatalf("Next = %s", raw)
	}
}

func TestWebHookAuthAndMethodChecks(t *testing.T) {
	wh := transport.NewWebHook(transport.WebHookConfig{AccessToken: "tok", VerifyToken: true})
	srv := httptest.NewServer(wh.Handler())
	defer srv.Close()

	// 无 token
	if resp := postEvent(t, srv.URL, `{}`, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("缺 token 时 status = %d, want 401", resp.StatusCode)
	}
	// 错误 token
	if resp := postEvent(t, srv.URL, `{}`, map[string]string{"Authorization": "Bearer bad"}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("错误 token 时 status = %d, want 401", resp.StatusCode)
	}
	// 正确 token
	if resp := postEvent(t, srv.URL, `{"a":1}`, map[string]string{"Authorization": "Bearer tok"}); resp.StatusCode != http.StatusNoContent {
		t.Errorf("正确 token 时 status = %d, want 204", resp.StatusCode)
	}
	// GET 不允许
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
	// 非 JSON Content-Type
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("x"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer tok")
	badCT, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer badCT.Body.Close()
	if badCT.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("错误 Content-Type 时 status = %d, want 415", badCT.StatusCode)
	}
}

func TestWebHookQueueFull(t *testing.T) {
	wh := transport.NewWebHook(transport.WebHookConfig{Buffer: 1})
	srv := httptest.NewServer(wh.Handler())
	defer srv.Close()

	// 没有消费者：第一条入队，第二条应被拒绝。
	if resp := postEvent(t, srv.URL, `{"n":1}`, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("第一条 status = %d", resp.StatusCode)
	}
	if resp := postEvent(t, srv.URL, `{"n":2}`, nil); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("第二条 status = %d, want 429", resp.StatusCode)
	}
	if got := wh.Dropped(); got != 1 {
		t.Errorf("Dropped = %d, want 1", got)
	}
}

func TestWebHookClose(t *testing.T) {
	wh := transport.NewWebHook(transport.WebHookConfig{})
	srv := httptest.NewServer(wh.Handler())
	defer srv.Close()

	sess, err := wh.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := wh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sess.Next(context.Background()); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("关闭后 Next err = %v, want ErrClosed", err)
	}
	if _, err := wh.Connect(context.Background()); !errors.Is(err, transport.ErrClosed) {
		t.Fatalf("关闭后 Connect err = %v, want ErrClosed", err)
	}
	if resp := postEvent(t, srv.URL, `{"a":1}`, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("关闭后 status = %d, want 503", resp.StatusCode)
	}
}

func TestWebHookName(t *testing.T) {
	if got := transport.NewWebHook(transport.WebHookConfig{}).Name(); got != transport.SourceWebHook {
		t.Fatalf("Name = %q", got)
	}
}
