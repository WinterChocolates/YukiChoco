package transport_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/WinterChocolates/YukiChoco/transport"
)

func TestDialAndEcho(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := up.Upgrade(rw, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			_ = c.WriteMessage(websocket.TextMessage, data)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, transport.DialConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.Write(ctx, []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	got, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"x":1}` {
		t.Fatalf("got %s", got)
	}
}

func TestDialFallsBackToQueryToken(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	const token = "tok"
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != token {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := up.Upgrade(rw, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_, _, _ = c.ReadMessage()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, transport.DialConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), AccessToken: token,
	})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

func TestReverseServer(t *testing.T) {
	s := transport.NewReverseServer(transport.ReverseConfig{
		Listen: "127.0.0.1:0", Path: "/ws", AccessToken: "tok",
	})
	msgCh := make(chan string, 1)
	onConnect := make(chan struct{}, 1)
	s.OnConnect(func(*transport.Conn) { onConnect <- struct{}{} })
	s.OnMessage(func(_ *transport.Conn, data []byte) { msgCh <- string(data) })
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())

	addr := s.Addr()
	if addr == nil {
		t.Fatal("addr nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, transport.DialConfig{
		URL: "ws://" + addr.String() + "/ws", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	select {
	case <-onConnect:
	case <-ctx.Done():
		t.Fatal("onConnect 未触发")
	}
	if err := conn.Write(ctx, []byte(`{"post_type":"meta_event"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-msgCh:
		if got != `{"post_type":"meta_event"}` {
			t.Fatalf("got %s", got)
		}
	case <-ctx.Done():
		t.Fatal("未收到消息")
	}

	if err := s.Send(ctx, []byte(`hello`)); err != nil {
		t.Fatal(err)
	}
	got, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %s", got)
	}
}

func TestReverseServerRejectsBadToken(t *testing.T) {
	s := transport.NewReverseServer(transport.ReverseConfig{
		Listen: "127.0.0.1:0", Path: "/ws", AccessToken: "tok",
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := transport.Dial(ctx, transport.DialConfig{
		URL: "ws://" + s.Addr().String() + "/ws", AccessToken: "bad",
	})
	if err == nil {
		t.Fatal("应拒绝错误 token")
	}
}

func TestWebhookAuthAndSignature(t *testing.T) {
	w := transport.NewWebhook(transport.WebhookConfig{
		AccessToken: "tok", VerifyToken: true, Secret: "sec",
	})
	got := make(chan string, 1)
	w.OnEvent(func(b []byte) { got <- string(b) })
	h := w.Handler()

	body := `{"post_type":"meta_event"}`
	mac := hmac.New(sha1.New, []byte("sec"))
	mac.Write([]byte(body))
	sig := "sha1=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Signature", sig)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rw.Code, rw.Body)
	}
	if v := <-got; v != body {
		t.Fatalf("event=%s", v)
	}

	bad := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	bad.Header.Set("Authorization", "Bearer nope")
	bad.Header.Set("X-Signature", sig)
	rw = httptest.NewRecorder()
	h.ServeHTTP(rw, bad)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("bad token code=%d", rw.Code)
	}

	badSig := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	badSig.Header.Set("Authorization", "Bearer tok")
	badSig.Header.Set("X-Signature", "sha1=deadbeef")
	rw = httptest.NewRecorder()
	h.ServeHTTP(rw, badSig)
	if rw.Code != http.StatusForbidden {
		t.Fatalf("bad sig code=%d", rw.Code)
	}
}
