// echo-bot 演示正向 WebSocket（通用单连接）与 @ 回复。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	wsURL := os.Getenv("ONEBOT_WS_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:3001/"
	}

	c, err := YukiChoco.New(YukiChoco.Config{
		Transport:   YukiChoco.TransportForwardWS,
		AccessToken: os.Getenv("ONEBOT_ACCESS_TOKEN"),
		ForwardWS:   YukiChoco.ForwardWSConfig{URL: wsURL},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	event.On(c.Events(), func(ctx context.Context, msg *types.GroupMessageEvent) error {
		if !msg.AtMe(c.SelfID()) {
			return nil
		}
		text := msg.PlainText()
		if text == "" {
			text = "（空消息）"
		}
		_, err := c.SendGroupMessage(ctx, msg.GroupID, types.Message{
			segment.Reply(msg.MessageID),
			segment.Text("收到：" + text),
		})
		return err
	})

	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
	if err := c.WaitReady(ctx); err != nil {
		log.Fatal(err)
	}
	if err := c.Init(ctx); err != nil {
		// Init 只用于填充 SelfID / 版本信息；失败不阻塞事件。
		log.Printf("init: %v", err)
	}
	log.Printf("echo-bot 已就绪，self_id=%s", c.SelfID())

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Shutdown(shutdown)
}
