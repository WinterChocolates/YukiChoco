package transport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/WinterChocolates/YukiChoco/transport"
)

func upgrader() websocket.Upgrader {
	return websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
}

// TestNextReturnsEvents 验证事件按条读出。
func TestNextReturnsEvents(t *testing.T) {
	up := upgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"event_type":"a"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"event_type":"b"}`))
		<-time.After(300 * time.Millisecond)
	}))
	defer srv.Close()

	src := transport.NewWS(transport.WSConfig{URL: wsURL(srv.URL), IdleTimeout: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sess, err := src.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer sess.Close()

	for _, want := range []string{`{"event_type":"a"}`, `{"event_type":"b"}`} {
		raw, err := sess.Next(ctx)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if string(raw) != want {
			t.Fatalf("Next = %s, want %s", raw, want)
		}
	}
}

// TestAuthorizationHeaderThenQueryFallback 验证 401 时降级到 query token。
func TestAuthorizationHeaderThenQueryFallback(t *testing.T) {
	up := upgrader()
	var mu sync.Mutex
	var attempts int
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		lastQuery = r.URL.RawQuery
		hasHeader := r.Header.Get("Authorization") != ""
		mu.Unlock()

		// 模拟不接受自定义 Header 的环境：带 Authorization 一律 401。
		if hasHeader {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("access_token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"ok":true}`))
		<-time.After(200 * time.Millisecond)
	}))
	defer srv.Close()

	src := transport.NewWS(transport.WSConfig{URL: wsURL(srv.URL), AccessToken: "tok", IdleTimeout: time.Second})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect 应降级成功: %v", err)
	}
	defer sess.Close()

	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Errorf("握手次数 = %d, want 2（先 Header 后 query）", attempts)
	}
	if !strings.Contains(lastQuery, "access_token=tok") {
		t.Errorf("降级请求未携带 token: %s", lastQuery)
	}
}

// TestIdleTimeoutDetectsDeadConnection 验证读空闲超时会结束会话。
func TestIdleTimeoutDetectsDeadConnection(t *testing.T) {
	up := upgrader()
	block := make(chan struct{})
	defer close(block)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// 握手后既不读也不写：客户端发的 ping 得不到 pong。
		<-block
	}))
	defer srv.Close()

	src := transport.NewWS(transport.WSConfig{
		URL:          wsURL(srv.URL),
		IdleTimeout:  80 * time.Millisecond,
		PingInterval: 20 * time.Millisecond,
	})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer sess.Close()

	started := time.Now()
	_, err = sess.Next(context.Background())
	if err == nil {
		t.Fatal("读空闲超时应返回错误")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("空闲检测耗时过长: %s", elapsed)
	}
}

// TestCloseUnblocksNext 验证 Close 能让阻塞中的 Next 返回。
func TestCloseUnblocksNext(t *testing.T) {
	up := upgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-r.Context().Done()
	}))
	defer srv.Close()

	src := transport.NewWS(transport.WSConfig{URL: wsURL(srv.URL), IdleTimeout: 30 * time.Second})
	sess, err := src.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := sess.Next(context.Background())
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close 后 Next 应返回错误")
		}
	case <-time.After(time.Second):
		t.Fatal("Close 未能解除 Next 阻塞")
	}
	// Close 可重复调用。
	if err := sess.Close(); err != nil {
		t.Fatalf("二次 Close: %v", err)
	}
}

// TestContextCancellationStopsSession 验证父 ctx 取消会关闭连接。
func TestContextCancellationStopsSession(t *testing.T) {
	up := upgrader()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	src := transport.NewWS(transport.WSConfig{URL: wsURL(srv.URL), IdleTimeout: 30 * time.Second})
	sess, err := src.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := sess.Next(ctx)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ctx 取消未能解除 Next 阻塞")
	}
}

// TestConnectRefused 验证连接失败返回错误。
func TestConnectRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := wsURL(srv.URL)
	srv.Close()

	src := transport.NewWS(transport.WSConfig{URL: url, DialTimeout: 500 * time.Millisecond})
	if _, err := src.Connect(context.Background()); err == nil {
		t.Fatal("期望连接失败")
	}
}

// TestConnectUnauthorized 验证 401 时返回 ErrUnauthorized。
func TestConnectUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src := transport.NewWS(transport.WSConfig{URL: wsURL(srv.URL), AccessToken: "bad"})
	_, err := src.Connect(context.Background())
	if !errors.Is(err, transport.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestName(t *testing.T) {
	if got := transport.NewWS(transport.WSConfig{URL: "ws://x"}).Name(); got != transport.SourceWS {
		t.Fatalf("Name = %q", got)
	}
}

// wsURL 把 httptest 的 http URL 转成事件通道地址。
func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1) + "/event"
}
