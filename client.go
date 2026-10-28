package YukiChoco

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/transport"
	"github.com/WinterChocolates/YukiChoco/types"
)

// Client 是 Milky 协议端（应用端视角）的客户端。
//
// 所有导出方法都可并发调用。事件通道由 Start 启动、Shutdown 停止；
// API 调用不依赖事件通道，因此 WebHook 部署或纯发送场景可以不启动事件源。
type Client struct {
	cfg     Config
	http    *http.Client
	logger  *slog.Logger
	limiter *tokenBucket
	// ownsHTTP 表示 http 客户端由 SDK 创建，Close 时才可关闭其空闲连接。
	ownsHTTP bool

	events      *event.Dispatcher
	eventSource transport.Source

	mu          sync.RWMutex
	selfID      types.UserID
	impl        *types.GetImplInfoResponse
	closed      bool
	started     bool
	shutdown    bool
	loopCancel  context.CancelFunc
	loopDone    chan struct{}
	stateHooks  []stateHook
	nextStateID atomic.Uint64
	rawHooks    []rawHook
	nextRawID   atomic.Uint64

	state     atomic.Int32
	ready     chan struct{}
	readyOnce sync.Once
	closedCh  chan struct{}
	stateCh   chan State
	stateDone chan struct{}

	system  SystemService
	message MessageService
	friend  FriendService
	group   GroupService
	file    FileService
}

// New 创建一个客户端。配置会被校验并补齐默认值。
//
// 该函数不发起网络请求；需要协议端信息时调用 Init。
func New(cfg Config) (*Client, error) {
	ownsHTTP := cfg.HTTPClient == nil
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	c := &Client{
		cfg:       cfg,
		http:      cfg.HTTPClient,
		logger:    cfg.Logger,
		limiter:   newTokenBucket(cfg.RateLimit),
		ownsHTTP:  ownsHTTP,
		ready:     make(chan struct{}),
		closedCh:  make(chan struct{}),
		stateCh:   make(chan State, 16),
		stateDone: make(chan struct{}),
		events: event.New(event.Config{
			Workers:        cfg.Dispatch.Workers,
			Buffer:         cfg.Dispatch.Buffer,
			Mode:           cfg.Dispatch.Mode,
			Backpressure:   cfg.Dispatch.Backpressure,
			HandlerTimeout: cfg.Dispatch.HandlerTimeout,
			Logger:         cfg.Logger,
		}),
	}
	switch cfg.EventSource {
	case EventSourceWS:
		c.eventSource = transport.NewWS(transport.WSConfig{
			URL:             cfg.EventURL,
			AccessToken:     cfg.AccessToken,
			Dialer:          cfg.WSDialer,
			DialTimeout:     cfg.DialTimeout,
			IdleTimeout:     cfg.IdleTimeout,
			PingInterval:    cfg.PingInterval,
			MaxMessageBytes: cfg.MaxResponseBytes,
			Logger:          cfg.Logger,
		})
	}
	go c.notifyStates()
	c.system = newSystemService(c)
	c.message = newMessageService(c)
	c.friend = newFriendService(c)
	c.group = newGroupService(c)
	c.file = newFileService(c)

	c.logger.Debug("milky: 客户端已创建", "config", cfg)
	return c, nil
}

// Config 返回客户端使用的配置副本。
func (c *Client) Config() Config { return c.cfg }

// System 返回系统 API service。
func (c *Client) System() SystemService { return c.system }

// Message 返回消息 API service。
func (c *Client) Message() MessageService { return c.message }

// Friend 返回好友 API service。
func (c *Client) Friend() FriendService { return c.friend }

// Group 返回群聊 API service。
func (c *Client) Group() GroupService { return c.group }

// File 返回文件 API service。
func (c *Client) File() FileService { return c.file }

// Init 拉取协议端信息与登录信息并缓存，同时校验协议版本。
//
// 由 mux 之类的上层调用一次即可；之后 SelfID 与 ProtocolInfo 立即可用。
func (c *Client) Init(ctx context.Context) error {
	impl, err := c.system.GetImplInfo(ctx)
	if err != nil {
		return fmt.Errorf("milky: 获取协议端信息失败: %w", err)
	}
	login, err := c.system.GetLoginInfo(ctx)
	if err != nil {
		return fmt.Errorf("milky: 获取登录信息失败: %w", err)
	}

	c.mu.Lock()
	c.impl = impl
	c.selfID = login.Uin
	c.mu.Unlock()

	c.logger.Info("milky: 已获取协议端信息",
		"impl", impl.ImplName,
		"impl_version", impl.ImplVersion,
		"milky_version", impl.MilkyVersion,
		"qq_protocol_type", impl.QQProtocolType,
		"self_id", login.Uin.Int64(),
	)
	if impl.MilkyVersion != "" && impl.MilkyVersion != types.ProtocolVersion {
		c.logger.Warn("milky: 协议端版本与 SDK 基线不一致，新增能力可能不可用",
			"sdk_protocol", types.ProtocolVersion, "impl_protocol", impl.MilkyVersion)
	}
	return nil
}

// SelfID 返回机器人自身的 QQ 号；未调用 Init 时为零值。
func (c *Client) SelfID() types.UserID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.selfID
}

// ProtocolInfo 返回 Init 缓存的协议端信息。
func (c *Client) ProtocolInfo() (*types.GetImplInfoResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.impl == nil {
		return nil, false
	}
	info := *c.impl
	return &info, true
}

// Close 关闭客户端：之后的调用返回 ErrClosed，事件循环会被取消。
//
// 仅当 http 客户端由 SDK 创建时才会关闭其空闲连接；注入的客户端交由调用方管理。
// 需要优雅停止事件处理请用 Shutdown。
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	cancel := c.loopCancel
	close(c.closedCh)
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if c.ownsHTTP {
		c.http.CloseIdleConnections()
	}
	c.logger.Debug("milky: 客户端已关闭")
	return nil
}

// checkOpen 在客户端已关闭时返回 ErrClosed。
func (c *Client) checkOpen() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return ErrClosed
	}
	return nil
}
