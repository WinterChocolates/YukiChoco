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

	"github.com/WinterChocolates/YukiChoco/types"
)

// DefaultMaxInlineBytes 是 base64 内联的上限（8 MiB）。
//
// 超过该大小的文件请使用 file:// 或 http(s)://，避免消息体膨胀到协议端拒绝。
const DefaultMaxInlineBytes int64 = 8 << 20

// ErrFileTooLarge 表示资源超过 base64 内联上限。
var ErrFileTooLarge = errors.New("milky/segment: 资源超过 base64 内联上限，请改用 file:// 或 http(s)://")

// URI 把多种来源统一转成 Milky 的 file_uri。
//
// 支持的来源：
//   - string：已是 file:// / http(s):// / base64:// 的直接返回；其余按本地路径处理
//   - []byte：base64:// 内联
//   - io.Reader：读取后 base64:// 内联（超过 DefaultMaxInlineBytes 报 ErrFileTooLarge）
func URI(src any) (string, error) {
	return URIWithLimit(src, DefaultMaxInlineBytes)
}

// URIWithLimit 与 URI 相同，但可自定义 base64 内联上限（<=0 表示用默认值）。
func URIWithLimit(src any, maxInline int64) (string, error) {
	if maxInline <= 0 {
		maxInline = DefaultMaxInlineBytes
	}
	switch v := src.(type) {
	case nil:
		return "", errors.New("milky/segment: 资源来源为空")
	case string:
		return normalizeStringURI(v)
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
		return "", fmt.Errorf("milky/segment: 不支持的资源来源类型 %T", src)
	}
}

// Base64URI 把字节编码为 base64:// URI。
func Base64URI(data []byte) string {
	return "base64://" + base64.StdEncoding.EncodeToString(data)
}

// FileURI 把本地路径转成 file:// URI（相对路径会被解析为绝对路径）。
//
// Windows 盘符路径会被规范化为 file:///C:/dir/a.png（三个斜杠），
// 避免协议端把 file://C:/... 解析成一个名为 C 的主机。
func FileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("milky/segment: 解析路径 %q 失败: %w", path, err)
	}
	slash := filepath.ToSlash(abs)
	// Windows 绝对路径形如 C:/dir（不带前导斜杠），URL 路径必须补上，
	// 否则 url.URL 会输出 file://C:/dir 这种错误形式。
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String(), nil
}

// ReadFileURI 读取本地文件并转为 base64://（超过上限报 ErrFileTooLarge）。
func ReadFileURI(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("milky/segment: 打开 %q 失败: %w", path, err)
	}
	defer f.Close()
	return URIWithLimit(f, DefaultMaxInlineBytes)
}

// normalizeStringURI 处理字符串来源。
//
// 判断顺序很关键：Windows 盘符路径（C:\dir\a.png）会被 url.Parse 误认为
// scheme 为 "c"，所以必须先识别本地路径，再判断 URI。
func normalizeStringURI(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("milky/segment: 资源 URI 为空")
	}

	lower := strings.ToLower(s)
	for _, prefix := range []string{"file://", "http://", "https://", "base64://"} {
		if strings.HasPrefix(lower, prefix) {
			return s, nil
		}
	}

	// 本地路径：Windows 盘符、UNC、POSIX 绝对路径、不含 scheme 的相对路径。
	if isWindowsDrivePath(s) || strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "/") || !strings.Contains(s, ":") {
		return FileURI(s)
	}

	// 其余带 scheme 的形式：明确支持或明确拒绝，不做猜测。
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("milky/segment: 解析 URI %q 失败: %w", s, err)
	}
	if u.Scheme == "" {
		return FileURI(s)
	}
	return "", fmt.Errorf("milky/segment: 不支持的 URI scheme %q", u.Scheme)
}

// isWindowsDrivePath 报告 s 是否是盘符路径（C: 开头，且第二个字符是冒号）。
func isWindowsDrivePath(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// readAtMost 读取最多 limit 字节，超出则报错而不是静默截断。
func readAtMost(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("milky/segment: 读取资源失败: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, ErrFileTooLarge
	}
	return data, nil
}

// ImageFromFile 用本地文件构造图片段。
func ImageFromFile(path string) (types.OutgoingSegment, error) {
	uri, err := FileURI(path)
	if err != nil {
		return nil, err
	}
	return Image(uri), nil
}

// ImageFromBytes 用内存数据构造图片段（base64 内联）。
func ImageFromBytes(data []byte) (types.OutgoingSegment, error) {
	uri, err := URI(data)
	if err != nil {
		return nil, err
	}
	return Image(uri), nil
}

// ImageFromReader 从 reader 读取并构造图片段（base64 内联）。
func ImageFromReader(r io.Reader) (types.OutgoingSegment, error) {
	uri, err := URI(r)
	if err != nil {
		return nil, err
	}
	return Image(uri), nil
}

// ImageFromURL 用远程 URL 构造图片段。
func ImageFromURL(u string) types.OutgoingSegment {
	return Image(u)
}

// RecordFromFile 用本地文件构造语音段。
func RecordFromFile(path string) (types.OutgoingSegment, error) {
	uri, err := FileURI(path)
	if err != nil {
		return nil, err
	}
	return Record(uri), nil
}

// VideoFromFile 用本地文件构造视频段。
func VideoFromFile(path string) (types.OutgoingSegment, error) {
	uri, err := FileURI(path)
	if err != nil {
		return nil, err
	}
	return Video(uri), nil
}

// Segments 是 []types.OutgoingSegment 的便捷别名，便于拼装消息。
type Segments = types.OutgoingSegments
