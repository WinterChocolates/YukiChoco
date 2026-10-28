package segment_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestConstructors(t *testing.T) {
	if seg := segment.Text("hi"); seg.Type != types.SegmentText || seg.String("text") != "hi" {
		t.Errorf("text=%+v", seg)
	}
	if seg := segment.At(42); seg.String("qq") != "42" {
		t.Errorf("at=%+v", seg)
	}
	if seg := segment.AtAll(); seg.String("qq") != "all" {
		t.Errorf("atall=%+v", seg)
	}
	if seg := segment.Reply(99); seg.Int64("id") != 99 {
		t.Errorf("reply=%+v", seg)
	}
	if seg := segment.Image("a.png"); seg.String("file") != "a.png" {
		t.Errorf("image=%+v", seg)
	}
	if seg := segment.Dice(); seg.Type != types.SegmentDice || len(seg.Data) != 0 {
		t.Errorf("dice=%+v", seg)
	}
	node := segment.Node(1, "n", segment.Text("a"))
	if node.Type != types.SegmentNode {
		t.Fatalf("node=%+v", node)
	}
	content, ok := node.Get("content")
	if !ok {
		t.Fatal("缺少 content")
	}
	segs, ok := content.([]types.Segment)
	if !ok || len(segs) != 1 {
		t.Fatalf("content=%#v", content)
	}
}

func TestURI(t *testing.T) {
	if got, err := segment.URI("https://example.com/a.png"); err != nil || got != "https://example.com/a.png" {
		t.Errorf("url: %q %v", got, err)
	}
	if got, err := segment.URI("base64://abc"); err != nil || got != "base64://abc" {
		t.Errorf("base64: %q %v", got, err)
	}
	got, err := segment.URI("relative/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "relative") || !filepath.IsAbs(got) {
		t.Errorf("path=%q", got)
	}
	b64, err := segment.URI([]byte("abc"))
	if err != nil || b64 != "base64://YWJj" {
		t.Errorf("bytes=%q %v", b64, err)
	}
}

func TestURILimit(t *testing.T) {
	if _, err := segment.URIWithLimit(strings.NewReader("0123456789"), 5); err != segment.ErrFileTooLarge {
		t.Fatalf("err=%v", err)
	}
}

func TestImageFrom(t *testing.T) {
	seg, err := segment.ImageFrom([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if seg.Type != types.SegmentImage || !strings.HasPrefix(seg.String("file"), "base64://") {
		t.Errorf("seg=%+v", seg)
	}
	if _, err := segment.ImageFrom(nil); err == nil {
		t.Error("nil 应报错")
	}
}

func TestAllConstructors(t *testing.T) {
	segs := []types.Segment{
		segment.Text("t"), segment.At(1), segment.AtAll(), segment.Face("1"),
		segment.MFace("e", "s"), segment.Image("f"), segment.ImageWithSubType("f", "1"),
		segment.Record("f"), segment.Video("f"), segment.Reply(1), segment.Forward("id"),
		segment.Node(1, "n"), segment.NodeWithTime(1, "n", 100), segment.Poke(1),
		segment.Dice(), segment.RPS(), segment.Shake(), segment.JSON("{}"),
		segment.XML("<x/>"), segment.Card("c"), segment.Music("qq", "1"),
		segment.CustomMusic("u", "a", "t", "i"), segment.Location("1", "2", "t", "c"),
		segment.Contact("friend", 1), segment.Share("u", "t", "c", "i"),
		segment.Anonymous(true), segment.File("n", "f"), segment.TTS("t"),
		segment.Markdown("m"),
	}
	for i, seg := range segs {
		if seg.Type == "" {
			t.Errorf("segment %d 缺少 type", i)
		}
	}
	if segment.MustImage([]byte("x")).Type != types.SegmentImage {
		t.Error("MustImage")
	}
	if _, err := segment.RecordFrom([]byte("x")); err != nil {
		t.Error(err)
	}
	if _, err := segment.VideoFrom("http://a/b.mp4"); err != nil {
		t.Error(err)
	}
	if _, err := segment.FileURI("."); err != nil {
		t.Error(err)
	}
	if _, err := segment.ReadFileBase64("does-not-exist"); err == nil {
		t.Error("不存在文件应报错")
	}
}
