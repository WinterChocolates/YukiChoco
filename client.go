package YukiChoco

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/transport"
	"github.com/WinterChocolates/YukiChoco/types"
)

// Client 是 OneBot v11 客户端。
//
// 所有导出方法都可并发调用。Start 启动事件通道；HTTP API 不依赖 Start，
// 但正向 / 反向 WS 模式下的 WS API 调用需要连接已建立。
type Client struct {
	cfg      Config
	http     *http.Client
	logger   *slog.Logger
	limiter  *tokenBucket
	ownsHTTP bool

	events *event.Dispatcher

	reverse *transport.ReverseServer
	webhook *transport.Webhook

	connMu    sync.RWMutex
	eventConn *transport.Conn
	apiConn   *transport.Conn

	pendingMu sync.Mutex
	pending   map[string]chan *wsResponse
	echoSeq   atomic.Uint64

	selfID atomic.Int64

	infoMu  sync.RWMutex
	login   *types.LoginInfo
	version *types.VersionInfo

	started   atomic.Bool
	closedCh  chan struct{}
	closeOnce sync.Once

	loopCtx    context.Context
	loopCancel context.CancelFunc
	loopWG     sync.WaitGroup

	state     atomic.Int32
	stateCh   chan State
	stateMu   sync.Mutex
	stateDone bool
	ready     chan struct{}
	readyOnce sync.Once

	dumpMu sync.Mutex

	system  *SystemAPI
	message *MessageAPI
	friend  *FriendAPI
	group   *GroupAPI
	file    *FileAPI
}

// New 创建客户端。配置会被校验并补齐默认值；该函数不发起网络连接。
func New(cfg Config) (*Client, error) {
	ownsHTTP := cfg.HTTPClient == nil
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	c := &Client{
		cfg:      cfg,
		http:     cfg.HTTPClient,
		logger:   cfg.Logger,
		limiter:  newTokenBucket(cfg.RateLimit),
		ownsHTTP: ownsHTTP,
		pending:  make(map[string]chan *wsResponse),
		closedCh: make(chan struct{}),
		stateCh:  make(chan State, 16),
		ready:    make(chan struct{}),
	}
	c.events = event.New(event.Config{
		Workers:        cfg.Dispatch.Workers,
		Buffer:         cfg.Dispatch.Buffer,
		Mode:           cfg.Dispatch.Mode,
		Backpressure:   cfg.Dispatch.Backpressure,
		HandlerTimeout: cfg.Dispatch.HandlerTimeout,
		KeyFunc:        cfg.Dispatch.KeyFunc,
		Logger:         cfg.Logger,
	})

	switch cfg.Transport {
	case TransportReverseWS:
		c.reverse = transport.NewReverseServer(transport.ReverseConfig{
			Listen:          cfg.ReverseWS.Listen,
			Path:            cfg.ReverseWS.Path,
			AccessToken:     cfg.AccessToken,
			CheckOrigin:     cfg.ReverseWS.CheckOrigin,
			MaxMessageBytes: cfg.MaxResponseBytes,
			IdleTimeout:     cfg.ReverseWS.IdleTimeout,
			PingInterval:    cfg.ReverseWS.PingInterval,
			Logger:          cfg.Logger,
		})
		c.reverse.OnMessage(func(_ *transport.Conn, data []byte) { c.handleFrame(data) })
		c.reverse.OnConnect(c.handleReverseConnect)
	case TransportHTTP:
		c.webhook = transport.NewWebhook(transport.WebhookConfig{
			AccessToken: cfg.AccessToken,
			VerifyToken: cfg.Webhook.VerifyToken,
			Secret:      cfg.Webhook.Secret,
			MaxBytes:    cfg.MaxResponseBytes,
			Logger:      cfg.Logger,
		})
		c.webhook.OnEvent(func(data []byte) { c.handleFrame(data) })
	}

	c.system = &SystemAPI{c: c}
	c.message = &MessageAPI{c: c}
	c.friend = &FriendAPI{c: c}
	c.group = &GroupAPI{c: c}
	c.file = &FileAPI{c: c}

	c.logger.Debug("yukichoco: 客户端已创建", "config", cfg)
	return c, nil
}

// Config 返回配置副本。
func (c *Client) Config() Config { return c.cfg }

// System 返回系统 API。
func (c *Client) System() *SystemAPI { return c.system }

// Message 返回消息 API。
func (c *Client) Message() *MessageAPI { return c.message }

// Friend 返回好友 API。
func (c *Client) Friend() *FriendAPI { return c.friend }

// Group 返回群 API。
func (c *Client) Group() *GroupAPI { return c.group }

// File 返回文件 API。
func (c *Client) File() *FileAPI { return c.file }

// Events 返回事件分发器，用于高级订阅。
func (c *Client) Events() *event.Dispatcher { return c.events }

// WebhookHandler 返回接收 HTTP POST 事件的 handler。
//
// 仅在 TransportHTTP 时非 nil；调用方自行挂载到任意路径。
func (c *Client) WebhookHandler() http.Handler {
	if c.webhook == nil {
		return nil
	}
	return c.webhook.Handler()
}

// Start 启动事件通道。
//
// - HTTP / None：立即就绪（不建立事件连接；HTTP 模式需挂载 WebhookHandler）。
// - 正向 WS：后台拨号，连接建立后 WaitReady 返回。
// - 反向 WS：启动监听并立即就绪。
func (c *Client) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.checkOpen(); err != nil {
		return err
	}
	if !c.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	c.loopCtx, c.loopCancel = context.WithCancel(context.Background())

	switch c.cfg.Transport {
	case TransportHTTP, TransportNone:
		c.setState(StateConnected)
		c.markReady()
	case TransportReverseWS:
		if err := c.reverse.Start(); err != nil {
			c.loopCancel()
			c.started.Store(false)
			return fmt.Errorf("yukichoco: 启动反向 WS 服务失败: %w", err)
		}
		c.setState(StateConnected)
		c.markReady()
	case TransportForwardWS:
		c.setState(StateDialing)
		c.startForward()
	}
	return nil
}

// WaitReady 等待事件通道就绪、客户端关闭或 ctx 结束。
func (c *Client) WaitReady(ctx context.Context) error {
	select {
	case <-c.closedCh:
		return ErrClosed
	default:
	}
	select {
	case <-c.closedCh:
		return ErrClosed
	case <-c.ready:
		select {
		case <-c.closedCh:
			return ErrClosed
		default:
			return nil
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// State 返回当前连接状态。
func (c *Client) State() State { return State(c.state.Load()) }

// StateChanges 返回状态变化通知（容量 16，丢弃最旧以外的策略为不阻塞发送）。
func (c *Client) StateChanges() <-chan State { return c.stateCh }

// SelfID 返回机器人 QQ 号；未收到事件或未调用 Init 时为零值。
func (c *Client) SelfID() types.UserID { return types.UserID(c.selfID.Load()) }

// LoginInfo 返回 Init 缓存的登录信息。
func (c *Client) LoginInfo() (*types.LoginInfo, bool) {
	c.infoMu.RLock()
	defer c.infoMu.RUnlock()
	if c.login == nil {
		return nil, false
	}
	v := *c.login
	return &v, true
}

// VersionInfo 返回 Init 缓存的协议端版本信息。
func (c *Client) VersionInfo() (*types.VersionInfo, bool) {
	c.infoMu.RLock()
	defer c.infoMu.RUnlock()
	if c.version == nil {
		return nil, false
	}
	v := *c.version
	return &v, true
}

// Init 拉取登录信息与版本信息并缓存，同时更新 SelfID。
//
// HTTP 模式下可直接调用；WS 模式下需连接已建立。事件通道不可用时，
// 也可以只依赖事件中的 self_id。
func (c *Client) Init(ctx context.Context) error {
	login, err := c.system.GetLoginInfo(ctx)
	if err != nil {
		return fmt.Errorf("yukichoco: 获取登录信息失败: %w", err)
	}
	version, verr := c.system.GetVersionInfo(ctx)
	if verr != nil {
		c.logger.Warn("yukichoco: 获取协议端版本失败", "err", verr)
	}

	c.infoMu.Lock()
	c.login = login
	c.version = version
	c.infoMu.Unlock()
	if !login.UserID.IsZero() {
		c.selfID.Store(login.UserID.Int64())
	}

	c.logger.Info("yukichoco: 已获取登录信息",
		"self_id", login.UserID.Int64(), "nickname", login.Nickname)
	if version != nil {
		c.logger.Info("yukichoco: 协议端版本",
			"app", version.AppName, "version", version.AppVersion,
			"protocol", version.ProtocolVersion)
	}
	return nil
}

// Shutdown 优雅关闭：取消重连、关闭连接与事件分发器，等待 handler 结束。
func (c *Client) Shutdown(ctx context.Context) error {
	var err error
	c.closeOnce.Do(func() {
		c.started.Store(false)
		if c.loopCancel != nil {
			c.loopCancel()
		}
		if c.reverse != nil {
			if e := c.reverse.Shutdown(ctx); e != nil {
				err = e
			}
		}
		if c.webhook != nil {
			c.webhook.Close()
		}

		c.connMu.Lock()
		ev, api := c.eventConn, c.apiConn
		c.eventConn, c.apiConn = nil, nil
		c.connMu.Unlock()
		if ev != nil {
			_ = ev.Close()
		}
		if api != nil && api != ev {
			_ = api.Close()
		}

		c.failPending(ErrClosed)
		c.setState(StateClosed)
		close(c.closedCh)
		if c.ownsHTTP {
			c.http.CloseIdleConnections()
		}
		if e := c.events.Close(ctx); e != nil && err == nil {
			err = e
		}
		c.loopWG.Wait()
		c.closeState()
		c.logger.Debug("yukichoco: 客户端已关闭")
	})
	return err
}

// Close 是 Shutdown 的快捷方式（最多等待 5 秒）。
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Shutdown(ctx)
}

// Router 返回通用事件路由器。
func (c *Client) Router() *event.Router { return event.NewRouter(c.events) }

// GroupRouter 返回群消息路由器。
func (c *Client) GroupRouter() *event.GroupRouter { return event.RouteGroup(c.events) }

// PrivateRouter 返回私聊消息路由器。
func (c *Client) PrivateRouter() *event.PrivateRouter { return event.RoutePrivate(c.events) }

func (c *Client) checkOpen() error {
	select {
	case <-c.closedCh:
		return ErrClosed
	default:
		return nil
	}
}

func (c *Client) setState(s State) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.stateDone {
		return
	}
	old := State(c.state.Swap(int32(s)))
	if old == s {
		return
	}
	select {
	case c.stateCh <- s:
	default:
	}
}

func (c *Client) closeState() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.stateDone {
		return
	}
	c.stateDone = true
	close(c.stateCh)
}

func (c *Client) markReady() { c.readyOnce.Do(func() { close(c.ready) }) }

func (c *Client) setConn(event bool, conn *transport.Conn) {
	c.connMu.Lock()
	if event {
		c.eventConn = conn
	} else {
		c.apiConn = conn
	}
	c.connMu.Unlock()
}

func (c *Client) clearConn(event bool, conn *transport.Conn) {
	c.connMu.Lock()
	if event && c.eventConn == conn {
		c.eventConn = nil
	}
	if !event && c.apiConn == conn {
		c.apiConn = nil
	}
	c.connMu.Unlock()
}

// startForward 启动正向 WS 连接循环。
func (c *Client) startForward() {
	f := c.cfg.ForwardWS
	if f.EventURL == "" {
		// 通用单连接：事件与 API 响应共用。
		c.loopWG.Add(1)
		go c.forwardLoop(f.URL, true, true)
		return
	}
	c.loopWG.Add(1)
	go c.forwardLoop(f.EventURL, true, false)
	// 只有需要 WS API（未配置 HTTP API）时才单独维护 API 连接。
	if c.cfg.HTTP.APIBaseURL == "" {
		c.loopWG.Add(1)
		go c.forwardLoop(f.URL, false, false)
	}
}

// forwardLoop 维护一条连接，断线后按退避重连。
func (c *Client) forwardLoop(rawURL string, isEvent, both bool) {
	defer c.loopWG.Done()
	defer func() {
		if (isEvent || both) && c.loopCtx.Err() == nil {
			c.setState(StateDisconnected)
		}
	}()
	f := c.cfg.ForwardWS
	attempt := 0

	for {
		if c.loopCtx.Err() != nil {
			return
		}
		if attempt > 0 {
			c.setState(StateReconnecting)
			delay := c.backoff(attempt)
			c.logger.Warn("yukichoco: WebSocket 重连",
				"url", rawURL, "attempt", attempt, "delay", delay)
			if !sleepContext(c.loopCtx, delay) {
				return
			}
		}

		conn, err := transport.Dial(c.loopCtx, transport.DialConfig{
			URL:             rawURL,
			AccessToken:     c.cfg.AccessToken,
			Header:          f.Header,
			Dialer:          f.Dialer,
			DialTimeout:     f.DialTimeout,
			IdleTimeout:     f.IdleTimeout,
			PingInterval:    f.PingInterval,
			MaxMessageBytes: c.cfg.MaxResponseBytes,
			Logger:          c.logger,
		})
		if err != nil {
			if c.loopCtx.Err() != nil {
				return
			}
			attempt++
			c.logger.Warn("yukichoco: WebSocket 连接失败", "url", rawURL, "err", err)
			if f.Reconnect.Disable || (f.Reconnect.MaxAttempts > 0 && attempt >= f.Reconnect.MaxAttempts) {
				c.logger.Error("yukichoco: 放弃重连", "url", rawURL)
				return
			}
			continue
		}

		attempt = 0
		if both {
			c.setConn(true, conn)
			c.setConn(false, conn)
		} else {
			c.setConn(isEvent, conn)
		}
		if isEvent || both {
			c.setState(StateConnected)
			c.markReady()
		}
		c.logger.Info("yukichoco: WebSocket 已连接", "url", rawURL, "role", roleName(isEvent, both))

		c.readLoop(conn)

		if both {
			c.clearConn(true, conn)
			c.clearConn(false, conn)
		} else {
			c.clearConn(isEvent, conn)
		}
		_ = conn.Close()
		if c.loopCtx.Err() != nil {
			return
		}
		if isEvent || both {
			c.setState(StateReconnecting)
		}
		if f.Reconnect.Disable {
			return
		}
		attempt++
	}
}

func roleName(isEvent, both bool) string {
	if both {
		return "universal"
	}
	if isEvent {
		return "event"
	}
	return "api"
}

// readLoop 读取一条连接上的消息，直到出错。
func (c *Client) readLoop(conn *transport.Conn) {
	for {
		data, err := conn.Read(c.loopCtx)
		if err != nil {
			if c.loopCtx.Err() == nil {
				c.logger.Warn("yukichoco: WebSocket 读取结束", "remote", conn.RemoteAddr(), "err", err)
			}
			return
		}
		c.handleFrame(data)
	}
}

func (c *Client) handleReverseConnect(conn *transport.Conn) {
	if raw := conn.Header().Get("X-Self-ID"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			c.selfID.Store(id)
		}
	}
	c.setState(StateConnected)
	c.markReady()
}

// handleFrame 自动区分事件与 API 响应。
func (c *Client) handleFrame(data []byte) {
	var probe struct {
		PostType string          `json:"post_type"`
		Echo     json.RawMessage `json:"echo"`
		Status   *string         `json:"status"`
		Retcode  *int            `json:"retcode"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		c.logger.Warn("yukichoco: 收到非法 JSON 帧", "err", err, "snippet", snippet(data))
		return
	}
	if probe.PostType != "" {
		ev, err := types.ParseEvent(data)
		if err != nil {
			c.logger.Warn("yukichoco: 解析事件失败", "err", err, "snippet", snippet(data))
			return
		}
		if id := ev.EventSelfID(); !id.IsZero() {
			c.selfID.Store(id.Int64())
		}
		c.events.Dispatch(context.Background(), ev)
		return
	}
	if len(probe.Echo) > 0 {
		c.completePending(data, probe.Echo)
		return
	}
	if probe.Status != nil || probe.Retcode != nil {
		c.logger.Warn("yukichoco: 收到无 echo 的 API 响应，无法关联", "snippet", snippet(data))
		return
	}
	c.logger.Debug("yukichoco: 忽略未知帧", "snippet", snippet(data))
}

func (c *Client) completePending(data, echoRaw json.RawMessage) {
	var echo string
	if err := json.Unmarshal(echoRaw, &echo); err != nil {
		echo = string(echoRaw)
	}
	c.pendingMu.Lock()
	ch := c.pending[echo]
	c.pendingMu.Unlock()
	if ch == nil {
		c.logger.Debug("yukichoco: 收到未知 echo 的响应", "echo", echo)
		return
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		env = envelope{Status: "failed", Retcode: RetcodeAsyncFailed, Message: "响应解析失败: " + err.Error()}
	}
	select {
	case ch <- &wsResponse{env: env, raw: append([]byte(nil), data...)}:
	default:
	}
}

func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan *wsResponse)
	c.pendingMu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- &wsResponse{err: err}:
		default:
		}
	}
}

// backoff 计算第 n 次重连的等待时间。
func (c *Client) backoff(n int) time.Duration {
	rc := c.cfg.ForwardWS.Reconnect
	d := float64(rc.InitialDelay)
	for i := 1; i < n; i++ {
		d *= rc.Multiplier
		if d >= float64(rc.MaxDelay) {
			d = float64(rc.MaxDelay)
			break
		}
	}
	if rc.Jitter > 0 {
		j := rc.Jitter
		d *= 1 + (randFloat()*2-1)*j
	}
	if d < float64(10*time.Millisecond) {
		d = float64(10 * time.Millisecond)
	}
	return time.Duration(d)
}

func (c *Client) dump(kind, action string, body []byte) {
	if c.cfg.DebugDump == nil {
		return
	}
	if len(body) > 4096 {
		body = body[:4096]
	}
	c.dumpMu.Lock()
	defer c.dumpMu.Unlock()
	fmt.Fprintf(c.cfg.DebugDump, "%s %s action=%s body=%s\n", time.Now().Format(time.RFC3339Nano), kind, action, body)
}

// wsResponse 是 WS API 调用的响应。
type wsResponse struct {
	env envelope
	raw []byte
	err error
}

func randFloat() float64 { return rand.Float64() }

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ReverseAddr 返回反向 WS 的实际监听地址（仅 TransportReverseWS 且已 Start 时非 nil）。
func (c *Client) ReverseAddr() net.Addr {
	if c.reverse == nil {
		return nil
	}
	return c.reverse.Addr()
}
