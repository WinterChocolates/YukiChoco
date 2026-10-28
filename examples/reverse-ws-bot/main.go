// reverse-ws-bot 演示反向 WebSocket：SDK 监听，协议端主动连接。
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
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	addr := os.Getenv("ONEBOT_REVERSE_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	c, err := YukiChoco.New(YukiChoco.Config{
		Transport:   YukiChoco.TransportReverseWS,
		AccessToken: os.Getenv("ONEBOT_ACCESS_TOKEN"),
		ReverseWS: YukiChoco.ReverseWSConfig{
			Listen: addr,
			Path:   "/onebot/v11/ws",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	event.On(c.Events(), func(ctx context.Context, msg *types.GroupMessageEvent) error {
		if msg.PlainText() != "ping" {
			return nil
		}
		_, err := c.SendGroupText(ctx, msg.GroupID, "pong")
		return err
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("反向 WS 监听 %s，路径 /onebot/v11/ws", addr)
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Shutdown(shutdown)
}
