package transport_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/transport"
)

// sseServer 启动一个 SSE 服务端；handler 拿到 http.ResponseWriter 后可自由写入。
func sseServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sseURL(srv *httptest.Server) string { return srv.URL + "/event" }

// TestSSEParsesEvents 验证 event/data/多行 data/注释行的解析。
func TestSSEParsesEvents(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		// 保活注释行
		_, _ = fmt.Fprint(w, ": keep-alive\n\n")
		// 简单事件
		_, _ = fmt.Fprint(w, "event: milky_event\ndata: {\"a\":1}\n\n")
		// 多行 data（需要拼接）
		_, _ = fmt.Fprint(w, "event: milky_event\ndata: {\"b\":\ndata: 2}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	})

	src := transport.NewSSE(transport.SSEConfig{URL: sseURL(srv), IdleTimeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sess, err := src.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer sess.Close()

	for _, want := range []string{`{"a":1}`, "{\"b\":\n2}"} {
		raw, err := sess.Next(ctx)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if string(raw) != want {
			t.Fatalf("Next = %q, want %q", raw, want)
		}
	}
}

// TestSSEAuthorizationFallback 验证 401 时降级到 query token。
func TestSSEAuthorizationFallback(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("access_token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		_, _ = fmt.Fprint(w, "data: {\"ok\":true}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	src := transport.NewSSE(transport.SSEConfig{URL: srv.URL + "/event", AccessToken: "tok", IdleTimeout: time.Second})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect 应降级成功: %v", err)
	}
	defer sess.Close()
	if attempts != 2 {
		t.Errorf("握手次数 = %d, want 2", attempts)
	}
}

func TestSSEIdleTimeout(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		// 只发一条保活注释，之后保持静默。
		_, _ = fmt.Fprint(w, ": alive\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	src := transport.NewSSE(transport.SSEConfig{URL: sseURL(srv), IdleTimeout: 80 * time.Millisecond})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer sess.Close()

	started := time.Now()
	if _, err := sess.Next(context.Background()); err == nil {
		t.Fatal("读空闲超时应返回错误")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("空闲检测耗时过长: %s", elapsed)
	}
}

func TestSSECloseUnblocksNext(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	src := transport.NewSSE(transport.SSEConfig{URL: sseURL(srv), IdleTimeout: 30 * time.Second})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := sess.Next(context.Background())
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = sess.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close 后 Next 应返回错误")
		}
	case <-time.After(time.Second):
		t.Fatal("Close 未解除 Next 阻塞")
	}
}

func TestSSEContentTypeMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	src := transport.NewSSE(transport.SSEConfig{URL: srv.URL + "/event"})
	if _, err := src.Connect(context.Background()); err == nil {
		t.Fatal("Content-Type 不匹配应报错")
	}
}

func TestSSEUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src := transport.NewSSE(transport.SSEConfig{URL: srv.URL + "/event", AccessToken: "bad"})
	_, err := src.Connect(context.Background())
	if !errors.Is(err, transport.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestSSEName(t *testing.T) {
	if got := transport.NewSSE(transport.SSEConfig{URL: "http://x/event"}).Name(); got != transport.SourceSSE {
		t.Fatalf("Name = %q", got)
	}
}
