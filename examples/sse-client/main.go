// sse-client 演示 SSE 事件通道：适合 WebSocket 被中间设备阻断的环境。
//
// 运行：
//
//	MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/sse-client
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := YukiChoco.New(YukiChoco.Config{
		BaseURL:     env("MILKY_BASE_URL", "http://127.0.0.1:6700"),
		AccessToken: os.Getenv("MILKY_TOKEN"),
		EventSource: YukiChoco.EventSourceSSE,
		Logger:      logger,
		// SSE 没有 ping/pong，用读空闲超时判定僵死连接并重连。
		IdleTimeout: 90 * time.Second,
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

	// 中间件：给每条事件加处理耗时日志；单条事件处理超过 5s 自动取消 ctx。
	c.Use(event.Logging(logger, slog.LevelDebug), event.Timeout(5*time.Second))

	YukiChoco.On(c, func(ctx context.Context, ev *types.GroupMemberIncreaseEvent) error {
		logger.Info("新成员入群",
			"group_id", ev.Data.GroupID.Int64(),
			"user_id", ev.Data.UserID.Int64())
		return nil
	})

	c.OnStateChange(func(s YukiChoco.State) {
		logger.Info("连接状态变化", "state", s.String())
	})

	if err := c.Start(ctx); err != nil {
		logger.Error("启动失败", "err", err)
		os.Exit(1)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := c.WaitReady(readyCtx); err != nil {
		logger.Error("等待就绪失败", "err", err)
		os.Exit(1)
	}
	logger.Info("SSE 客户端已就绪", "event_url", c.Config().EventURL)

	<-ctx.Done()
	stats := c.Events().Stats()
	logger.Info("退出版本统计",
		"handled", stats.Handled, "failed", stats.Failed,
		"dropped", stats.Dropped, "panicked", stats.Panicked)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
