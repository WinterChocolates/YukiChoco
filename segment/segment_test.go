package segment_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(raw)
}

func TestConstructors(t *testing.T) {
	cases := []struct {
		name string
		seg  types.OutgoingSegment
		want string
	}{
		{"text", segment.Text("hi"), `{"type":"text","data":{"text":"hi"}}`},
		{"mention", segment.Mention(42), `{"type":"mention","data":{"user_id":42}}`},
		{"mention_all", segment.MentionAll(), `{"type":"mention_all","data":{}}`},
		{"face", segment.Face("1"), `{"type":"face","data":{"face_id":"1"}}`},
		{"large_face", segment.LargeFace("1"), `{"type":"face","data":{"face_id":"1","is_large":true}}`},
		{"reply", segment.Reply(9), `{"type":"reply","data":{"message_seq":9}}`},
		{"image", segment.Image("file:///a.png"), `{"type":"image","data":{"uri":"file:///a.png"}}`},
		{"sticker", segment.Sticker("https://x/y.png"), `{"type":"image","data":{"uri":"https://x/y.png","sub_type":"sticker"}}`},
		{"record", segment.Record("base64://AA=="), `{"type":"record","data":{"uri":"base64://AA=="}}`},
		{"video", segment.Video("file:///v.mp4"), `{"type":"video","data":{"uri":"file:///v.mp4"}}`},
		{"light_app", segment.LightApp(`{"a":1}`), `{"type":"light_app","data":{"json_payload":"{\"a\":1}"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustJSON(t, tc.seg); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestForward(t *testing.T) {
	seg := segment.Forward(
		types.OutgoingForwardedMessage{
			UserID:     1,
			SenderName: "甲",
			Segments:   types.OutgoingSegments{segment.Text("a")},
		},
		types.OutgoingForwardedMessage{
			UserID:     2,
			SenderName: "乙",
			Segments:   types.OutgoingSegments{segment.Text("b")},
		},
	)
	raw := mustJSON(t, seg)
	if !strings.HasPrefix(raw, `{"type":"forward","data":{"messages":[`) {
		t.Fatalf("forward 段结构错误: %s", raw)
	}
	if strings.Count(raw, `"sender_name"`) != 2 {
		t.Fatalf("嵌套消息数量错误: %s", raw)
	}

	withOpts := mustJSON(t, segment.ForwardWithOptions(nil, "标题", "预览", "摘要", "提示"))
	for _, want := range []string{`"title":"标题"`, `"preview":["预览"]`, `"summary":"摘要"`, `"prompt":"提示"`} {
		if !strings.Contains(withOpts, want) {
			t.Errorf("缺少 %s: %s", want, withOpts)
		}
	}
}

func TestURIFromString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	uri, err := segment.URI(path)
	if err != nil {
		t.Fatalf("URI(path): %v", err)
	}
	if !strings.HasPrefix(uri, "file:///") {
		t.Fatalf("本地路径应转成 file:// 绝对路径，得到 %s", uri)
	}

	for _, passthrough := range []string{
		"file:///tmp/a.png",
		"https://example.com/a.png",
		"http://example.com/a.png",
		"base64://AAAA",
	} {
		got, err := segment.URI(passthrough)
		if err != nil {
			t.Fatalf("URI(%s): %v", passthrough, err)
		}
		if got != passthrough {
			t.Fatalf("URI(%s) = %s，应原样返回", passthrough, got)
		}
	}

	if _, err := segment.URI("ftp://example.com/a"); err == nil {
		t.Fatal("不支持的 scheme 应报错")
	}
	if _, err := segment.URI(""); err == nil {
		t.Fatal("空 URI 应报错")
	}
}

func TestURIFromBytesAndReader(t *testing.T) {
	uri, err := segment.URI([]byte("hello"))
	if err != nil {
		t.Fatalf("URI([]byte): %v", err)
	}
	if uri != "base64://aGVsbG8=" {
		t.Fatalf("base64 URI = %s", uri)
	}

	uri, err = segment.URI(bytes.NewReader([]byte("abc")))
	if err != nil {
		t.Fatalf("URI(reader): %v", err)
	}
	if uri != "base64://YWJj" {
		t.Fatalf("reader URI = %s", uri)
	}

	// 超过内联上限应报 ErrFileTooLarge，而不是静默截断。
	big := bytes.Repeat([]byte("x"), 32)
	if _, err := segment.URIWithLimit(big, 16); !errors.Is(err, segment.ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if _, err := segment.URI(big); err != nil {
		t.Fatalf("32 字节不应超默认上限: %v", err)
	}

	if _, err := segment.URI(123); err == nil {
		t.Fatal("不支持的来源类型应报错")
	}
}

func TestMediaHelpers(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}

	seg, err := segment.ImageFromFile(img)
	if err != nil {
		t.Fatalf("ImageFromFile: %v", err)
	}
	if got := seg.SegmentType(); got != types.SegmentImage {
		t.Fatalf("SegmentType = %s", got)
	}
	if !strings.Contains(mustJSON(t, seg), "file:///") {
		t.Fatalf("图片段未使用 file://: %s", mustJSON(t, seg))
	}

	seg, err = segment.ImageFromBytes([]byte("img"))
	if err != nil {
		t.Fatalf("ImageFromBytes: %v", err)
	}
	if !strings.Contains(mustJSON(t, seg), "base64://") {
		t.Fatalf("图片段未使用 base64://: %s", mustJSON(t, seg))
	}

	if _, err := segment.ImageFromFile(filepath.Join(dir, "missing.png")); err != nil {
		t.Fatalf("ImageFromFile 只做路径转换，不打开文件: %v", err)
	}
	if _, err := segment.ReadFileURI(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("ReadFileURI 对不存在的文件应报错")
	}

	if _, err := segment.RecordFromFile(img); err != nil {
		t.Fatal(err)
	}
	if _, err := segment.VideoFromFile(img); err != nil {
		t.Fatal(err)
	}
}

func TestImageFromReaderAndURL(t *testing.T) {
	seg, err := segment.ImageFromReader(bytes.NewReader([]byte("img")))
	if err != nil {
		t.Fatalf("ImageFromReader: %v", err)
	}
	if !strings.Contains(mustJSON(t, seg), "base64://") {
		t.Fatalf("ImageFromReader 应使用 base64: %s", mustJSON(t, seg))
	}

	seg = segment.ImageFromURL("https://example.com/a.png")
	if !strings.Contains(mustJSON(t, seg), `"uri":"https://example.com/a.png"`) {
		t.Fatalf("ImageFromURL = %s", mustJSON(t, seg))
	}
}

// TestURILocalPaths 覆盖各平台的本地路径识别（Windows 盘符不应被当成 scheme）。
func TestURILocalPaths(t *testing.T) {
	// 相对路径在任何平台都应转成 file:// 绝对路径。
	uri, err := segment.URI("relative/a.png")
	if err != nil {
		t.Fatalf("URI(相对路径): %v", err)
	}
	if !strings.HasPrefix(uri, "file:///") {
		t.Fatalf("相对路径应转成 file:/// 形式，得到 %s", uri)
	}

	// POSIX 绝对路径。
	uri, err = segment.URI("/tmp/a.png")
	if err != nil {
		t.Fatalf("URI(/tmp/a.png): %v", err)
	}
	if !strings.HasPrefix(uri, "file:///") {
		t.Fatalf("绝对路径应转成 file:/// 形式，得到 %s", uri)
	}

	if runtime.GOOS != "windows" {
		return
	}
	// Windows 盘符路径：反斜杠与正斜杠两种写法都不能被当成 URI scheme。
	for _, in := range []string{
		`C:\temp\a.png`,
		"C:/temp/a.png",
		"c:/temp/a.png",
	} {
		uri, err := segment.URI(in)
		if err != nil {
			t.Fatalf("URI(%s): %v", in, err)
		}
		if !strings.HasPrefix(strings.ToLower(uri), "file:///c:/") {
			t.Fatalf("URI(%s) = %s，期望 file:///C:/... 形式", in, uri)
		}
	}
}
