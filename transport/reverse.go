package transport

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ReverseConfig 是反向 WebSocket 服务端的配置。
type ReverseConfig struct {
	// Listen 是监听地址，例如 "127.0.0.1:8080"；":8080" 表示所有网卡。
	// 零值默认 "127.0.0.1:8080"。
	Listen string

	// Path 是 WebSocket 路径，零值默认 "/onebot/v11/ws"。
	Path string

	// AccessToken 非空时校验 Authorization: Bearer 或 ?access_token=。
	AccessToken string

	// CheckOrigin 可自定义 Origin 校验；零值允许所有来源（请配合 AccessToken 使用）。
	CheckOrigin func(*http.Request) bool

	// MaxMessageBytes 限制单条消息大小，零值默认 16 MiB。
	MaxMessageBytes int64

	// IdleTimeout 是读空闲超时，零值默认 90s；负值关闭。
	IdleTimeout time.Duration

	// PingInterval 是 ping 间隔，零值默认 30s；负值关闭。
	PingInterval time.Duration

	// WriteTimeout 是单次写入超时，零值默认 10s。
	WriteTimeout time.Duration

	// Logger 用于传输层日志，零值丢弃。
	Logger *slog.Logger
}

func (c *ReverseConfig) setDefaults() {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}
	if c.Path == "" {
		c.Path = "/onebot/v11/ws"
	}
	if !strings.HasPrefix(c.Path, "/") {
		c.Path = "/" + c.Path
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultMaxMessageBytes
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.PingInterval == 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// ReverseServer 接受协议端主动建立的 WebSocket 连接（反向 WS）。
type ReverseServer struct {
	cfg ReverseConfig

	upgrader websocket.Upgrader

	mu      sync.Mutex
	conns   map[*Conn]struct{}
	started bool
	srv     *http.Server
	ln      net.Listener

	onConnect    func(*Conn)
	onMessage    func(*Conn, []byte)
	onDisconnect func(*Conn)
}

// NewReverseServer 创建反向 WebSocket 服务端。
func NewReverseServer(cfg ReverseConfig) *ReverseServer {
	cfg.setDefaults()
	s := &ReverseServer{cfg: cfg, conns: make(map[*Conn]struct{})}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			if cfg.CheckOrigin != nil {
				return cfg.CheckOrigin(r)
			}
			return true
		},
	}
	return s
}

// OnConnect 设置新连接回调（连接完成握手后调用）。
func (s *ReverseServer) OnConnect(fn func(*Conn)) {
	s.mu.Lock()
	s.onConnect = fn
	s.mu.Unlock()
}

// OnMessage 设置消息回调（在读循环中同步执行，回调应尽快返回）。
func (s *ReverseServer) OnMessage(fn func(*Conn, []byte)) {
	s.mu.Lock()
	s.onMessage = fn
	s.mu.Unlock()
}

// OnDisconnect 设置断开回调。
func (s *ReverseServer) OnDisconnect(fn func(*Conn)) {
	s.mu.Lock()
	s.onDisconnect = fn
	s.mu.Unlock()
}

// Handler 返回处理 WebSocket 升级的 http.Handler。
func (s *ReverseServer) Handler() http.Handler {
	return http.HandlerFunc(s.serveWS)
}

// Start 开始监听。
func (s *ReverseServer) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("transport: 反向 WS 服务端已启动")
	}
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.ln = ln
	s.started = true
	mux := http.NewServeMux()
	mux.Handle(s.cfg.Path, s.Handler())
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	srv := s.srv
	s.mu.Unlock()

	s.cfg.Logger.Info("transport: 反向 WS 服务端已启动",
		"addr", ln.Addr().String(), "path", s.cfg.Path)

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.cfg.Logger.Error("transport: 反向 WS 服务端退出", "err", err)
		}
	}()
	return nil
}

// Addr 返回实际监听地址；未启动时返回 nil。
func (s *ReverseServer) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Shutdown 停止监听并关闭所有连接。
func (s *ReverseServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	srv := s.srv
	s.started = false
	s.srv = nil
	s.ln = nil
	s.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// Connections 返回当前连接快照。
func (s *ReverseServer) Connections() []*Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		out = append(out, c)
	}
	return out
}

// Send 通过任意一条连接发送数据。
func (s *ReverseServer) Send(ctx context.Context, data []byte) error {
	conns := s.Connections()
	if len(conns) == 0 {
		return ErrNotConnected
	}
	return conns[0].Write(ctx, data)
}

func (s *ReverseServer) serveWS(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.Header().Set("Allow", http.MethodGet)
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		s.cfg.Logger.Warn("transport: 反向 WS 鉴权失败", "remote", r.RemoteAddr)
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	ws, err := s.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		s.cfg.Logger.Warn("transport: 反向 WS 升级失败", "remote", r.RemoteAddr, "err", err)
		return
	}

	cfg := DialConfig{
		AccessToken:     s.cfg.AccessToken,
		IdleTimeout:     s.cfg.IdleTimeout,
		PingInterval:    s.cfg.PingInterval,
		WriteTimeout:    s.cfg.WriteTimeout,
		MaxMessageBytes: s.cfg.MaxMessageBytes,
		Logger:          s.cfg.Logger,
	}
	conn := newConn(ws, cfg, r.Header.Clone())
	conn.start()

	s.mu.Lock()
	s.conns[conn] = struct{}{}
	onConnect := s.onConnect
	s.mu.Unlock()

	s.cfg.Logger.Info("transport: 反向 WS 已连接",
		"remote", conn.RemoteAddr(),
		"self_id", r.Header.Get("X-Self-ID"),
		"role", r.Header.Get("X-Client-Role"),
	)
	if onConnect != nil {
		onConnect(conn)
	}

	for {
		data, err := conn.Read(context.Background())
		if err != nil {
			break
		}
		s.mu.Lock()
		onMessage := s.onMessage
		s.mu.Unlock()
		if onMessage != nil {
			onMessage(conn, data)
		}
	}

	s.mu.Lock()
	delete(s.conns, conn)
	onDisconnect := s.onDisconnect
	s.mu.Unlock()

	_ = conn.Close()
	s.cfg.Logger.Info("transport: 反向 WS 已断开", "remote", conn.RemoteAddr())
	if onDisconnect != nil {
		onDisconnect(conn)
	}
}

func (s *ReverseServer) authorized(r *http.Request) bool {
	if s.cfg.AccessToken == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	if got == "" {
		got = "Bearer " + r.URL.Query().Get("access_token")
	}
	want := "Bearer " + s.cfg.AccessToken
	// 常量时间比较，避免逐字符时序泄漏。
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
