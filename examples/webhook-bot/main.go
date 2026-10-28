// webhook-bot 演示 WebHook 模式：协议端把事件 POST 到本服务的 /milky/event。
//
// 运行：
//
//	MILKY_BASE_URL=http://10.0.0.2:6700 MILKY_TOKEN=xxx LISTEN=:8080 \
//	  go run ./examples/webhook-bot
//
// 然后在协议端配置 WebHook 地址为 http://<本机>:8080/milky/event。
//
// 注意：WebHook 没有断线重连语义，协议端推送失败时事件即丢失；
// 公网部署时应打开 WebHookVerifyToken 或在反向代理层做来源校验。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := YukiChoco.New(YukiChoco.Config{
		BaseURL:     env("MILKY_BASE_URL", "http://127.0.0.1:6700"),
		AccessToken: os.Getenv("MILKY_TOKEN"),
		EventSource: YukiChoco.EventSourceWebhook,
		// 协议端不保证发送 Authorization 头；确认它支持后再打开。
		WebHookVerifyToken: os.Getenv("MILKY_WEBHOOK_AUTH") == "1",
		Logger:             logger,
	})
	if err != nil {
		logger.Error("创建客户端失败", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := c.Shutdown(context.Background()); err != nil {
			logger.Error("关闭失败", "err", err)
		}
	}()

	YukiChoco.On(c, func(ctx context.Context, ev *types.MessageReceiveEvent) error {
		msg, ok := ev.FriendMessage()
		if !ok {
			return nil
		}
		_, err := c.Message().SendPrivateMessage(ctx, msg.Friend.UserID,
			segment.Reply(msg.MessageSeq),
			segment.Text("收到私聊："+msg.PlainText()),
		)
		return err
	})

	mux := http.NewServeMux()
	mux.Handle("/milky/event", c.EventHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              env("LISTEN", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP 服务退出", "err", err)
			stop()
		}
	}()

	if err := c.Start(ctx); err != nil {
		logger.Error("启动失败", "err", err)
		os.Exit(1)
	}
	logger.Info("webhook-bot 已启动", "listen", srv.Addr)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP 优雅关闭失败", "err", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
