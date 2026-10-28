package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultSSEMaxEventBytes = 16 << 20

// SSEConfig 是 SSE（Server-Sent Events）事件源的配置。
type SSEConfig struct {
	// URL 是事件通道地址，例如 http://127.0.0.1:6700/event。
	URL string

	// AccessToken 用于鉴权：优先 Authorization: Bearer，401 时降级为
	// ?access_token= 查询参数。
	AccessToken string

	// Header 是额外的请求头。
	Header http.Header

	// HTTPClient 用于建立流式请求。零值使用内置客户端。
	HTTPClient *http.Client

	// IdleTimeout 是读空闲超时：超过该时间没有收到任何数据即判定连接已死。
	// 零值默认 90s。
	IdleTimeout time.Duration

	// MaxMessageBytes 限制单条事件大小。零值默认 16 MiB。
	MaxMessageBytes int64

	// Logger 用于传输层日志。零值丢弃。
	Logger *slog.Logger
}

func (c *SSEConfig) setDefaults() {
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultSSEMaxEventBytes
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

// SSE 是 Server-Sent Events 事件源。
//
// 按 Milky 通信规范：服务端返回 text/event-stream，每条事件是
// `event: milky_event` + 若干 `data:` 行，以空行结束。注释行（以 `:` 开头）
// 用于保活，会被忽略。
type SSE struct {
	cfg SSEConfig
}

// NewSSE 创建 SSE 事件源。
func NewSSE(cfg SSEConfig) *SSE {
	cfg.setDefaults()
	return &SSE{cfg: cfg}
}

// Name 实现 Source。
func (s *SSE) Name() string { return SourceSSE }

// Connect 实现 Source：建立一个长连接。
func (s *SSE) Connect(ctx context.Context) (Session, error) {
	resp, err := s.open(ctx, false)
	if err != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized && s.cfg.AccessToken != "" {
		// 回退到 query 参数鉴权（部分环境无法自定义请求头）。
		s.cfg.Logger.Debug("milky/transport: SSE 的 Authorization 头被拒，改用 query 参数重试")
		resp, err = s.open(ctx, true)
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
		}
		if resp != nil {
			return nil, fmt.Errorf("%w: %d %s", &StatusError{StatusCode: resp.StatusCode, Status: resp.Status}, resp.StatusCode, resp.Status)
		}
		return nil, fmt.Errorf("milky/transport: 连接 SSE %s 失败: %w", s.cfg.URL, err)
	}

	sess := &sseSession{
		body:   resp.Body,
		idle:   s.cfg.IdleTimeout,
		max:    s.cfg.MaxMessageBytes,
		logger: s.cfg.Logger,
		events: make(chan []byte, 16),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}
	go sess.run()
	return sess, nil
}

// open 发起 SSE 请求。useQueryToken 为 true 时把 token 放进查询参数。
func (s *SSE) open(ctx context.Context, useQueryToken bool) (*http.Response, error) {
	target := s.cfg.URL
	header := s.cfg.Header.Clone()
	if header == nil {
		header = http.Header{}
	}
	if s.cfg.AccessToken != "" && !useQueryToken {
		header.Set("Authorization", "Bearer "+s.cfg.AccessToken)
	}
	if useQueryToken && s.cfg.AccessToken != "" {
		u, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("access_token", s.cfg.AccessToken)
		u.RawQuery = q.Encode()
		target = u.String()
	}
	header.Set("Accept", "text/event-stream")
	header.Set("Cache-Control", "no-cache")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header = header

	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(strings.NewReader(string(body)))
		return resp, fmt.Errorf("milky/transport: SSE 握手返回 HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		return resp, fmt.Errorf("milky/transport: SSE 响应的 Content-Type 为 %q，期望 text/event-stream", ct)
	}
	return resp, nil
}

// sseSession 是一条 SSE 连接。
type sseSession struct {
	body   io.ReadCloser
	idle   time.Duration
	max    int64
	logger *slog.Logger

	events chan []byte
	done   chan struct{}
	closed chan struct{}

	mu        sync.Mutex
	err       error
	closeOnce sync.Once
}

// run 解析事件流。
func (s *sseSession) run() {
	defer close(s.done)
	err := s.readEvents()
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// readEvents 按 SSE 规范逐行解析。
func (s *sseSession) readEvents() error {
	br := bufio.NewReaderSize(s.body, 64<<10)
	var dataLines []string
	var size int64

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("milky/transport: SSE 连接被对端关闭: %w", io.EOF)
			}
			select {
			case <-s.closed:
				return ErrClosed
			default:
			}
			return fmt.Errorf("milky/transport: 读取 SSE 流失败: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// 空行：一条事件结束。
			if len(dataLines) == 0 {
				continue
			}
			payload := strings.Join(dataLines, "\n")
			dataLines = nil
			size = 0
			if strings.TrimSpace(payload) == "" {
				continue
			}
			select {
			case s.events <- []byte(payload):
			case <-s.closed:
				return ErrClosed
			}
			continue
		}

		// 注释行（常用于保活）。
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			size += int64(len(value))
			if s.max > 0 && size > s.max {
				return fmt.Errorf("milky/transport: SSE 事件超过大小上限 %d 字节", s.max)
			}
			dataLines = append(dataLines, value)
		case "event", "id", "retry":
			// Milky 固定使用 milky_event，这里不区分事件名。
		}
	}
}

// Next 实现 Session。
func (s *sseSession) Next(ctx context.Context) ([]byte, error) {
	idle := time.NewTimer(s.idle)
	defer idle.Stop()

	resetIdle := func() {
		if !idle.Stop() {
			select {
			case <-idle.C:
			default:
			}
		}
		idle.Reset(s.idle)
	}

	for {
		select {
		case raw := <-s.events:
			resetIdle()
			return raw, nil

		case <-s.done:
			// 先排空已解析但未消费的事件。
			select {
			case raw := <-s.events:
				return raw, nil
			default:
			}
			s.mu.Lock()
			err := s.err
			s.mu.Unlock()
			if err == nil {
				err = ErrClosed
			}
			return nil, err

		case <-idle.C:
			_ = s.Close()
			return nil, fmt.Errorf("milky/transport: SSE 读空闲超时（%s）", s.idle)

		case <-ctx.Done():
			_ = s.Close()
			return nil, ctx.Err()
		}
	}
}

// Close 实现 Session，可重复调用。
func (s *sseSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closed)
		err = s.body.Close()
	})
	return err
}
