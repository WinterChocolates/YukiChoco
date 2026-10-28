package YukiChoco

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/mock"
	"github.com/WinterChocolates/YukiChoco/types"
)

// mockImplInfo 是 mock 协议端的默认能力声明。
func mockImplInfo(json.RawMessage) (any, error) {
	return map[string]any{
		"impl_name":           "mock",
		"impl_version":        "0.0.1",
		"qq_protocol_version": "9.9.9",
		"qq_protocol_type":    "linux",
		"milky_version":       types.ProtocolVersion,
	}, nil
}

func mockLoginInfo(json.RawMessage) (any, error) {
	return map[string]any{"uin": 100, "nickname": "mock-bot"}, nil
}

// TestMockEndToEndWebSocket 用假协议端跑通「初始化 → 收事件 → 调 API」。
func TestMockEndToEndWebSocket(t *testing.T) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("get_impl_info", mockImplInfo)
	srv.Handle("get_login_info", mockLoginInfo)
	srv.Handle("send_group_message", func(json.RawMessage) (any, error) {
		return map[string]any{"message_seq": 42, "time": 1700000000}, nil
	})

	c, err := New(eventConfig(srv.URL(), nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	replied := make(chan struct{}, 1)
	On(c, func(ctx context.Context, ev *types.MessageReceiveEvent) error {
		msg, ok := ev.GroupMessage()
		if !ok {
			return nil
		}
		if _, err := c.Group().SendMessage(ctx, msg.Group.GroupID,
			&types.OutgoingReplySegment{MessageSeq: msg.MessageSeq},
			&types.OutgoingTextSegment{Text: "pong"},
		); err != nil {
			return err
		}
		select {
		case replied <- struct{}{}:
		default:
		}
		return nil
	})

	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if c.SelfID() != 100 {
		t.Fatalf("SelfID = %d", c.SelfID())
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitConnected(ctx, 1); err != nil {
		t.Fatalf("等待客户端连接: %v", err)
	}
	if err := srv.PushJSON([]byte(groupMessageJSON)); err != nil {
		t.Fatalf("PushJSON: %v", err)
	}

	select {
	case <-replied:
	case <-ctx.Done():
		t.Fatal("未收到事件")
	}

	call, ok := srv.LastCall("send_group_message")
	if !ok {
		t.Fatalf("mock 未记录 send_group_message；实际调用: %+v", srv.Calls())
	}
	if !strings.Contains(string(call.Body), `"group_id":200`) {
		t.Errorf("请求体缺少 group_id: %s", call.Body)
	}
	if !strings.Contains(string(call.Body), `"text":"pong"`) {
		t.Errorf("请求体缺少文本段: %s", call.Body)
	}

	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestMockEndToEndSSE 验证 SSE 事件源与分发链路。
func TestMockEndToEndSSE(t *testing.T) {
	srv := mock.New(mock.WithToken("secret-token"), mock.WithSSE())
	defer srv.Close()

	c, err := New(eventConfig(srv.URL(), func(cfg *Config) {
		cfg.EventSource = EventSourceSSE
		cfg.EventURL = srv.URL() + "/event"
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := make(chan string, 1)
	On(c, func(_ context.Context, ev *types.MessageReceiveEvent) error {
		if msg, ok := ev.GroupMessage(); ok {
			select {
			case got <- msg.PlainText():
			default:
			}
		}
		return nil
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.WaitConnected(ctx, 1); err != nil {
		t.Fatalf("等待 SSE 连接: %v", err)
	}
	if err := srv.PushJSON([]byte(groupMessageJSON)); err != nil {
		t.Fatalf("PushJSON: %v", err)
	}
	select {
	case text := <-got:
		if text != "你好" {
			t.Fatalf("PlainText = %q", text)
		}
	case <-ctx.Done():
		t.Fatal("SSE 未收到事件")
	}
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestWebhookEndToEnd 验证 WebHook 模式：外部 POST → EventHandler → 分发。
func TestWebhookEndToEnd(t *testing.T) {
	c, err := New(Config{
		BaseURL:            "http://127.0.0.1:1",
		AccessToken:        "secret-token",
		EventSource:        EventSourceWebhook,
		WebHookVerifyToken: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/milky/event", c.EventHandler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var received int32
	On(c, func(context.Context, *types.MessageReceiveEvent) error {
		atomic.AddInt32(&received, 1)
		return nil
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	post := func(token string) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/milky/event", strings.NewReader(groupMessageJSON))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := post("bad"); code != http.StatusUnauthorized {
		t.Fatalf("错误 token status = %d", code)
	}
	if code := post("secret-token"); code != http.StatusNoContent {
		t.Fatalf("正确 token status = %d", code)
	}

	waitUntil(t, 3*time.Second, func() bool { return atomic.LoadInt32(&received) == 1 }, "WebHook 事件分发")

	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// 关闭后 EventHandler 不再接受事件。
	if code := post("secret-token"); code != http.StatusServiceUnavailable {
		t.Errorf("Shutdown 后 status = %d, want 503", code)
	}
}

// TestWebhookHandlerRequiresConfig 验证未启用 WebHook 时 EventHandler 返回 501。
func TestWebhookHandlerRequiresConfig(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", EventSource: EventSourceNone})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	rec := httptest.NewRecorder()
	c.EventHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if c.WebhookDropped() != 0 {
		t.Fatalf("非 WebHook 模式的 dropped 应为 0")
	}
}

// TestUploadAndDownloadHelpers 验证文件上传/下载助手拼装与错误路径。
func TestUploadAndDownloadHelpers(t *testing.T) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("upload_group_file", func(req json.RawMessage) (any, error) {
		if !strings.Contains(string(req), `"file_uri":"base64://`) {
			// 走 reader/bytes 时应内联为 base64。
			return nil, &mock.APIError{Retcode: -400, Message: "file_uri 不是 base64"}
		}
		return map[string]any{"file_id": "f1"}, nil
	})
	srv.Handle("get_resource_temp_url", func(json.RawMessage) (any, error) {
		return map[string]any{"url": srv.URL() + "/blob"}, nil
	})

	c, err := New(eventConfig(srv.URL(), nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	res, err := c.UploadGroupFileWith(context.Background(), 200, "", "a.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("UploadGroupFileWith: %v", err)
	}
	if res.FileID != "f1" {
		t.Errorf("FileID = %q", res.FileID)
	}

	call, ok := srv.LastCall("upload_group_file")
	if !ok || !strings.Contains(string(call.Body), `"file_name":"a.txt"`) {
		t.Errorf("upload 请求体 = %s", call.Body)
	}

	// 下载：mock 的 /blob 不在 mux 上，应当返回 404 而不是 panic。
	if _, err := c.DownloadResource(context.Background(), "res-1"); err == nil {
		t.Fatal("下载不存在的资源应报错")
	} else if !errors.Is(err, ErrUnexpectedStatus) {
		t.Logf("下载错误（可接受）: %v", err)
	}
}
