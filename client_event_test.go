package YukiChoco

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/WinterChocolates/YukiChoco/types"
)

// groupMessageJSON 是一条最小可用的群消息事件。
const groupMessageJSON = `{"time":1,"self_id":100,"event_type":"message_receive","data":{
	"message_scene":"group","peer_id":200,"message_seq":7,"sender_id":300,"time":1,
	"segments":[{"type":"text","data":{"text":"你好"}},{"type":"mention","data":{"user_id":100,"name":"bot"}}],
	"group":{"group_id":200,"group_name":"测试群","member_count":2,"max_member_count":500},
	"group_member":{"user_id":300,"nickname":"群友","sex":"unknown","group_id":200,"card":"昵称",
		"title":"","level":1,"role":"member","join_time":1,"last_sent_time":1}}}`

// newEventServer 启动一个只处理 /event 的 WebSocket 服务端。
func newEventServer(t *testing.T, handle func(conn *websocket.Conn, r *http.Request)) *httptest.Server {
	t.Helper()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event" {
			http.NotFound(w, r)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// waitConnClosed 阻塞到客户端关闭连接。
//
// 注意不能用 r.Context().Done()：WebSocket 是 hijacked 连接，srv.Close 不会
// 关闭它，也不会取消 request context，测试会留下僵尸 handler。
func waitConnClosed(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// eventConfig 返回一份重连极快、关闭抖动的测试配置。
func eventConfig(baseURL string, tweak func(*Config)) Config {
	cfg := Config{
		BaseURL:      baseURL,
		AccessToken:  "secret-token",
		EventSource:  EventSourceWS,
		IdleTimeout:  2 * time.Second,
		PingInterval: 200 * time.Millisecond,
		Reconnect: ReconnectConfig{
			InitialDelay: time.Millisecond,
			MaxDelay:     5 * time.Millisecond,
			Jitter:       -1, // 关闭抖动，让测试更稳定
		},
		Dispatch: DispatchConfig{Buffer: 32},
	}
	if tweak != nil {
		tweak(&cfg)
	}
	return cfg
}

// waitUntil 轮询等待条件成立。
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", msg)
}

func TestStartDeliversTypedEvents(t *testing.T) {
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		waitConnClosed(conn)
	})

	c, err := New(eventConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := make(chan *types.GroupMessage, 1)
	off := On(c, func(_ context.Context, ev *types.MessageReceiveEvent) error {
		if msg, ok := ev.GroupMessage(); ok {
			select {
			case got <- msg:
			default:
			}
		}
		return nil
	})
	defer off()

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	select {
	case msg := <-got:
		if msg.Group.GroupName != "测试群" || msg.Group.GroupID != 200 {
			t.Fatalf("群信息错误: %+v", msg.Group)
		}
		if got := msg.PlainText(); got != "你好" {
			t.Fatalf("PlainText = %q", got)
		}
		if !msg.Segments.Mentioned(100) {
			t.Error("Mentioned(100) = false")
		}
		if msg.GroupMember.DisplayName() != "昵称" {
			t.Errorf("DisplayName = %q", msg.GroupMember.DisplayName())
		}
	case <-ctx.Done():
		t.Fatal("未收到事件")
	}

	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if c.State() != StateClosed {
		t.Errorf("State = %v, want closed", c.State())
	}
	if _, err := c.System().GetLoginInfo(context.Background()); !errors.Is(err, ErrClosed) {
		t.Errorf("Shutdown 后 API err = %v, want ErrClosed", err)
	}
}

func TestOnRawHookSeesOriginalPayload(t *testing.T) {
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		waitConnClosed(conn)
	})
	c, err := New(eventConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Shutdown(context.Background())

	raws := make(chan string, 4)
	c.OnRaw(func(_ context.Context, raw []byte) error {
		raws <- string(raw)
		return nil
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case raw := <-raws:
		if raw != groupMessageJSON {
			t.Fatalf("原始载荷被修改:\n got=%s\nwant=%s", raw, groupMessageJSON)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("未收到原始事件")
	}
}

func TestReconnectAfterDisconnect(t *testing.T) {
	var connects int32
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		n := atomic.AddInt32(&connects, 1)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		if n >= 2 {
			waitConnClosed(conn)
			return
		}
		// 第一次连接：写完事件就断开，触发重连。
		_ = conn.Close()
	})

	c, err := New(eventConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var received int32
	On(c, func(context.Context, *types.MessageReceiveEvent) error {
		atomic.AddInt32(&received, 1)
		return nil
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitUntil(t, 5*time.Second, func() bool {
		return atomic.LoadInt32(&connects) >= 2 && atomic.LoadInt32(&received) >= 2
	}, "重连并再次收到事件")

	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestMalformedEventDoesNotKillConnection(t *testing.T) {
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`这不是 JSON`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		waitConnClosed(conn)
	})
	c, err := New(eventConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var received int32
	On(c, func(context.Context, *types.MessageReceiveEvent) error {
		atomic.AddInt32(&received, 1)
		return nil
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return atomic.LoadInt32(&received) >= 1 }, "坏事件之后的好事件")
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestStateTransitions(t *testing.T) {
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		// 主动断开，验证「重连关闭」时进入 Disconnected。
		_ = conn.Close()
	})

	c, err := New(eventConfig(srv.URL, func(cfg *Config) {
		cfg.Reconnect.Disable = true
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var mu sync.Mutex
	var seen []State
	c.OnStateChange(func(s State) {
		mu.Lock()
		seen = append(seen, s)
		mu.Unlock()
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range seen {
			if s == StateDisconnected {
				return true
			}
		}
		return false
	}, "进入 Disconnected")

	mu.Lock()
	defer mu.Unlock()
	want := []State{StateDialing, StateConnected, StateDisconnected}
	if len(seen) < len(want) {
		t.Fatalf("状态序列 = %v, want 至少 %v", seen, want)
	}
	for i, w := range want {
		if seen[i] != w {
			t.Fatalf("状态序列 = %v, want %v", seen, want)
		}
	}
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestStartTwiceAndNoneSource(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", EventSource: EventSourceNone})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 纯 API 模式立即就绪。
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if c.State() != StateConnected {
		t.Errorf("State = %v, want connected", c.State())
	}
	if err := c.Start(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("重复 Start err = %v, want ErrAlreadyStarted", err)
	}
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// Shutdown 幂等。
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("二次 Shutdown: %v", err)
	}
}

func TestShutdownTimesOutWithStuckHandler(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		waitConnClosed(conn)
	})
	c, err := New(eventConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	On(c, func(context.Context, *types.MessageReceiveEvent) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler 未开始")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown err = %v, want DeadlineExceeded", err)
	}

	close(release)
	// 重试 Shutdown 应当等到 handler 结束并成功。
	done := make(chan error, 1)
	go func() { done <- c.Shutdown(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("重试 Shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("重试 Shutdown 未返回")
	}
}

func TestNoGoroutineLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		waitConnClosed(conn)
	})

	for i := 0; i < 3; i++ {
		c, err := New(eventConfig(srv.URL, nil))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		On(c, func(context.Context, *types.MessageReceiveEvent) error { return nil })
		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := c.WaitReady(ctx); err != nil {
			cancel()
			t.Fatalf("WaitReady: %v", err)
		}
		cancel()
		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	}
	srv.Close()

	// 允许测试框架自身的少量波动。
	waitUntil(t, 3*time.Second, func() bool {
		return runtime.NumGoroutine() <= baseline+2
	}, "goroutine 回收")
}

func TestDispatchModesViaConfig(t *testing.T) {
	srv := newEventServer(t, func(conn *websocket.Conn, r *http.Request) {
		for i := 0; i < 5; i++ {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(groupMessageJSON))
		}
		waitConnClosed(conn)
	})
	c, err := New(eventConfig(srv.URL, func(cfg *Config) {
		cfg.Dispatch.Mode = DispatchSequential
		cfg.Dispatch.Buffer = 16
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var count int32
	On(c, func(context.Context, *types.MessageReceiveEvent) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return atomic.LoadInt32(&count) >= 5 }, "收到 5 条事件")
	if got := c.Events().Stats().Mode; got != DispatchSequential {
		t.Errorf("Mode = %v", got)
	}
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestEventURLDerivation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"ws http", Config{BaseURL: "http://127.0.0.1:6700"}, "ws://127.0.0.1:6700/event"},
		{"ws https", Config{BaseURL: "https://bot.example.com"}, "wss://bot.example.com/event"},
		{"ws 带路径", Config{BaseURL: "http://bot.example.com/milky/"}, "ws://bot.example.com/milky/event"},
		{"sse", Config{BaseURL: "http://127.0.0.1:6700", EventSource: EventSourceSSE}, "http://127.0.0.1:6700/event"},
		{"sse https", Config{BaseURL: "https://bot.example.com", EventSource: EventSourceSSE}, "https://bot.example.com/event"},
		{"webhook", Config{BaseURL: "http://127.0.0.1:6700", EventSource: EventSourceWebhook}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer c.Close()
			if got := c.Config().EventURL; got != tc.want {
				t.Errorf("EventURL = %q, want %q", got, tc.want)
			}
		})
	}

	bad := []Config{
		{BaseURL: "http://x", EventSource: "mqtt"},
		{BaseURL: "http://x", EventURL: "http://x/event"},                            // WS 需要 ws/wss
		{BaseURL: "http://x", EventSource: EventSourceSSE, EventURL: "ws://x/event"}, // SSE 需要 http(s)
	}
	for i, cfg := range bad {
		if _, err := New(cfg); err == nil {
			t.Errorf("case %d 应报错: %+v", i, cfg)
		}
	}
}
