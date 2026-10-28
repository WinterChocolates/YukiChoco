// Package mock 提供可编程的假 Milky 协议端。
//
// 它同时实现 HTTP API、WebSocket 事件流与 SSE 事件流，用于在不连接真实 QQ 的
// 情况下测试上层代码：
//
//	srv := mock.New(mock.WithToken("tok"))
//	srv.Handle("get_login_info", func(req json.RawMessage) (any, error) {
//		return map[string]any{"uin": 10001, "nickname": "bot"}, nil
//	})
//	c, _ := YukiChoco.New(YukiChoco.Config{BaseURL: srv.URL(), AccessToken: "tok"})
//	c.Start(ctx)
//	srv.Push(eventJSON) // 推给所有已连接的客户端
//
// 未注册的端点默认返回成功 + 空 data（`{}`），因此测试只需注册关心的端点。
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Handler 处理一个 API 请求。
//
// req 是原始 JSON body；返回值会被包进 `{"status":"ok","retcode":0,"data":...}`。
// 返回 *APIError 时生成失败信封。
type Handler func(req json.RawMessage) (any, error)

// APIError 让 Handler 返回协议层失败。
type APIError struct {
	Retcode int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mock: retcode=%d: %s", e.Retcode, e.Message)
}

// Call 记录一次 API 调用。
type Call struct {
	Endpoint string
	Body     json.RawMessage
	At       time.Time
}

// Server 是假的 Milky 协议端。
type Server struct {
	srv   *httptest.Server
	token string
	sse   bool

	mu       sync.RWMutex
	handlers map[string]Handler
	fallback Handler
	calls    []Call

	connMu sync.Mutex
	conns  map[*wsConn]struct{}
	sses   map[chan []byte]struct{}

	connCount atomic.Int32
	closed    atomic.Bool
}

// Option 用于配置 Server。
type Option func(*Server)

// WithToken 要求所有请求携带 `Authorization: Bearer <token>`（或事件通道的
// `?access_token=`）。
func WithToken(token string) Option {
	return func(s *Server) { s.token = token }
}

// WithSSE 让 /event 提供 SSE 而不是 WebSocket。
func WithSSE() Option {
	return func(s *Server) { s.sse = true }
}

// New 启动假协议端。使用完毕后必须调用 Close。
func New(opts ...Option) *Server {
	s := &Server{
		handlers: make(map[string]Handler),
		conns:    make(map[*wsConn]struct{}),
		sses:     make(map[chan []byte]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.handleAPI)
	mux.HandleFunc("/event", s.handleEvent)
	s.srv = httptest.NewServer(mux)
	return s
}

// URL 返回协议端地址，可直接作为 Config.BaseURL。
func (s *Server) URL() string { return s.srv.URL }

// Close 关闭服务端与所有连接。
func (s *Server) Close() {
	if s.closed.Swap(true) {
		return
	}
	s.connMu.Lock()
	for c := range s.conns {
		_ = c.ws.Close()
	}
	for ch := range s.sses {
		close(ch)
	}
	s.conns = map[*wsConn]struct{}{}
	s.sses = map[chan []byte]struct{}{}
	s.connMu.Unlock()
	s.srv.Close()
}

// Handle 注册端点处理函数。
func (s *Server) Handle(endpoint string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[endpoint] = h
}

// OnAnyAPI 注册未匹配端点时的兜底处理函数。
func (s *Server) OnAnyAPI(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback = h
}

// Calls 返回所有已收到的 API 调用（按时间顺序）。
func (s *Server) Calls() []Call {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Call, len(s.calls))
	copy(out, s.calls)
	return out
}

// LastCall 返回指定端点的最后一次调用。
func (s *Server) LastCall(endpoint string) (Call, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.calls) - 1; i >= 0; i-- {
		if s.calls[i].Endpoint == endpoint {
			return s.calls[i], true
		}
	}
	return Call{}, false
}

// Connected 返回当前事件连接数。
func (s *Server) Connected() int { return int(s.connCount.Load()) }

// WaitConnected 等待至少 n 条事件连接建立。
func (s *Server) WaitConnected(ctx context.Context, n int) error {
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.Connected() >= n {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Push 把任意值序列化为事件 JSON 并推送给所有客户端。
func (s *Server) Push(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.PushJSON(raw)
}

// PushJSON 把原始事件 JSON 推送给所有客户端。
//
// 返回的 error 仅在完全没有客户端时出现；单个连接写失败会被忽略并断开。
func (s *Server) PushJSON(raw []byte) error {
	payload := append([]byte(nil), raw...)

	s.connMu.Lock()
	conns := make([]*wsConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	sses := make([]chan []byte, 0, len(s.sses))
	for ch := range s.sses {
		sses = append(sses, ch)
	}
	s.connMu.Unlock()

	if len(conns) == 0 && len(sses) == 0 {
		return fmt.Errorf("mock: 没有已连接的事件客户端")
	}
	for _, c := range conns {
		select {
		case c.out <- payload:
		default:
			// 慢客户端：丢弃本次，避免阻塞测试。
		}
	}
	for _, ch := range sses {
		select {
		case ch <- payload:
		default:
		}
	}
	return nil
}

// ───────────────────────────── API ─────────────────────────────

func (s *Server) handleAPI(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	endpoint := strings.TrimPrefix(r.URL.Path, "/api/")
	if endpoint == "" {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.calls = append(s.calls, Call{Endpoint: endpoint, Body: body, At: time.Now()})
	h := s.handlers[endpoint]
	if h == nil {
		h = s.fallback
	}
	s.mu.Unlock()

	rw.Header().Set("Content-Type", "application/json")
	if h == nil {
		writeOK(rw, map[string]any{})
		return
	}

	data, err := h(body)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"status":  "failed",
				"retcode": apiErr.Retcode,
				"message": apiErr.Message,
			})
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"status":  "failed",
			"retcode": -500,
			"message": err.Error(),
		})
		return
	}
	writeOK(rw, data)
}

func writeOK(rw http.ResponseWriter, data any) {
	if data == nil {
		data = map[string]any{}
	}
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"status":  "ok",
		"retcode": 0,
		"data":    data,
	})
}

// ───────────────────────────── 事件通道 ─────────────────────────────

func (s *Server) handleEvent(rw http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	if s.sse {
		s.serveSSE(rw, r)
		return
	}
	s.serveWS(rw, r)
}

// authorized 校验 access_token（Header 或 query）。
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.token {
		return true
	}
	return r.URL.Query().Get("access_token") == s.token
}

// wsConn 是一条 WS 连接及其写队列。
type wsConn struct {
	ws  *websocket.Conn
	out chan []byte
	mu  sync.Mutex
}

func (s *Server) serveWS(rw http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	ws, err := up.Upgrade(rw, r, nil)
	if err != nil {
		return
	}
	conn := &wsConn{ws: ws, out: make(chan []byte, 32)}

	s.connMu.Lock()
	s.conns[conn] = struct{}{}
	s.connMu.Unlock()
	s.connCount.Add(1)

	// 写协程
	go func() {
		for payload := range conn.out {
			conn.mu.Lock()
			_ = ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := ws.WriteMessage(websocket.TextMessage, payload)
			conn.mu.Unlock()
			if err != nil {
				_ = ws.Close()
				return
			}
		}
	}()

	// 读协程：只用于感知断开与回应 ping（gorilla 默认处理 pong）。
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}

	close(conn.out)
	s.connMu.Lock()
	delete(s.conns, conn)
	s.connMu.Unlock()
	s.connCount.Add(-1)
}

func (s *Server) serveSSE(rw http.ResponseWriter, r *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan []byte, 32)
	s.connMu.Lock()
	s.sses[ch] = struct{}{}
	s.connMu.Unlock()
	s.connCount.Add(1)
	defer func() {
		s.connMu.Lock()
		if _, ok := s.sses[ch]; ok {
			delete(s.sses, ch)
			close(ch)
		}
		s.connMu.Unlock()
		s.connCount.Add(-1)
	}()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(rw, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case payload, ok := <-ch:
			if !ok {
				return
			}
			// 按 SSE 规范：data 内的换行需要拆成多行 data。
			lines := strings.Split(string(payload), "\n")
			var b strings.Builder
			b.WriteString("event: milky_event\n")
			for _, line := range lines {
				b.WriteString("data: ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			b.WriteString("\n")
			if _, err := fmt.Fprint(rw, b.String()); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
