// admin-bot 演示群管理常用操作：谓词路由、禁言、撤回、公告、上传群文件。
//
// 运行：
//
//	MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/admin-bot
//
// 命令（需 @机器人，且发送者为管理员或群主）：
//
//	/mute <QQ号> <秒数>
//	/recall <message_seq>
//	/announce <内容>
//	/upload <本地文件路径>
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
		Dispatch:    YukiChoco.DispatchConfig{Mode: YukiChoco.DispatchPerPeerSequential},
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

	// Init 必须在注册路由前完成：AtMe 在注册时求值 self_id。
	if err := c.Init(ctx); err != nil {
		logger.Error("初始化失败", "err", err)
		os.Exit(1)
	}

	admin := c.GroupRouter().AtMe(c.SelfID()).Admin()

	admin.Prefix("/mute").Handle(func(ctx context.Context, msg *types.GroupMessage) error {
		fields := strings.Fields(msg.PlainText())
		if len(fields) < 3 {
			return reply(c, ctx, msg, "用法：/mute <QQ号> <秒数>")
		}
		uid, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return reply(c, ctx, msg, "QQ 号不合法")
		}
		seconds, err := strconv.Atoi(fields[2])
		if err != nil || seconds < 0 {
			return reply(c, ctx, msg, "秒数不合法")
		}
		_, err = c.Group().SetMemberMute(ctx, &types.SetGroupMemberMuteRequest{
			GroupID:  msg.Group.GroupID,
			UserID:   types.UserID(uid),
			Duration: int32(seconds),
		})
		if err != nil {
			return reply(c, ctx, msg, "禁言失败："+err.Error())
		}
		return reply(c, ctx, msg, fmt.Sprintf("已禁言 %d 秒", seconds))
	})

	admin.Prefix("/recall").Handle(func(ctx context.Context, msg *types.GroupMessage) error {
		fields := strings.Fields(msg.PlainText())
		if len(fields) < 2 {
			return reply(c, ctx, msg, "用法：/recall <message_seq>")
		}
		seq, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return reply(c, ctx, msg, "message_seq 不合法")
		}
		if _, err := c.Message().RecallGroupMessage(ctx, &types.RecallGroupMessageRequest{
			GroupID:    msg.Group.GroupID,
			MessageSeq: types.MessageSeq(seq),
		}); err != nil {
			return reply(c, ctx, msg, "撤回失败："+err.Error())
		}
		return nil
	})

	admin.Prefix("/announce").Handle(func(ctx context.Context, msg *types.GroupMessage) error {
		content := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(msg.PlainText()), "/announce"))
		if content == "" {
			return reply(c, ctx, msg, "用法：/announce <内容>")
		}
		_, err := c.Group().SendAnnouncement(ctx, &types.SendGroupAnnouncementRequest{
			GroupID: msg.Group.GroupID,
			Content: content,
		})
		if err != nil {
			return reply(c, ctx, msg, "发布公告失败："+err.Error())
		}
		return reply(c, ctx, msg, "公告已发布")
	})

	admin.Prefix("/upload").Handle(func(ctx context.Context, msg *types.GroupMessage) error {
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(msg.PlainText()), "/upload"))
		if path == "" {
			return reply(c, ctx, msg, "用法：/upload <本地文件路径>")
		}
		res, err := c.UploadGroupFileWith(ctx, msg.Group.GroupID, "", filepath.Base(path), path)
		if err != nil {
			return reply(c, ctx, msg, "上传失败："+err.Error())
		}
		return reply(c, ctx, msg, "已上传，file_id="+res.FileID)
	})

	if err := c.Start(ctx); err != nil {
		logger.Error("启动失败", "err", err)
		os.Exit(1)
	}
	logger.Info("admin-bot 已启动", "self_id", c.SelfID().Int64())
	<-ctx.Done()
}

func reply(c *YukiChoco.Client, ctx context.Context, msg *types.GroupMessage, text string) error {
	_, err := c.Group().SendMessage(ctx, msg.Group.GroupID,
		segment.Reply(msg.MessageSeq),
		segment.Text(text),
	)
	return err
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
