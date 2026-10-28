package segment

import "testing"

// TestIsWindowsDrivePath 直接锁定盘符判定逻辑，使该分支在 Linux CI 上也能被覆盖。
func TestIsWindowsDrivePath(t *testing.T) {
	cases := map[string]bool{
		`C:\temp\a.png`:  true,
		"C:/temp/a.png":  true,
		"c:/x":           true,
		"D:":             true,
		`1:/x`:           false,
		":/x":            false,
		"":               false,
		"http://example": false,
		`\\server\share`: false,
		"/tmp/a.png":     false,
		"relative.txt":   false,
	}
	for in, want := range cases {
		if got := isWindowsDrivePath(in); got != want {
			t.Errorf("isWindowsDrivePath(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestNormalizeStringURI 覆盖 URI / 本地路径的分流规则。
func TestNormalizeStringURI(t *testing.T) {
	// 已知 URI 原样返回（大小写不敏感）。
	for _, in := range []string{
		"file:///tmp/a.png",
		"FILE:///tmp/a.png",
		"https://example.com/a.png",
		"BASE64://AAAA",
	} {
		got, err := normalizeStringURI(in)
		if err != nil {
			t.Fatalf("normalizeStringURI(%q): %v", in, err)
		}
		if got != in {
			t.Errorf("normalizeStringURI(%q) = %q，应原样返回", in, got)
		}
	}

	// 不支持的 scheme 与空串报错。
	if _, err := normalizeStringURI("ftp://x/a"); err == nil {
		t.Error("ftp scheme 应报错")
	}
	if _, err := normalizeStringURI("   "); err == nil {
		t.Error("空白字符串应报错")
	}
}
