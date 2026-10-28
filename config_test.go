package YukiChoco_test

import (
	"strings"
	"testing"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestConfigDefaults(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	cfg := c.Config()
	if cfg.Transport != YukiChoco.TransportHTTP {
		t.Errorf("transport=%s", cfg.Transport)
	}
	if cfg.HTTP.APIBaseURL != "http://127.0.0.1:5700" {
		t.Errorf("base=%s", cfg.HTTP.APIBaseURL)
	}
	if cfg.APITimeout <= 0 || cfg.MaxResponseBytes <= 0 {
		t.Errorf("defaults=%+v", cfg)
	}
	if cfg.UserAgent == "" || cfg.UserAgent != "YukiChoco/"+YukiChoco.Version+" (OneBot v11)" {
		t.Errorf("ua=%s", cfg.UserAgent)
	}
}

func TestConfigForwardWSDerivesEventURL(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{
		Transport: YukiChoco.TransportForwardWS,
		ForwardWS: YukiChoco.ForwardWSConfig{URL: "ws://127.0.0.1:6700/api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.Config().ForwardWS.EventURL; got != "ws://127.0.0.1:6700/event" {
		t.Fatalf("event url=%q", got)
	}
}

func TestConfigReverseDefaults(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{Transport: YukiChoco.TransportReverseWS})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	cfg := c.Config()
	if cfg.ReverseWS.Listen != "127.0.0.1:8080" || cfg.ReverseWS.Path != "/onebot/v11/ws" {
		t.Fatalf("reverse=%+v", cfg.ReverseWS)
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []YukiChoco.Config{
		{Transport: "bogus"},
		{Transport: YukiChoco.TransportForwardWS},
		{Transport: YukiChoco.TransportForwardWS, ForwardWS: YukiChoco.ForwardWSConfig{URL: "http://x"}},
		{Transport: YukiChoco.TransportHTTP, HTTP: YukiChoco.HTTPConfig{APIBaseURL: "ftp://x"}},
	}
	for i, cfg := range cases {
		if _, err := YukiChoco.New(cfg); err == nil {
			t.Errorf("case %d 应校验失败: %+v", i, cfg)
		}
	}
}

func TestConfigRedactsToken(t *testing.T) {
	cfg := YukiChoco.Config{AccessToken: "supersecret", HTTP: YukiChoco.HTTPConfig{APIBaseURL: "http://127.0.0.1:5700"}}
	s := cfg.String()
	if strings.Contains(s, "supersecret") {
		t.Fatalf("配置摘要泄漏 token: %s", s)
	}
	if !strings.Contains(s, "***") {
		t.Fatalf("摘要未标注脱敏: %s", s)
	}
}

func TestSelfIDZeroBeforeInit(t *testing.T) {
	c, err := YukiChoco.New(YukiChoco.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !c.SelfID().IsZero() {
		t.Fatalf("self id=%d", c.SelfID())
	}
	if _, ok := c.LoginInfo(); ok {
		t.Error("LoginInfo 不应存在")
	}
}

func TestVersion(t *testing.T) {
	if YukiChoco.Version != "0.2.0" {
		t.Fatalf("version=%s", YukiChoco.Version)
	}
	if types.EventTypeGroupMessage != "message.group" {
		t.Fatal("event type")
	}
}
