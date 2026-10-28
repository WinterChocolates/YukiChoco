package YukiChoco_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/mock"
	"github.com/WinterChocolates/YukiChoco/types"
)

func testGroupEvent(self, group, user types.UserID, text string) *types.GroupMessageEvent {
	return &types.GroupMessageEvent{MessageEvent: types.MessageEvent{
		EventBase:   types.EventBase{Time: time.Now().Unix(), SelfID: self, PostType: types.PostTypeMessage},
		MessageType: types.MessageTypeGroup,
		MessageID:   100,
		GroupID:     types.GroupID(group),
		UserID:      user,
		Message: types.Message{
			{Type: types.SegmentAt, Data: types.SegmentData{"qq": self.String()}},
			{Type: types.SegmentText, Data: types.SegmentData{"text": text}},
		},
		Sender: types.Sender{UserID: user, Nickname: "n", Role: types.RoleMember},
	}}
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func TestForwardWSIntegration(t *testing.T) {
	p := mock.New(10000)
	defer p.Close()

	c, err := YukiChoco.New(YukiChoco.Config{
		Transport: YukiChoco.TransportForwardWS,
		ForwardWS: YukiChoco.ForwardWSConfig{URL: p.WSURL()},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got := make(chan *types.GroupMessageEvent, 1)
	event.On(c.Events(), func(_ context.Context, ev *types.GroupMessageEvent) error {
		got <- ev
		return nil
	})

	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if c.State() != YukiChoco.StateConnected {
		t.Fatalf("state=%s", c.State())
	}
	if err := c.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if c.SelfID() != 10000 {
		t.Fatalf("self=%d", c.SelfID())
	}

	ev := testGroupEvent(10000, 20001, 30001, " hello")
	if err := p.Push(ev); err != nil {
		t.Fatal(err)
	}
	select {
	case gotEv := <-got:
		if gotEv.GroupID != 20001 || gotEv.PlainText() != " hello" {
			t.Fatalf("event=%+v", gotEv)
		}
	case <-ctx.Done():
		t.Fatal("未收到事件")
	}

	id, err := c.Group().SendMessage(ctx, 20001, types.Message{
		{Type: types.SegmentText, Data: types.SegmentData{"text": "reply"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("message id 为 0")
	}
	call, err := p.WaitCall(ctx, types.ActionSendGroupMsg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(call.Params), `"group_id":20001`) {
		t.Fatalf("params=%s", call.Params)
	}
}

func TestReverseWSIntegration(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{
		Transport: YukiChoco.TransportReverseWS,
		ReverseWS: YukiChoco.ReverseWSConfig{Listen: "127.0.0.1:0", Path: "/ws"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got := make(chan *types.PrivateMessageEvent, 1)
	event.On(c.Events(), func(_ context.Context, ev *types.PrivateMessageEvent) error {
		got <- ev
		return nil
	})
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	addr := c.ReverseAddr()
	if addr == nil {
		t.Fatal("反向 WS 未监听")
	}

	p := mock.New(10000)
	defer p.Close()
	if err := p.ConnectReverse(ctx, "ws://"+addr.String()+"/ws", ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		_, err := c.System().GetLoginInfo(ctx)
		return err == nil
	})
	eventually(t, time.Second, func() bool { return c.SelfID() == 10000 })

	ev := &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{
		EventBase:   types.EventBase{Time: time.Now().Unix(), SelfID: 10000, PostType: types.PostTypeMessage},
		MessageType: types.MessageTypePrivate,
		MessageID:   1,
		UserID:      30001,
		Message:     types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "hi"}}},
	}}
	if err := p.Push(ev); err != nil {
		t.Fatal(err)
	}
	select {
	case gotEv := <-got:
		if gotEv.UserID != 30001 {
			t.Fatalf("event=%+v", gotEv)
		}
	case <-ctx.Done():
		t.Fatal("未收到事件")
	}

	id, err := c.SendPrivateText(ctx, 30001, "pong")
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("message id 为 0")
	}
	if _, err := p.WaitCall(ctx, types.ActionSendPrivateMsg); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookIntegration(t *testing.T) {
	p := mock.New(10000)
	defer p.Close()

	c, err := YukiChoco.New(YukiChoco.Config{
		Transport:   YukiChoco.TransportHTTP,
		AccessToken: "tok",
		HTTP:        YukiChoco.HTTPConfig{APIBaseURL: p.URL()},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := make(chan *types.GroupMessageEvent, 1)
	event.On(c.Events(), func(_ context.Context, ev *types.GroupMessageEvent) error {
		got <- ev
		return nil
	})

	h := c.WebhookHandler()
	if h == nil {
		t.Fatal("HTTP 模式应提供 WebhookHandler")
	}
	raw, _ := json.Marshal(testGroupEvent(10000, 20001, 30001, "x"))
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer tok")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rw.Code, rw.Body)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("未分发事件")
	}

	bad := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(string(raw)))
	rw = httptest.NewRecorder()
	h.ServeHTTP(rw, bad)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("无 token code=%d", rw.Code)
	}
}

func TestWebhookHandlerNilForForwardWS(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{
		Transport: YukiChoco.TransportForwardWS,
		ForwardWS: YukiChoco.ForwardWSConfig{URL: "ws://127.0.0.1:1/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.WebhookHandler() != nil {
		t.Fatal("正向 WS 模式不应提供 WebhookHandler")
	}
}

func TestLifecycle(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.WaitReady(ctx); err == nil {
		t.Fatal("未 Start 时 WaitReady 应超时")
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != YukiChoco.ErrAlreadyStarted {
		t.Fatalf("重复 Start: %v", err)
	}
	if err := c.WaitReady(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.State() != YukiChoco.StateConnected {
		t.Fatalf("state=%s", c.State())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.State() != YukiChoco.StateClosed {
		t.Fatalf("state=%s", c.State())
	}
	if err := c.WaitReady(context.Background()); err != YukiChoco.ErrClosed {
		t.Fatalf("关闭后 WaitReady=%v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("重复 Close: %v", err)
	}
	if _, err := c.System().GetStatus(context.Background()); err != YukiChoco.ErrClosed {
		t.Fatalf("关闭后调用=%v", err)
	}
}

func TestStateChanges(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-c.StateChanges():
		if s != YukiChoco.StateConnected {
			t.Fatalf("state=%s", s)
		}
	case <-time.After(time.Second):
		t.Fatal("未收到状态变化")
	}
	_ = c.Close()
}
