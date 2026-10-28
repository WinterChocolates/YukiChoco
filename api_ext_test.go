package YukiChoco

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/mock"
	"github.com/WinterChocolates/YukiChoco/types"
)

// newSugarClient 返回一个指向假协议端、无需事件通道的客户端。
func newSugarClient(t *testing.T) (*Client, *mock.Server) {
	t.Helper()
	srv := mock.New(mock.WithToken("secret-token"))
	t.Cleanup(srv.Close)
	c, err := New(eventConfig(srv.URL(), func(cfg *Config) {
		cfg.EventSource = EventSourceNone
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, srv
}

func TestSendSugarMethods(t *testing.T) {
	c, srv := newSugarClient(t)
	srv.Handle("send_group_message", func(json.RawMessage) (any, error) {
		return map[string]any{"message_seq": 1, "time": 2}, nil
	})
	srv.Handle("send_private_message", func(json.RawMessage) (any, error) {
		return map[string]any{"message_seq": 3, "time": 4}, nil
	})

	ctx := context.Background()
	if _, err := c.SendGroupText(ctx, 200, "hi"); err != nil {
		t.Fatalf("SendGroupText: %v", err)
	}
	call, _ := srv.LastCall("send_group_message")
	if !strings.Contains(string(call.Body), `"group_id":200`) || !strings.Contains(string(call.Body), `"text":"hi"`) {
		t.Errorf("SendGroupText body = %s", call.Body)
	}

	if _, err := c.SendPrivateText(ctx, 300, "yo"); err != nil {
		t.Fatalf("SendPrivateText: %v", err)
	}
	call, _ = srv.LastCall("send_private_message")
	if !strings.Contains(string(call.Body), `"user_id":300`) {
		t.Errorf("SendPrivateText body = %s", call.Body)
	}

	if _, err := c.ReplyGroup(ctx, 200, 7, "re"); err != nil {
		t.Fatalf("ReplyGroup: %v", err)
	}
	call, _ = srv.LastCall("send_group_message")
	if !strings.Contains(string(call.Body), `"type":"reply"`) || !strings.Contains(string(call.Body), `"message_seq":7`) {
		t.Errorf("ReplyGroup body = %s", call.Body)
	}

	if _, err := c.ReplyPrivate(ctx, 300, 8, "re"); err != nil {
		t.Fatalf("ReplyPrivate: %v", err)
	}
	if _, err := c.SendGroupSegments(ctx, 200, &types.OutgoingTextSegment{Text: "s1"}); err != nil {
		t.Fatalf("SendGroupSegments: %v", err)
	}
	if _, err := c.SendPrivateSegments(ctx, 300, &types.OutgoingTextSegment{Text: "s2"}); err != nil {
		t.Fatalf("SendPrivateSegments: %v", err)
	}
}

func TestClientEventAccessors(t *testing.T) {
	c, _ := newSugarClient(t)

	var viaUse, viaOnEvent, viaOnce int64
	c.Use(event.Logging(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), slog.LevelDebug))
	c.OnEvent(func(context.Context, types.Event) error {
		atomic.AddInt64(&viaOnEvent, 1)
		return nil
	})
	Once(c, func(context.Context, *types.MessageRecallEvent) error {
		atomic.AddInt64(&viaOnce, 1)
		return nil
	})
	_ = viaUse

	ev, err := types.UnmarshalEvent([]byte(`{"time":1,"self_id":2,"event_type":"message_recall","data":{"message_scene":"group","peer_id":1,"message_seq":1,"sender_id":2,"operator_id":2,"display_suffix":""}}`))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	c.events.Dispatch(context.Background(), ev)

	waitUntil(t, 2*time.Second, func() bool {
		return atomic.LoadInt64(&viaOnEvent) == 1 && atomic.LoadInt64(&viaOnce) == 1
	}, "OnEvent / Once 分发")

	if c.Router() == nil || c.GroupRouter() == nil || c.Events() == nil {
		t.Fatal("事件访问器返回 nil")
	}
	if c.Connected() {
		t.Error("未 Start 时 Connected 应为 false")
	}
	if c.State() != StateDisconnected {
		t.Errorf("State = %v", c.State())
	}
}

func TestRawHookPanicAndErrorAreIsolated(t *testing.T) {
	c, _ := newSugarClient(t)
	var offCalls int64
	off := c.OnRaw(func(context.Context, []byte) error {
		panic("raw hook boom")
	})
	c.OnRaw(func(context.Context, []byte) error {
		atomic.AddInt64(&offCalls, 1)
		return errors.New("hook 返回错误只记录")
	})
	c.deliverRaw(context.Background(), []byte(`{}`))
	if offCalls != 1 {
		t.Fatalf("panic 的钩子不应影响其它钩子，offCalls = %d", offCalls)
	}
	off()
	c.deliverRaw(context.Background(), []byte(`{}`))
	if offCalls != 2 {
		t.Fatalf("注销后仍被调用: %d", offCalls)
	}
}

func TestStateChangeUnsubscribe(t *testing.T) {
	c, _ := newSugarClient(t)
	var calls int32
	off := c.OnStateChange(func(State) { atomic.AddInt32(&calls, 1) })
	off()
	c.setState(StateDialing)
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("注销后仍收到状态回调")
	}
}

func TestProtocolInfoBeforeInit(t *testing.T) {
	c, _ := newSugarClient(t)
	if _, ok := c.ProtocolInfo(); ok {
		t.Fatal("Init 前 ProtocolInfo 应为 false")
	}
	if c.SelfID() != 0 {
		t.Fatal("Init 前 SelfID 应为 0")
	}
}

func TestConfigStringAndLogValueRedacts(t *testing.T) {
	cfg := Config{
		BaseURL:     "http://127.0.0.1:6700",
		AccessToken: "super-secret",
		EventSource: EventSourceWS,
	}
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if strings.Contains(cfg.String(), "super-secret") {
		t.Fatalf("Config.String 泄露 token: %s", cfg.String())
	}
	if strings.Contains(cfg.LogValue().String(), "super-secret") {
		t.Fatalf("Config.LogValue 泄露 token: %s", cfg.LogValue().String())
	}
	if !strings.Contains(cfg.LogValue().String(), "***") {
		t.Errorf("LogValue 未显示脱敏标记: %s", cfg.LogValue().String())
	}

	// 未设置 token 时给出明确说明。
	if got := (Config{}).String(); !strings.Contains(got, "(未设置)") {
		t.Errorf("空 token 显示为 %s", got)
	}
}

func TestVersionAndUserAgent(t *testing.T) {
	if Version != "0.1.0" {
		t.Fatalf("Version = %q", Version)
	}
	c, _ := newSugarClient(t)
	ua := c.Config().UserAgent
	if !strings.Contains(ua, Version) || !strings.Contains(ua, types.ProtocolVersion) {
		t.Fatalf("UserAgent = %q", ua)
	}
}

func TestCallRawAndDecodeErrors(t *testing.T) {
	c, srv := newSugarClient(t)
	srv.Handle("get_login_info", func(json.RawMessage) (any, error) {
		return map[string]any{"uin": 1, "nickname": "n"}, nil
	})

	raw, err := c.CallRaw(context.Background(), "get_login_info", nil)
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if !strings.Contains(string(raw), `"uin":1`) {
		t.Fatalf("CallRaw = %s", raw)
	}

	// 响应 data 与目标类型不匹配时应报错（数字不能解码进 string）。
	var out struct {
		Uin string `json:"uin"`
	}
	err = c.Call(context.Background(), "get_login_info", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("err = %v", err)
	}
}
