package segment

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultMaxInlineBytes 是 base64 内联的默认上限（8 MiB）。
//
// 超过该大小的文件请使用 http(s):// 或协议端可访问的本地路径。
const DefaultMaxInlineBytes int64 = 8 << 20

// ErrFileTooLarge 表示资源超过 base64 内联上限。
var ErrFileTooLarge = errors.New("segment: 资源超过 base64 内联上限")

// URI 把多种资源来源统一成 OneBot 的 file 字段值。
//
// 支持的来源：
//   - string：http(s):// / base64:// / file:// 原样返回；本地路径转绝对路径
//   - []byte：base64:// 内联
//   - io.Reader：读取后 base64:// 内联（超过 DefaultMaxInlineBytes 报错）
func URI(src any) (string, error) {
	return URIWithLimit(src, DefaultMaxInlineBytes)
}

// URIWithLimit 与 URI 相同，但可自定义内联上限（<=0 使用默认值）。
func URIWithLimit(src any, maxInline int64) (string, error) {
	if maxInline <= 0 {
		maxInline = DefaultMaxInlineBytes
	}
	switch v := src.(type) {
	case nil:
		return "", errors.New("segment: 资源来源为空")
	case string:
		return normalizeString(v)
	case []byte:
		if int64(len(v)) > maxInline {
			return "", ErrFileTooLarge
		}
		return Base64URI(v), nil
	case io.Reader:
		data, err := readAtMost(v, maxInline)
		if err != nil {
			return "", err
		}
		return Base64URI(data), nil
	default:
		return "", fmt.Errorf("segment: 不支持的资源来源类型 %T", src)
	}
}

// Base64URI 把字节编码为 base64:// 形式。
func Base64URI(data []byte) string {
	return "base64://" + base64.StdEncoding.EncodeToString(data)
}

// FileURI 把本地路径转成 file:// URI。
func FileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("segment: 解析路径 %q 失败: %w", path, err)
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String(), nil
}

// ReadFileBase64 读取本地文件并编码为 base64://。
func ReadFileBase64(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("segment: 打开 %q 失败: %w", path, err)
	}
	defer f.Close()
	return URIWithLimit(f, DefaultMaxInlineBytes)
}

func normalizeString(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("segment: 资源路径为空")
	}
	lower := strings.ToLower(s)
	for _, prefix := range []string{"base64://", "http://", "https://", "file://"} {
		if strings.HasPrefix(lower, prefix) {
			return s, nil
		}
	}
	// 其余按本地路径处理（含 Windows 盘符与相对路径）。
	abs, err := filepath.Abs(s)
	if err != nil {
		return "", fmt.Errorf("segment: 解析路径 %q 失败: %w", s, err)
	}
	return abs, nil
}

func readAtMost(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("segment: 读取资源失败: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, ErrFileTooLarge
	}
	return data, nil
}
