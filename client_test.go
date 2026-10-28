package YukiChoco

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/types"
)

// testConfig 返回一份指向假协议端、重试间隔极短的配置。
func testConfig(baseURL string, tweak func(*Config)) Config {
	cfg := Config{
		BaseURL:     baseURL,
		AccessToken: "secret-token",
		APITimeout:  2 * time.Second,
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Millisecond,
			MaxDelay:    5 * time.Millisecond,
		},
	}
	if tweak != nil {
		tweak(&cfg)
	}
	return cfg
}

// newTestClient 启动假协议端并返回客户端。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(testConfig(srv.URL, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// okEnvelope 构造成功信封。
func okEnvelope(data string) string {
	return `{"status":"ok","retcode":0,"data":` + data + `}`
}

func TestCallRequestShape(t *testing.T) {
	var bodySeen, authSeen, ctSeen, methodSeen, pathSeen string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodySeen = strings.TrimSpace(string(raw))
		authSeen = r.Header.Get("Authorization")
		ctSeen = r.Header.Get("Content-Type")
		methodSeen = r.Method
		pathSeen = r.URL.Path
		_, _ = w.Write([]byte(okEnvelope(`{"uin":123456,"nickname":"测试机器人"}`)))
	})

	info, err := c.System().GetLoginInfo(context.Background())
	if err != nil {
		t.Fatalf("GetLoginInfo: %v", err)
	}
	if methodSeen != http.MethodPost {
		t.Errorf("method = %s, want POST", methodSeen)
	}
	if pathSeen != "/api/get_login_info" {
		t.Errorf("path = %s", pathSeen)
	}
	if !strings.HasPrefix(ctSeen, "application/json") {
		t.Errorf("Content-Type = %q", ctSeen)
	}
	if authSeen != "Bearer secret-token" {
		t.Errorf("Authorization = %q", authSeen)
	}
	// 无参端点也必须发送空对象。
	if bodySeen != "{}" {
		t.Errorf("body = %q, want {}", bodySeen)
	}
	if info.Uin != 123456 || info.Nickname != "测试机器人" {
		t.Errorf("响应解码错误: %+v", info)
	}
}

func TestCallRequestBody(t *testing.T) {
	var bodySeen string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodySeen = strings.TrimSpace(string(raw))
		_, _ = w.Write([]byte(okEnvelope(`{"message_seq":23333,"time":1700000000}`)))
	})

	res, err := c.Group().SendMessage(context.Background(), 987654321,
		&types.OutgoingReplySegment{MessageSeq: 23332},
		&types.OutgoingTextSegment{Text: "你好"},
	)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if res.MessageSeq != 23333 {
		t.Errorf("MessageSeq = %d", res.MessageSeq)
	}
	want := `{"group_id":987654321,"message":[{"type":"reply","data":{"message_seq":23332}},{"type":"text","data":{"text":"你好"}}]}`
	if bodySeen != want {
		t.Errorf("body =\n %s\nwant\n %s", bodySeen, want)
	}
}

func TestAPIErrorRetcodes(t *testing.T) {
	cases := []struct {
		name    string
		retcode int
		want    error
	}{
		{"参数错误", RetcodeBadRequest, ErrBadRequest},
		{"未登录", RetcodeNotLoggedIn, ErrNotLoggedIn},
		{"资源不存在", RetcodeNotFound, ErrNotFound},
		{"自定义错误", -500, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"status":"failed","retcode":` +
					itoa(tc.retcode) + `,"message":"出错了"}`))
			})
			_, err := c.System().GetLoginInfo(context.Background())
			if err == nil {
				t.Fatal("期望失败")
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("错误类型 = %T, want *APIError", err)
			}
			if apiErr.Retcode != tc.retcode || apiErr.Endpoint != "get_login_info" {
				t.Errorf("APIError = %+v", apiErr)
			}
			if !strings.Contains(apiErr.Error(), "出错了") {
				t.Errorf("Error() 未包含协议端信息: %s", apiErr.Error())
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(%v, %v) = false", err, tc.want)
			}
		})
	}
}

func TestHTTPStatusErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusNotFound, ErrEndpointNotFound},
		{http.StatusUnsupportedMediaType, ErrUnsupportedMediaType},
		{http.StatusServiceUnavailable, ErrUnexpectedStatus},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			})
			// 用非幂等端点，避免 5xx 触发重试拖慢测试。
			err := c.Call(context.Background(), "send_group_message", nil, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is = false, err = %v", err)
			}
		})
	}
}

func TestRetryIdempotentOnly(t *testing.T) {
	t.Run("幂等端点重试", func(t *testing.T) {
		var calls int32
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) < 3 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(okEnvelope(`{"uin":1,"nickname":"n"}`)))
		})
		if _, err := c.System().GetLoginInfo(context.Background()); err != nil {
			t.Fatalf("期望重试后成功: %v", err)
		}
		if got := atomic.LoadInt32(&calls); got != 3 {
			t.Errorf("调用次数 = %d, want 3", got)
		}
	})

	t.Run("发送端点不重试", func(t *testing.T) {
		var calls int32
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusBadGateway)
		})
		_, err := c.Group().SendMessage(context.Background(), 1, &types.OutgoingTextSegment{Text: "x"})
		if !errors.Is(err, ErrUnexpectedStatus) {
			t.Fatalf("err = %v", err)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("调用次数 = %d, want 1（send_* 不得重试）", got)
		}
	})

	t.Run("显式允许非幂等重试", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&calls, 1) < 2 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(okEnvelope(`{"message_seq":1,"time":1}`)))
		}))
		defer srv.Close()
		c, err := New(testConfig(srv.URL, func(cfg *Config) { cfg.Retry.RetryNonIdempotent = true }))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer c.Close()
		if _, err := c.Group().SendMessage(context.Background(), 1, &types.OutgoingTextSegment{Text: "x"}); err != nil {
			t.Fatalf("期望重试后成功: %v", err)
		}
		if got := atomic.LoadInt32(&calls); got != 2 {
			t.Errorf("调用次数 = %d, want 2", got)
		}
	})
}

func TestContextCanceledBeforeCall(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(okEnvelope(`{}`)))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.System().GetLoginInfo(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("已取消的 ctx 不应发出请求，实际发出 %d 次", got)
	}
}

func TestCallEscapeHatch(t *testing.T) {
	var bodySeen string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodySeen = strings.TrimSpace(string(raw))
		_, _ = w.Write([]byte(okEnvelope(`{"friends":[{"user_id":1,"nickname":"a","sex":"unknown","qid":"","remark":"","category":{"category_id":0,"category_name":""}}]}`)))
	})

	var out struct {
		Friends []types.FriendEntity `json:"friends"`
	}
	if err := c.Call(context.Background(), "get_friend_list", map[string]any{"no_cache": true}, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if bodySeen != `{"no_cache":true}` {
		t.Errorf("body = %s", bodySeen)
	}
	if len(out.Friends) != 1 || out.Friends[0].UserID != 1 {
		t.Errorf("out = %+v", out)
	}

	raw, err := c.CallRaw(context.Background(), "get_login_info", nil)
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if !strings.Contains(string(raw), "friends") {
		t.Errorf("CallRaw = %s", raw)
	}
}

func TestTokenNeverLeaks(t *testing.T) {
	var logs bytes.Buffer
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// 假设协议端把请求头原样回显到错误信息里。
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"failed","message":"bad token: secret-token"}`))
	})
	_ = c.Close()

	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c2, err := New(testConfig(c.cfg.BaseURL, func(cfg *Config) {
		cfg.Logger = logger
		cfg.DebugDump = &logs
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c2.Close()

	_, callErr := c2.System().GetLoginInfo(context.Background())
	if callErr == nil {
		t.Fatal("期望失败")
	}
	// 错误信息、调试输出、日志都不允许出现 token。
	if strings.Contains(callErr.Error(), "secret-token") {
		t.Errorf("错误信息泄露 token: %s", callErr.Error())
	}
	if strings.Contains(logs.String(), "secret-token") {
		t.Errorf("日志/调试输出泄露 token:\n%s", logs.String())
	}
	if strings.Contains(c2.Config().String(), "secret-token") {
		t.Errorf("Config.String() 泄露 token: %s", c2.Config().String())
	}
	// 但调试输出应包含把 token 替换后的痕迹，证明脱敏逻辑生效。
	if !strings.Contains(logs.String(), "***") {
		t.Errorf("调试输出缺少脱敏标记:\n%s", logs.String())
	}
}

func TestRateLimit(t *testing.T) {
	b := newTokenBucket(RateLimitConfig{QPS: 1, Burst: 1})
	ctx := context.Background()
	if err := b.wait(ctx); err != nil {
		t.Fatalf("首个令牌应立即可用: %v", err)
	}
	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := b.wait(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}

	// QPS<=0 表示不限流。
	var nilBucket *tokenBucket
	if err := nilBucket.wait(ctx); err != nil {
		t.Fatalf("不限流时应直接返回: %v", err)
	}
}

func TestRateLimitAppliesToCalls(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(okEnvelope(`{"uin":1,"nickname":"n"}`)))
	}))
	defer srv.Close()
	c, err := New(testConfig(srv.URL, func(cfg *Config) {
		cfg.RateLimit = RateLimitConfig{QPS: 50, Burst: 1}
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	started := time.Now()
	for i := 0; i < 4; i++ {
		if _, err := c.System().GetLoginInfo(context.Background()); err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i, err)
		}
	}
	if elapsed := time.Since(started); elapsed < 40*time.Millisecond {
		t.Errorf("限流未生效，4 次调用仅耗时 %s", elapsed)
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("调用次数 = %d, want 4", got)
	}
}

func TestInitCachesProtocolInfo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/get_impl_info":
			_, _ = w.Write([]byte(okEnvelope(`{"impl_name":"Lagrange.Milky","impl_version":"1.0.0","qq_protocol_version":"9.9.9","qq_protocol_type":"linux","milky_version":"1.3"}`)))
		case "/api/get_login_info":
			_, _ = w.Write([]byte(okEnvelope(`{"uin":10001,"nickname":"bot"}`)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if c.SelfID() != 10001 {
		t.Errorf("SelfID = %d", c.SelfID())
	}
	info, ok := c.ProtocolInfo()
	if !ok {
		t.Fatal("ProtocolInfo 未缓存")
	}
	if info.ImplName != "Lagrange.Milky" || info.MilkyVersion != "1.3" {
		t.Errorf("ProtocolInfo = %+v", info)
	}
	// 返回的是副本，外部修改不应影响缓存。
	info.ImplName = "hacked"
	if again, _ := c.ProtocolInfo(); again.ImplName != "Lagrange.Milky" {
		t.Error("ProtocolInfo 应该返回副本")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(okEnvelope(`{}`)))
	})
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("二次 Close 应幂等: %v", err)
	}
	if _, err := c.System().GetLoginInfo(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestConfigDefaultsAndValidation(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"空地址", "", true},
		{"非 http scheme", "ftp://example.com", true},
		{"缺少主机", "http://", true},
		{"带尾斜杠", "http://127.0.0.1:6700/", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(Config{BaseURL: tc.baseURL})
			if tc.wantErr {
				if err == nil {
					t.Fatal("期望校验失败")
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer c.Close()
			if strings.HasSuffix(c.Config().BaseURL, "/") {
				t.Errorf("BaseURL 未去掉尾斜杠: %s", c.Config().BaseURL)
			}
			cfg := c.Config()
			if cfg.APITimeout != 15*time.Second || cfg.Retry.MaxAttempts != 3 || cfg.MaxResponseBytes != 16<<20 {
				t.Errorf("默认值未填充: %+v", cfg)
			}
			if cfg.UserAgent == "" || !strings.Contains(cfg.UserAgent, types.ProtocolVersion) {
				t.Errorf("UserAgent = %q", cfg.UserAgent)
			}
		})
	}
}

func TestMarshalRequestNilPointer(t *testing.T) {
	// 泛型方法在无请求参数时传入 nil 指针，必须序列化为 {} 而不是 null。
	var req *types.GetUserProfileRequest
	body, err := marshalRequest(req)
	if err != nil {
		t.Fatalf("marshalRequest: %v", err)
	}
	if string(body) != "{}" {
		t.Fatalf("body = %s, want {}", body)
	}
}

func TestDecodeDataEmpty(t *testing.T) {
	var out struct {
		Uin types.UserID `json:"uin"`
	}
	if err := decodeData("x", nil, &out); err != nil {
		t.Fatalf("空 data 不应报错: %v", err)
	}
	if err := decodeData("x", json.RawMessage(`{"uin":5}`), &out); err != nil || out.Uin != 5 {
		t.Fatalf("decodeData = %v, uin = %d", err, out.Uin)
	}
}

func TestBackoffBounds(t *testing.T) {
	c := &Client{cfg: Config{Retry: RetryConfig{BaseDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond}}}
	for attempt := 1; attempt <= 6; attempt++ {
		d := c.backoff(attempt)
		if d < 80*time.Millisecond || d > 360*time.Millisecond {
			t.Fatalf("attempt %d 的退避 = %s，超出 [0.8*base, 1.2*max]", attempt, d)
		}
	}
}

// itoa 避免在测试里引入 strconv 只为一处调用。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
