package mock_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/mock"
)

func postJSON(t *testing.T, url, body string, header map[string]string) (int, map[string]any) {
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
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func TestAPIHandlerAndRecords(t *testing.T) {
	srv := mock.New()
	defer srv.Close()

	srv.Handle("get_login_info", func(req json.RawMessage) (any, error) {
		return map[string]any{"uin": 100, "nickname": "bot"}, nil
	})

	code, env := postJSON(t, srv.URL()+"/api/get_login_info", `{}`, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if env["status"] != "ok" || env["retcode"].(float64) != 0 {
		t.Fatalf("信封错误: %v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok || data["uin"].(float64) != 100 {
		t.Fatalf("data = %v", env["data"])
	}

	call, ok := srv.LastCall("get_login_info")
	if !ok {
		t.Fatal("未记录调用")
	}
	if string(call.Body) != `{}` {
		t.Errorf("记录的请求体 = %s", call.Body)
	}
	if len(srv.Calls()) != 1 {
		t.Errorf("调用记录数 = %d", len(srv.Calls()))
	}
}

func TestUnregisteredEndpointReturnsEmptySuccess(t *testing.T) {
	srv := mock.New()
	defer srv.Close()

	code, env := postJSON(t, srv.URL()+"/api/unknown_endpoint", `{}`, nil)
	if code != http.StatusOK || env["status"] != "ok" {
		t.Fatalf("未注册端点应返回空成功: %d %v", code, env)
	}

	// 兜底 handler 生效。
	srv.OnAnyAPI(func(json.RawMessage) (any, error) {
		return map[string]any{"from": "fallback"}, nil
	})
	_, env = postJSON(t, srv.URL()+"/api/another", `{}`, nil)
	if data, ok := env["data"].(map[string]any); !ok || data["from"] != "fallback" {
		t.Fatalf("兜底 handler 未生效: %v", env)
	}
}

func TestHandlerErrorsBecomeFailedEnvelope(t *testing.T) {
	srv := mock.New()
	defer srv.Close()

	srv.Handle("api_error", func(json.RawMessage) (any, error) {
		return nil, &mock.APIError{Retcode: -403, Message: "未登录"}
	})
	srv.Handle("generic_error", func(json.RawMessage) (any, error) {
		return nil, errors.New("内部错误")
	})

	_, env := postJSON(t, srv.URL()+"/api/api_error", `{}`, nil)
	if env["status"] != "failed" || env["retcode"].(float64) != -403 {
		t.Fatalf("APIError 信封错误: %v", env)
	}
	if env["message"] != "未登录" {
		t.Errorf("message = %v", env["message"])
	}

	_, env = postJSON(t, srv.URL()+"/api/generic_error", `{}`, nil)
	if env["status"] != "failed" || env["retcode"].(float64) != -500 {
		t.Fatalf("通用错误应映射为 -500: %v", env)
	}
}

func TestAuthAndMethodChecks(t *testing.T) {
	srv := mock.New(mock.WithToken("tok"))
	defer srv.Close()

	if code, _ := postJSON(t, srv.URL()+"/api/x", `{}`, nil); code != http.StatusUnauthorized {
		t.Errorf("无 token status = %d, want 401", code)
	}
	if code, _ := postJSON(t, srv.URL()+"/api/x", `{}`, map[string]string{"Authorization": "Bearer tok"}); code != http.StatusOK {
		t.Errorf("Header token status = %d, want 200", code)
	}
	if code, _ := postJSON(t, srv.URL()+"/api/x?access_token=tok", `{}`, nil); code != http.StatusOK {
		t.Errorf("query token status = %d, want 200", code)
	}

	// GET 不允许
	resp, err := http.Get(srv.URL() + "/api/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
	// 空端点
	srv2 := mock.New()
	defer srv2.Close()
	if code, _ := postJSON(t, srv2.URL()+"/api/", `{}`, nil); code != http.StatusNotFound {
		t.Errorf("空端点 status = %d, want 404", code)
	}
}

func TestPushRequiresClient(t *testing.T) {
	srv := mock.New()
	defer srv.Close()
	if err := srv.PushJSON([]byte(`{}`)); err == nil {
		t.Fatal("没有客户端时 Push 应报错")
	}
	if err := srv.Push(map[string]any{"a": 1}); err == nil {
		t.Fatal("没有客户端时 Push 应报错")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := srv.WaitConnected(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitConnected err = %v, want DeadlineExceeded", err)
	}
}

func TestSSEMode(t *testing.T) {
	srv := mock.New(mock.WithSSE())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL()+"/event", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /event: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.WaitConnected(ctx, 1); err != nil {
		t.Fatalf("WaitConnected: %v", err)
	}
	if err := srv.PushJSON([]byte(`{"event_type":"bot_offline"}`)); err != nil {
		t.Fatalf("PushJSON: %v", err)
	}

	buf := make([]byte, 512)
	deadline := time.Now().Add(2 * time.Second)
	var received string
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			received += string(buf[:n])
			if strings.Contains(received, `"bot_offline"`) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if !strings.Contains(received, "event: milky_event") || !strings.Contains(received, `"bot_offline"`) {
		t.Fatalf("SSE 输出不符合规范:\n%s", received)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	srv := mock.New()
	srv.Close()
	srv.Close()
}

func TestAPIErrorMessage(t *testing.T) {
	err := &mock.APIError{Retcode: -403, Message: "未登录"}
	if !strings.Contains(err.Error(), "-403") || !strings.Contains(err.Error(), "未登录") {
		t.Fatalf("Error() = %s", err.Error())
	}
}
