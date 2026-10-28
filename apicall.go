package YukiChoco

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// envelope 是 Milky 的统一响应信封（见通信规范「API 调用」）。
type envelope struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// idempotentPrefixes 列出可安全重试的端点前缀。
//
// 判断依据是「重复执行不产生额外副作用」：Milky 没有幂等键，重试 send_* 会重复
// 发送消息，重试 create_* 可能创建出第二个文件夹。
var idempotentPrefixes = []string{
	"get_", "set_", "mark_", "delete_", "move_", "rename_", "accept_", "reject_", "quit_", "persist_",
}

// Call 以泛化方式调用任意端点，用于尚未建模或协议端自扩展的 API。
//
// in 为请求参数（可为 nil，此时发送空对象 {}）；out 为响应 data 的目标（可为 nil）。
//
//	var out struct {
//	    Friends []types.FriendEntity `json:"friends"`
//	}
//	err := c.Call(ctx, "get_friend_list", map[string]any{}, &out)
func (c *Client) Call(ctx context.Context, endpoint string, in, out any) error {
	return c.call(ctx, endpoint, in, out)
}

// CallRaw 与 Call 相同，但把响应 data 原样返回为 JSON。
func (c *Client) CallRaw(ctx context.Context, endpoint string, in any) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.call(ctx, endpoint, in, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// callEndpoint 是生成代码使用的类型化调用入口。
func callEndpoint[Req any, Resp any](ctx context.Context, c *Client, endpoint string, req *Req) (*Resp, error) {
	var out Resp
	if err := c.call(ctx, endpoint, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// call 执行一次 API 调用，并按幂等性决定是否重试。
func (c *Client) call(ctx context.Context, endpoint string, in, out any) error {
	if endpoint == "" {
		return fmt.Errorf("milky: endpoint 不能为空")
	}
	if err := c.checkOpen(); err != nil {
		return err
	}

	body, err := marshalRequest(in)
	if err != nil {
		return fmt.Errorf("milky: 序列化 %s 的请求参数失败: %w", endpoint, err)
	}

	attempts := 1
	if c.retryable(endpoint) {
		attempts = c.cfg.Retry.MaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			if err := sleepContext(ctx, c.backoff(attempt-1)); err != nil {
				return err
			}
			c.logger.Debug("milky: 重试 API 调用",
				"endpoint", endpoint, "attempt", attempt, "max_attempts", attempts)
		}
		if err := c.limiter.wait(ctx); err != nil {
			return err
		}

		started := time.Now()
		data, retryable, err := c.doOnce(ctx, endpoint, body)
		elapsed := time.Since(started)
		if err != nil {
			lastErr = err
			c.logger.Warn("milky: API 调用失败",
				"endpoint", endpoint, "attempt", attempt, "elapsed_ms", elapsed.Milliseconds(), "err", err)
			if !retryable {
				return err
			}
			continue
		}
		c.logger.Debug("milky: API 调用成功",
			"endpoint", endpoint, "attempt", attempt, "elapsed_ms", elapsed.Milliseconds())
		return decodeData(endpoint, data, out)
	}
	return lastErr
}

// doOnce 执行单次 HTTP 往返。
//
// 返回值 retryable 表示「这次失败值得重试」，仅网络错误与 5xx 为真。
func (c *Client) doOnce(ctx context.Context, endpoint string, body []byte) (json.RawMessage, bool, error) {
	reqCtx := ctx
	if c.cfg.APITimeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.cfg.APITimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.endpointURL(endpoint), bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("milky: 构造 %s 请求失败: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if c.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}

	c.dump("request", endpoint, body)

	resp, err := c.http.Do(req)
	if err != nil {
		// 用户取消或超时：不重试。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, true, fmt.Errorf("milky: 调用 %s 失败: %w", endpoint, err)
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxResponseBytes))
	if readErr != nil {
		return nil, true, fmt.Errorf("milky: 读取 %s 的响应失败: %w", endpoint, readErr)
	}
	c.dump("response", endpoint, raw)

	switch resp.StatusCode {
	case http.StatusOK:
		// 继续解析信封。
	case http.StatusUnauthorized:
		return nil, false, ErrUnauthorized
	case http.StatusNotFound:
		return nil, false, fmt.Errorf("%w: %s", ErrEndpointNotFound, endpoint)
	case http.StatusUnsupportedMediaType:
		return nil, false, ErrUnsupportedMediaType
	default:
		err := fmt.Errorf("%w: HTTP %d（%s）", ErrUnexpectedStatus, resp.StatusCode, c.redact(raw))
		return nil, resp.StatusCode >= http.StatusInternalServerError, err
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false, fmt.Errorf("%w: %s: %v", ErrInvalidResponse, endpoint, err)
	}
	if env.Status != "ok" || env.Retcode != RetcodeOK {
		return nil, false, &APIError{
			Endpoint: endpoint,
			Retcode:  env.Retcode,
			Message:  env.Message,
			Snippet:  c.redact(raw),
		}
	}
	return env.Data, false, nil
}

// marshalRequest 序列化请求参数；nil 或 nil 指针一律发送空对象 {}。
func marshalRequest(in any) ([]byte, error) {
	if in == nil {
		return []byte("{}"), nil
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || string(body) == "null" {
		return []byte("{}"), nil
	}
	return body, nil
}

// decodeData 把响应 data 写入 out。
func decodeData(endpoint string, data json.RawMessage, out any) error {
	if out == nil {
		return nil
	}
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage("{}")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("milky: 解析 %s 的响应数据失败: %w", endpoint, err)
	}
	return nil
}

// retryable 报告某端点是否允许自动重试。
func (c *Client) retryable(endpoint string) bool {
	if c.cfg.Retry.MaxAttempts <= 1 {
		return false
	}
	if c.cfg.Retry.RetryNonIdempotent {
		return true
	}
	for _, prefix := range idempotentPrefixes {
		if strings.HasPrefix(endpoint, prefix) {
			return true
		}
	}
	return false
}

// backoff 计算第 attempt 次重试前的等待时间（指数退避 + 抖动）。
func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.Retry.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= c.cfg.Retry.MaxDelay {
			break
		}
	}
	if d > c.cfg.Retry.MaxDelay {
		d = c.cfg.Retry.MaxDelay
	}
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(d) * jitter)
}

// endpointURL 拼接 API 地址。
func (c *Client) endpointURL(endpoint string) string {
	return c.cfg.BaseURL + "/api/" + endpoint
}

// dump 在启用调试输出时写入一行脱敏信息。
func (c *Client) dump(kind, endpoint string, body []byte) {
	if c.cfg.DebugDump == nil {
		return
	}
	fmt.Fprintf(c.cfg.DebugDump, "{\"kind\":%q,\"endpoint\":%q,\"body\":%q}\n", kind, endpoint, c.redact(body))
}

// redact 移除响应中的 access_token 并截断，供日志与错误信息使用。
func (c *Client) redact(b []byte) string {
	s := string(b)
	if c.cfg.AccessToken != "" {
		s = strings.ReplaceAll(s, c.cfg.AccessToken, "***")
	}
	const limit = 256
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

// sleepContext 等待 d，或在 ctx 结束时提前返回。
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
