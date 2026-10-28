package YukiChoco

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/mock"
)

// TestUploadPrivateFileHelpers 覆盖私聊上传的两种入口。
func TestUploadPrivateFileHelpers(t *testing.T) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("upload_private_file", func(req json.RawMessage) (any, error) {
		return map[string]any{"file_id": "priv-1"}, nil
	})

	c, err := New(eventConfig(srv.URL(), func(cfg *Config) { cfg.EventSource = EventSourceNone }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := c.UploadPrivateFileFrom(context.Background(), 300, path, "a.txt")
	if err != nil {
		t.Fatalf("UploadPrivateFileFrom: %v", err)
	}
	if res.FileID != "priv-1" {
		t.Errorf("FileID = %q", res.FileID)
	}
	call, _ := srv.LastCall("upload_private_file")
	if !strings.Contains(string(call.Body), `"user_id":300`) || !strings.Contains(string(call.Body), `"file://`) {
		t.Errorf("请求体 = %s", call.Body)
	}

	if _, err := c.UploadPrivateFileWith(context.Background(), 300, "b.txt", []byte("data")); err != nil {
		t.Fatalf("UploadPrivateFileWith: %v", err)
	}
	call, _ = srv.LastCall("upload_private_file")
	if !strings.Contains(string(call.Body), `"base64://`) {
		t.Errorf("字节来源应内联为 base64: %s", call.Body)
	}

	// 不支持的来源类型应报错且不发请求。
	if _, err := c.UploadPrivateFileWith(context.Background(), 300, "c.txt", 42); err == nil {
		t.Fatal("非法来源应报错")
	}
}

// TestDownloadFileHelpers 覆盖群文件 / 私聊文件下载。
func TestDownloadFileHelpers(t *testing.T) {
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("file-content"))
	}))
	defer blob.Close()

	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("get_group_file_download_url", func(json.RawMessage) (any, error) {
		return map[string]any{"download_url": blob.URL + "/f"}, nil
	})
	srv.Handle("get_private_file_download_url", func(json.RawMessage) (any, error) {
		return map[string]any{"download_url": blob.URL + "/p"}, nil
	})
	srv.Handle("get_group_file_list", func(json.RawMessage) (any, error) { return map[string]any{}, nil })

	c, err := New(eventConfig(srv.URL(), func(cfg *Config) { cfg.EventSource = EventSourceNone }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	rc, err := c.DownloadFile(context.Background(), 200, "f1", "")
	if err != nil {
		t.Fatalf("DownloadFile(group): %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "file-content" {
		t.Errorf("群文件内容 = %q", data)
	}

	// groupID 为 0 走私聊下载，需要 file_hash。
	rc, err = c.DownloadFile(context.Background(), 0, "p1", "hash")
	if err != nil {
		t.Fatalf("DownloadFile(private): %v", err)
	}
	data, _ = io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "file-content" {
		t.Errorf("私聊文件内容 = %q", data)
	}

	call, _ := srv.LastCall("get_private_file_download_url")
	if !strings.Contains(string(call.Body), `"file_hash":"hash"`) {
		t.Errorf("私聊下载请求体 = %s", call.Body)
	}
}

// TestDownloadFileLinksErrors 覆盖下载链接缺失与 HTTP 错误。
func TestDownloadFileLinksErrors(t *testing.T) {
	srv := mock.New(mock.WithToken("secret-token"))
	defer srv.Close()
	srv.Handle("get_group_file_download_url", func(json.RawMessage) (any, error) {
		return map[string]any{"download_url": ""}, nil
	})
	srv.Handle("get_resource_temp_url", func(json.RawMessage) (any, error) {
		return map[string]any{"url": srv.URL() + "/missing"}, nil
	})

	c, err := New(eventConfig(srv.URL(), func(cfg *Config) { cfg.EventSource = EventSourceNone }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if _, err := c.DownloadFile(context.Background(), 200, "f1", ""); err == nil {
		t.Fatal("空下载链接应报错")
	}
	if _, err := c.DownloadResource(context.Background(), "r1"); err == nil {
		t.Fatal("404 下载应报错")
	}
}
