package YukiChoco_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/types"
)

func envelopeJSON(status string, retcode int, data string) string {
	if data == "" {
		data = "null"
	}
	return fmt.Sprintf(`{"status":%q,"retcode":%d,"data":%s}`, status, retcode, data)
}

func newHTTPClient(t *testing.T, h http.HandlerFunc) *YukiChoco.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := YukiChoco.New(YukiChoco.Config{
		Transport: YukiChoco.TransportHTTP,
		HTTP:      YukiChoco.HTTPConfig{APIBaseURL: srv.URL},
		Retry:     YukiChoco.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHTTPCall(t *testing.T) {
	var gotAction, gotBody string
	c := newHTTPClient(t, func(rw http.ResponseWriter, r *http.Request) {
		gotAction = strings.TrimPrefix(r.URL.Path, "/")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"user_id":123,"nickname":"bot"}`)))
	})

	info, err := c.System().GetLoginInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.UserID != 123 || info.Nickname != "bot" {
		t.Fatalf("info=%+v", info)
	}
	if gotAction != "get_login_info" || gotBody != "{}" {
		t.Fatalf("action=%s body=%s", gotAction, gotBody)
	}
}

func TestHTTPCallIntoRaw(t *testing.T) {
	c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"yes":true}`)))
	})
	raw, err := c.Call(context.Background(), "can_send_image", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"yes":true}` {
		t.Fatalf("raw=%s", raw)
	}
	var out types.CanSendResult
	if err := c.CallInto(context.Background(), "can_send_image", nil, &out); err != nil || !out.Yes {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestHTTPErrorMapping(t *testing.T) {
	cases := []struct {
		status string
		code   int
		want   error
	}{
		{"failed", 100, YukiChoco.ErrBadRequest},
		{"failed", 102, YukiChoco.ErrBadRequest},
		{"failed", 103, YukiChoco.ErrNotFound},
		{"failed", 104, YukiChoco.ErrUnauthorized},
		{"failed", 200, YukiChoco.ErrForbidden},
		{"failed", 999, YukiChoco.ErrActionFailed},
	}
	for _, tc := range cases {
		c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte(envelopeJSON(tc.status, tc.code, "")))
		})
		_, err := c.System().GetStatus(context.Background())
		if !errors.Is(err, tc.want) {
			t.Errorf("retcode %d: err=%v, want %v", tc.code, err, tc.want)
		}
		var apiErr *YukiChoco.APIError
		if !errors.As(err, &apiErr) || apiErr.Retcode != tc.code {
			t.Errorf("retcode %d: APIError=%+v", tc.code, apiErr)
		}
	}
}

func TestHTTPAsyncIsSuccess(t *testing.T) {
	c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(envelopeJSON("async", 1, "null")))
	})
	if err := c.Message().Delete(context.Background(), 1); err != nil {
		t.Fatalf("async 应视为受理: %v", err)
	}
}

func TestHTTPRetriesIdempotent(t *testing.T) {
	var attempts atomic.Int32
	c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte("boom"))
			return
		}
		_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"online":true,"good":true}`)))
	})
	st, err := c.System().GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Online || attempts.Load() != 3 {
		t.Fatalf("attempts=%d status=%+v", attempts.Load(), st)
	}
}

func TestHTTPDoesNotRetrySend(t *testing.T) {
	var attempts atomic.Int32
	c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		rw.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.Message().SendGroup(context.Background(), 1, types.Message{{Type: types.SegmentText, Data: types.SegmentData{"text": "x"}}})
	if err == nil {
		t.Fatal("应失败")
	}
	if attempts.Load() != 1 {
		t.Fatalf("send 不应重试，attempts=%d", attempts.Load())
	}
}

func TestHTTPStatusErrors(t *testing.T) {
	c := newHTTPClient(t, func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/get_status") {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	})
	if _, err := c.System().GetStatus(context.Background()); !errors.Is(err, YukiChoco.ErrUnauthorized) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.System().GetVersionInfo(context.Background()); !errors.Is(err, YukiChoco.ErrActionNotFound) {
		t.Fatalf("404: %v", err)
	}
}

func TestHTTPInvalidJSON(t *testing.T) {
	c := newHTTPClient(t, func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte("not json"))
	})
	_, err := c.System().GetStatus(context.Background())
	if !errors.Is(err, YukiChoco.ErrInvalidResponse) {
		t.Fatalf("err=%v", err)
	}
}

func TestCallEmptyAction(t *testing.T) {
	c := newHTTPClient(t, func(http.ResponseWriter, *http.Request) {})
	if _, err := c.Call(context.Background(), "  ", nil); err == nil {
		t.Fatal("空 action 应报错")
	}
}

func TestDebugDump(t *testing.T) {
	var buf strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(envelopeJSON("ok", 0, "null")))
	}))
	defer srv.Close()
	c, err := YukiChoco.New(YukiChoco.Config{
		HTTP:      YukiChoco.HTTPConfig{APIBaseURL: srv.URL},
		DebugDump: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Call(context.Background(), "get_status", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "get_status") {
		t.Fatalf("dump=%s", buf.String())
	}
}
