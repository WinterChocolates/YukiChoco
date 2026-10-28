// Package mock 提供一个可编程的假 OneBot v11 协议端。
//
// 它同时支持 HTTP API、正向 WebSocket 与反向 WebSocket，用于在不启动真实
// QQ 账号的情况下编写集成测试。
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/WinterChocolates/YukiChoco/types"
)

// ActionHandler 处理一个 action，返回 data 或 error。
//
// error 会被编码为 status=failed / retcode=100。
type ActionHandler func(params json.RawMessage) (any, error)

// Call 是一次被记录的动作调用。
type Call struct {
	Action string
	Params json.RawMessage
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Protocol 是假协议端。
type Protocol struct {
	SelfID  types.UserID
	AppName string

	server *httptest.Server
	closed atomic.Bool
	msgSeq atomic.Int64

	mu       sync.Mutex
	handlers map[string]ActionHandler
	calls    []Call
	conns    map[*wsClient]struct{}
	reverse  map[*wsClient]struct{}
}

// New 创建并启动假协议端。
func New(selfID types.UserID) *Protocol {
	p := &Protocol{
		SelfID:   selfID,
		AppName:  "mock",
		handlers: make(map[string]ActionHandler),
		conns:    make(map[*wsClient]struct{}),
		reverse:  make(map[*wsClient]struct{}),
		msgSeq:   atomic.Int64{},
	}
	p.msgSeq.Store(1000)

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.serve)
	p.server = httptest.NewServer(mux)
	return p
}

// URL 返回 HTTP API 根地址。
func (p *Protocol) URL() string { return p.server.URL }

// WSURL 返回正向 WebSocket 地址（通用单连接）。
func (p *Protocol) WSURL() string { return "ws" + strings.TrimPrefix(p.server.URL, "http") + "/" }

// APIWSURL 返回正向 WebSocket 的 /api 地址（配套 EventWSURL）。
func (p *Protocol) APIWSURL() string { return "ws" + strings.TrimPrefix(p.server.URL, "http") + "/api" }

// EventWSURL 返回正向 WebSocket 的 /event 地址。
func (p *Protocol) EventWSURL() string {
	return "ws" + strings.TrimPrefix(p.server.URL, "http") + "/event"
}

// Close 关闭假协议端与所有连接。
func (p *Protocol) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	p.mu.Lock()
	for c := range p.conns {
		_ = c.conn.Close()
	}
	for c := range p.reverse {
		_ = c.conn.Close()
	}
	p.mu.Unlock()
	p.server.Close()
}

// On 注册 action 处理器。
func (p *Protocol) On(action string, fn ActionHandler) {
	p.mu.Lock()
	p.handlers[action] = fn
	p.mu.Unlock()
}

// Calls 返回已记录的动作调用。
func (p *Protocol) Calls() []Call {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Call(nil), p.calls...)
}

// LastCall 返回最后一次调用；没有时返回零值。
func (p *Protocol) LastCall() (Call, bool) {
	calls := p.Calls()
	if len(calls) == 0 {
		return Call{}, false
	}
	return calls[len(calls)-1], true
}

// WaitCall 等待某个 action 至少被调用一次。
func (p *Protocol) WaitCall(ctx context.Context, action string) (Call, error) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, c := range p.Calls() {
			if c.Action == action {
				return c, nil
			}
		}
		select {
		case <-ctx.Done():
			return Call{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Protocol) serve(rw http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/api" || r.URL.Path == "/event"):
		p.serveWS(rw, r)
	case r.Method == http.MethodPost:
		action := strings.TrimPrefix(r.URL.Path, "/")
		if action == "" {
			http.NotFound(rw, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		if err != nil {
			http.Error(rw, "bad body", http.StatusBadRequest)
			return
		}
		env := p.dispatch(action, body)
		env.Echo = json.RawMessage(`"http"`)
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(env)
	default:
		http.NotFound(rw, r)
	}
}

func (p *Protocol) serveWS(rw http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return
	}
	c := &wsClient{conn: conn}
	p.mu.Lock()
	p.conns[c] = struct{}{}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.conns, c)
		p.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		p.handleWSFrame(c, data)
	}
}

func (p *Protocol) handleWSFrame(c *wsClient, data []byte) {
	var req struct {
		Action string          `json:"action"`
		Params json.RawMessage `json:"params"`
		Echo   json.RawMessage `json:"echo"`
	}
	if err := json.Unmarshal(data, &req); err != nil || req.Action == "" {
		return
	}
	env := p.dispatch(req.Action, req.Params)
	env.Echo = req.Echo
	raw, err := json.Marshal(env)
	if err != nil {
		return
	}
	_ = c.write(raw)
}

func (p *Protocol) dispatch(action string, params json.RawMessage) envelope {
	p.mu.Lock()
	p.calls = append(p.calls, Call{Action: action, Params: append(json.RawMessage(nil), params...)})
	fn := p.handlers[action]
	p.mu.Unlock()

	if fn == nil {
		fn = p.defaultHandler(action)
	}
	data, err := fn(params)
	if err != nil {
		return envelope{Status: "failed", Retcode: 100, Message: err.Error()}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return envelope{Status: "failed", Retcode: 100, Message: err.Error()}
	}
	return envelope{Status: "ok", Retcode: 0, Data: raw}
}

func (p *Protocol) defaultHandler(action string) ActionHandler {
	switch action {
	case types.ActionGetLoginInfo:
		return func(json.RawMessage) (any, error) {
			return map[string]any{"user_id": p.SelfID, "nickname": "mock-bot"}, nil
		}
	case types.ActionGetVersionInfo:
		return func(json.RawMessage) (any, error) {
			return map[string]any{
				"app_name": p.AppName, "app_version": "0.0.0", "protocol_version": "v11",
			}, nil
		}
	case types.ActionGetStatus:
		return func(json.RawMessage) (any, error) {
			return map[string]any{"online": true, "good": true}, nil
		}
	case types.ActionGetGroupInfo:
		return func(params json.RawMessage) (any, error) {
			var req struct {
				GroupID types.GroupID `json:"group_id"`
			}
			_ = json.Unmarshal(params, &req)
			return map[string]any{
				"group_id": req.GroupID, "group_name": "mock-group",
				"member_count": 1, "max_member_count": 200,
			}, nil
		}
	case types.ActionGetGroupMemberInfo:
		return func(params json.RawMessage) (any, error) {
			var req struct {
				GroupID types.GroupID `json:"group_id"`
				UserID  types.UserID  `json:"user_id"`
			}
			_ = json.Unmarshal(params, &req)
			return map[string]any{
				"group_id": req.GroupID, "user_id": req.UserID,
				"nickname": "mock-user", "card": "",
				"role": "member", "join_time": 0,
			}, nil
		}
	case types.ActionGetFriendList:
		return func(json.RawMessage) (any, error) {
			return []map[string]any{{"user_id": 10001, "nickname": "friend", "remark": ""}}, nil
		}
	case types.ActionGetGroupList:
		return func(json.RawMessage) (any, error) {
			return []map[string]any{{"group_id": 20001, "group_name": "mock-group", "member_count": 1, "max_member_count": 200}}, nil
		}
	case types.ActionSendPrivateMsg, types.ActionSendGroupMsg, types.ActionSendMsg,
		types.ActionSendGroupForwardMsg, types.ActionSendPrivateForwardMsg:
		return func(json.RawMessage) (any, error) {
			return map[string]any{"message_id": p.msgSeq.Add(1)}, nil
		}
	case types.ActionGetMsg:
		return func(params json.RawMessage) (any, error) {
			var req types.GetMsgRequest
			_ = json.Unmarshal(params, &req)
			return map[string]any{
				"message_id": req.MessageID, "message_type": "group",
				"user_id": 10001, "time": time.Now().Unix(),
				"message": []map[string]any{{"type": "text", "data": map[string]any{"text": "mock"}}},
			}, nil
		}
	}
	return func(json.RawMessage) (any, error) { return map[string]any{}, nil }
}

// Push 向所有已连接的正向 WS 与反向 WS 客户端推送事件。
func (p *Protocol) Push(ev types.Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return p.PushJSON(raw)
}

// PushJSON 推送原始事件 JSON。
func (p *Protocol) PushJSON(raw []byte) error {
	p.mu.Lock()
	conns := make([]*wsClient, 0, len(p.conns)+len(p.reverse))
	for c := range p.conns {
		conns = append(conns, c)
	}
	for c := range p.reverse {
		conns = append(conns, c)
	}
	p.mu.Unlock()
	if len(conns) == 0 {
		return fmt.Errorf("mock: 没有已连接的 WebSocket 客户端")
	}
	var firstErr error
	for _, c := range conns {
		if err := c.write(raw); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ConnectReverse 主动连接被测客户端的反向 WS 服务。
//
// 返回值在连接建立后即返回；后续事件通过 Push 发送，客户端发来的 API 请求
// 会被记录并自动响应。
func (p *Protocol) ConnectReverse(ctx context.Context, rawURL string, accessToken string) error {
	header := http.Header{}
	header.Set("X-Self-ID", p.SelfID.String())
	header.Set("X-Client-Role", "Universal")
	if accessToken != "" {
		header.Set("Authorization", "Bearer "+accessToken)
	}
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.DialContext(ctx, rawURL, header)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
			return fmt.Errorf("mock: 反向 WS 连接失败: HTTP %s: %w", resp.Status, err)
		}
		return err
	}
	c := &wsClient{conn: conn}
	p.mu.Lock()
	p.reverse[c] = struct{}{}
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.reverse, c)
			p.mu.Unlock()
			_ = conn.Close()
		}()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			p.handleWSFrame(c, data)
		}
	}()
	return nil
}

// envelope 与 OneBot 响应结构一致。
type envelope struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data,omitempty"`
	Echo    json.RawMessage `json:"echo,omitempty"`
	Message string          `json:"message,omitempty"`
}
