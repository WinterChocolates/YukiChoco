// http-bot 演示 HTTP API + WebHook 事件。
//
// 协议端需要把事件 POST 到 http://<本机>:8081/onebot/v11/events。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	apiBase := os.Getenv("ONEBOT_HTTP_API")
	if apiBase == "" {
		apiBase = "http://127.0.0.1:5700"
	}

	c, err := YukiChoco.New(YukiChoco.Config{
		Transport:   YukiChoco.TransportHTTP,
		AccessToken: os.Getenv("ONEBOT_ACCESS_TOKEN"),
		HTTP:        YukiChoco.HTTPConfig{APIBaseURL: apiBase},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	if err := c.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	event.On(c.Events(), func(ctx context.Context, msg *types.PrivateMessageEvent) error {
		_, err := c.ReplyPrivateMessage(ctx, msg, "echo: "+msg.PlainText())
		return err
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("/onebot/v11/events", c.WebhookHandler())
	srv := &http.Server{Addr: "127.0.0.1:8081", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("webhook 监听 :8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	_ = c.Shutdown(shutdown)
}
