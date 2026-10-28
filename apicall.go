package YukiChoco

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/WinterChocolates/YukiChoco/transport"
)

// envelope 是 OneBot v11 的 API 响应。
type envelope struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Echo    json.RawMessage `json:"echo"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
}

// Call 以泛化方式调用任意 OneBot action。
//
// params 为请求参数（nil 发送空对象 {}）；返回响应 data 的原始 JSON。
// 未建模的动作、扩展动作都可以用它调用。
func (c *Client) Call(ctx context.Context, action string, params any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.callInto(ctx, action, params, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CallInto 与 Call 相同，但把响应 data 解码到 out。
func (c *Client) CallInto(ctx context.Context, action string, params, out any) error {
	return c.callInto(ctx, action, params, out)
}

func (c *Client) callInto(ctx context.Context, action string, params, out any) error {
	if strings.TrimSpace(action) == "" {
		return errors.New("yukichoco: action 不能为空")
	}
	if err := c.checkOpen(); err != nil {
		return err
	}
	body, err := marshalParams(params)
	if err != nil {
		return fmt.Errorf("yukichoco: 序列化 %s 的参数失败: %w", action, err)
	}

	attempts := 1
	if c.idempotent(action) || c.cfg.Retry.RetryNonIdempotent {
		attempts = c.cfg.Retry.MaxAttempts
		if attempts < 1 {
			attempts = 1
		}
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			delay := c.retryDelay(attempt - 1)
			c.logger.Debug("yukichoco: 重试 API 调用",
				"action", action, "attempt", attempt, "max", attempts, "delay", delay)
			if !sleepContext(ctx, delay) {
				return ctx.Err()
			}
		}
		if err := c.limiter.wait(ctx); err != nil {
			return err
		}

		started := time.Now()
		data, retryable, err := c.callOnce(ctx, action, body)
		elapsed := time.Since(started)
		if err == nil {
			c.logger.Debug("yukichoco: API 调用成功",
				"action", action, "attempt", attempt, "elapsed_ms", elapsed.Milliseconds())
			return decodeData(action, data, out)
		}
		lastErr = err
		c.logger.Warn("yukichoco: API 调用失败",
			"action", action, "attempt", attempt, "elapsed_ms", elapsed.Milliseconds(), "err", err)
		if !retryable || ctx.Err() != nil {
			return err
		}
	}
	return lastErr
}

// callOnce 执行一次调用：优先 HTTP，其次 WS。
func (c *Client) callOnce(ctx context.Context, action string, body []byte) (json.RawMessage, bool, error) {
	if c.cfg.HTTP.APIBaseURL != "" {
		return c.httpOnce(ctx, action, body)
	}
	return c.wsOnce(ctx, action, body)
}

func (c *Client) httpOnce(ctx context.Context, action string, body []byte) (json.RawMessage, bool, error) {
	reqCtx := ctx
	if c.cfg.APITimeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.cfg.APITimeout)
		defer cancel()
	}

	endpoint := c.cfg.HTTP.APIBaseURL + "/" + url.PathEscape(action)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("yukichoco: 构造 %s 请求失败: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	c.dump("request", action, body)

	resp, err := c.http.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, true, fmt.Errorf("yukichoco: 调用 %s 失败: %w", action, err)
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxResponseBytes))
	if readErr != nil {
		return nil, true, fmt.Errorf("yukichoco: 读取 %s 响应失败: %w", action, readErr)
	}
	c.dump("response", action, raw)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
		return json.RawMessage("null"), false, nil
	case http.StatusUnauthorized:
		return nil, false, ErrUnauthorized
	case http.StatusNotFound:
		return nil, false, fmt.Errorf("%w: %s", ErrActionNotFound, action)
	default:
		e := fmt.Errorf("%w: HTTP %d（%s）", ErrUnexpectedStatus, resp.StatusCode, snippet(raw))
		return nil, resp.StatusCode >= http.StatusInternalServerError, e
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false, fmt.Errorf("%w: %s: %v", ErrInvalidResponse, action, err)
	}
	if err := checkEnvelope(action, env, raw); err != nil {
		return nil, false, err
	}
	return env.Data, false, nil
}

func (c *Client) wsOnce(ctx context.Context, action string, body []byte) (json.RawMessage, bool, error) {
	conn, err := c.currentAPIConn()
	if err != nil {
		return nil, false, err
	}
	reqCtx := ctx
	if c.cfg.APITimeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.cfg.APITimeout)
		defer cancel()
	}

	if conn == nil && c.reverse == nil {
		return nil, false, ErrNotConnected
	}

	echo := fmt.Sprintf("yukichoco-%d", c.echoSeq.Add(1))
	payload, err := json.Marshal(map[string]any{
		"action": action,
		"params": json.RawMessage(body),
		"echo":   echo,
	})
	if err != nil {
		return nil, false, fmt.Errorf("yukichoco: 构造 %s 请求失败: %w", action, err)
	}

	ch := make(chan *wsResponse, 1)
	c.pendingMu.Lock()
	c.pending[echo] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, echo)
		c.pendingMu.Unlock()
	}()

	if conn != nil {
		if err := conn.Write(reqCtx, payload); err != nil {
			if c.checkOpen() != nil {
				return nil, false, ErrClosed
			}
			return nil, true, err
		}
	} else if c.reverse != nil {
		if err := c.reverse.Send(reqCtx, payload); err != nil {
			if errors.Is(err, transport.ErrNotConnected) {
				return nil, false, ErrNotConnected
			}
			return nil, false, err
		}
	}
	c.dump("ws-request", action, payload)

	select {
	case resp := <-ch:
		if resp.err != nil {
			return nil, false, resp.err
		}
		if resp.raw != nil {
			c.dump("ws-response", action, resp.raw)
		}
		if err := checkEnvelope(action, resp.env, resp.raw); err != nil {
			return nil, false, err
		}
		return resp.env.Data, false, nil
	case <-reqCtx.Done():
		return nil, errors.Is(reqCtx.Err(), context.DeadlineExceeded), reqCtx.Err()
	case <-c.closedCh:
		return nil, false, ErrClosed
	}
}

func (c *Client) currentAPIConn() (*transport.Conn, error) {
	c.connMu.RLock()
	conn := c.apiConn
	c.connMu.RUnlock()
	if conn != nil {
		return conn, nil
	}
	if c.reverse != nil && len(c.reverse.Connections()) > 0 {
		// 反向 WS 通过 server.Send 发送，这里返回 nil 表示走 reverse。
		return nil, nil
	}
	return nil, ErrNotConnected
}

func (c *Client) idempotent(action string) bool {
	return strings.HasPrefix(action, "get_") || strings.HasPrefix(action, "can_send_")
}

func (c *Client) retryDelay(n int) time.Duration {
	d := c.cfg.Retry.BaseDelay
	for i := 1; i < n; i++ {
		d *= 2
		if d >= c.cfg.Retry.MaxDelay {
			return c.cfg.Retry.MaxDelay
		}
	}
	return d
}

func marshalParams(params any) ([]byte, error) {
	if params == nil {
		return []byte("{}"), nil
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || string(body) == "null" {
		return []byte("{}"), nil
	}
	return body, nil
}

func decodeData(action string, data json.RawMessage, out any) error {
	if out == nil {
		return nil
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("yukichoco: 解码 %s 的响应失败: %w", action, err)
	}
	return nil
}

func checkEnvelope(action string, env envelope, raw []byte) error {
	okStatus := env.Status == "ok" || env.Status == "async" || env.Status == ""
	okRetcode := env.Retcode == RetcodeOK || env.Retcode == RetcodeAsync
	if okStatus && okRetcode {
		return nil
	}
	return &APIError{
		Action:  action,
		Status:  env.Status,
		Retcode: env.Retcode,
		Message: env.Message,
		Wording: env.Wording,
		Snippet: snippet(raw),
	}
}

// callTyped 是 service 方法使用的泛型入口。
func callTyped[Req any, Resp any](ctx context.Context, c *Client, action string, req *Req) (*Resp, error) {
	var out Resp
	if err := c.callInto(ctx, action, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// callVoid 调用没有返回值的动作。
func callVoid(ctx context.Context, c *Client, action string, req any) error {
	return c.callInto(ctx, action, req, nil)
}
