package YukiChoco

// 真机 e2e 测试：默认跳过，设置环境变量后对真实 Milky 协议端执行。
//
//	MILKY_E2E_BASE_URL=http://127.0.0.1:6700 \
//	MILKY_E2E_TOKEN=xxx \
//	MILKY_E2E_GROUP_ID=123456 \  # 可选：提供后会实际发消息并等待回显
//	go test -run TestE2E -v ./...
//
// 说明：
//   - 未设置 MILKY_E2E_BASE_URL 时整个测试被跳过，CI 无需真实协议端；
//   - 提供 MILKY_E2E_GROUP_ID 时会真的向该群发一条文本消息（请使用测试群）；
//   - 全部步骤依赖协议端已登录，否则 get_login_info 会返回 retcode -403。

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestE2E(t *testing.T) {
	baseURL := os.Getenv("MILKY_E2E_BASE_URL")
	if baseURL == "" {
		t.Skip("设置 MILKY_E2E_BASE_URL 后运行真机 e2e（例如 http://127.0.0.1:6700）")
	}
	if testing.Short() {
		t.Skip("short 模式跳过真机 e2e")
	}

	token := os.Getenv("MILKY_E2E_TOKEN")
	groupID := types.GroupID(0)
	if v := os.Getenv("MILKY_E2E_GROUP_ID"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("MILKY_E2E_GROUP_ID 不合法: %v", err)
		}
		groupID = types.GroupID(id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c, err := New(Config{
		BaseURL:     baseURL,
		AccessToken: token,
		EventSource: EventSourceWS,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Reconnect: ReconnectConfig{
			InitialDelay: time.Second,
			MaxDelay:     10 * time.Second,
		},
		IdleTimeout: 120 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Shutdown(context.Background()) }()

	// 1. 协议端信息与登录状态
	if err := c.Init(ctx); err != nil {
		if errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("协议端未登录: %v", err)
		}
		t.Fatalf("Init: %v", err)
	}
	impl, ok := c.ProtocolInfo()
	if !ok {
		t.Fatal("ProtocolInfo 未缓存")
	}
	t.Logf("协议端: %s %s / QQ 协议 %s (%s) / Milky %s / self_id=%d",
		impl.ImplName, impl.ImplVersion, impl.QQProtocolVersion, impl.QQProtocolType,
		impl.MilkyVersion, c.SelfID().Int64())

	// 2. 只读接口
	if _, err := c.System().GetFriendList(ctx, &types.GetFriendListRequest{}); err != nil {
		t.Errorf("GetFriendList: %v", err)
	}
	groups, err := c.System().GetGroupList(ctx, &types.GetGroupListRequest{})
	if err != nil {
		t.Errorf("GetGroupList: %v", err)
	} else {
		t.Logf("群数量: %d", len(groups.Groups))
		if groupID == 0 && len(groups.Groups) > 0 {
			groupID = groups.Groups[0].GroupID
			t.Logf("未指定 MILKY_E2E_GROUP_ID，使用第一个群 %d 做发送测试", groupID.Int64())
		}
	}

	// 3. 事件通道 + 发送回显
	received := make(chan *types.GroupMessage, 4)
	On(c, func(_ context.Context, ev *types.MessageReceiveEvent) error {
		msg, ok := ev.GroupMessage()
		if !ok || msg.Group.GroupID != groupID {
			return nil
		}
		select {
		case received <- msg:
		default:
		}
		return nil
	})

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	readyCtx, readyCancel := context.WithTimeout(ctx, 20*time.Second)
	defer readyCancel()
	if err := c.WaitReady(readyCtx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	t.Logf("事件通道已就绪: %s（%s）", c.Config().EventURL, c.Config().EventSource)

	if groupID == 0 {
		t.Log("没有可用群，跳过发送回显测试")
		return
	}

	marker := "e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	res, err := c.Group().SendMessage(ctx, groupID,
		segment.Text("YukiChoco e2e "),
		segment.Text(marker),
	)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	t.Logf("已发送消息 seq=%d marker=%s", res.MessageSeq.Int64(), marker)

	// 协议端应把刚发的消息回推给事件通道。
	select {
	case msg := <-received:
		if msg.SenderID != c.SelfID() {
			t.Logf("收到群消息来自 %d（非自身，可能是并发消息）", msg.SenderID.Int64())
		}
		t.Logf("事件通道收到群消息 seq=%d text=%q", msg.MessageSeq.Int64(), msg.PlainText())
	case <-time.After(30 * time.Second):
		t.Fatalf("30s 内未通过事件通道收回自己发送的消息（seq=%d）", res.MessageSeq.Int64())
	}

	// 4. 撤回（清理测试消息）
	if _, err := c.Message().RecallGroupMessage(ctx, &types.RecallGroupMessageRequest{
		GroupID:    groupID,
		MessageSeq: res.MessageSeq,
	}); err != nil {
		t.Errorf("RecallGroupMessage: %v", err)
	}

	// 5. 运行统计
	stats := c.Events().Stats()
	t.Logf("分发统计: handled=%d failed=%d dropped=%d panicked=%d mode=%s",
		stats.Handled, stats.Failed, stats.Dropped, stats.Panicked, stats.Mode)
	if stats.Failed > 0 {
		t.Errorf("存在 %d 条事件处理失败", stats.Failed)
	}
}
