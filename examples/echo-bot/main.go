// echo-bot 演示最小可用的群聊回声机器人：被 @ 时引用回复原文本。
//
// 运行：
//
//	MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/echo-bot
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
		EventSource: YukiChoco.EventSourceWS,
		Logger:      logger,
		// 同一群内的消息串行处理，避免并发回复乱序。
		Dispatch: YukiChoco.DispatchConfig{Mode: YukiChoco.DispatchPerPeerSequential},
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
		msg, ok := ev.GroupMessage()
		if !ok || !msg.Segments.Mentioned(c.SelfID()) {
			return nil
		}
		_, err := c.Group().SendMessage(ctx, msg.Group.GroupID,
			segment.Reply(msg.MessageSeq),
			segment.Text("收到："),
			segment.Text(msg.PlainText()),
		)
		if err != nil {
			logger.Error("发送失败", "group_id", msg.Group.GroupID.Int64(), "err", err)
		}
		return err
	})

	if err := c.Init(ctx); err != nil {
		logger.Error("初始化失败", "err", err)
		os.Exit(1)
	}
	if err := c.Start(ctx); err != nil {
		logger.Error("启动失败", "err", err)
		os.Exit(1)
	}
	logger.Info("echo-bot 已启动", "self_id", c.SelfID().Int64(), "event_url", c.Config().EventURL)

	<-ctx.Done()
	logger.Info("正在退出")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
