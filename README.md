# YukiChoco

[![CI](https://github.com/WinterChocolates/YukiChoco/actions/workflows/ci.yml/badge.svg)](https://github.com/WinterChocolates/YukiChoco/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Milky](https://img.shields.io/badge/protocol-Milky%201.3-ff69b4)](https://milky.ntqqrev.org/)

**Milky 协议的 Go SDK**：面向 QQ 机器人应用端，对齐 [Milky 1.3](https://milky.ntqqrev.org/)，
协议类型由官方 [Milky IR](https://www.npmjs.com/package/@saltify/milky-protocol) 代码生成，而非手抄。

```go
import "github.com/WinterChocolates/YukiChoco"
```

- **协议端**（服务端）：Lagrange.Milky、Yogurt、LuckyLilliaBot 等
- **应用端**（本 SDK）：HTTP 调 `/api/{endpoint}`，通过 WebSocket / SSE / WebHook 收事件
- **不负责**：QQ 协议本身、bot 框架（插件/权限/命令系统）、OneBot 兼容层

| 项 | 数量 / 状态 |
| --- | --- |
| 协议版本 | Milky **1.3**（IR `1.3.0`，sha256 锁定） |
| 端点 | **65**（System 17 / Message 8 / Friend 6 / Group 22 / File 12） |
| 事件 | **21** |
| 消息段 | **24**（接收 14 / 发送 10） |
| 测试 | **132** 个用例/子测试，`-race` 全绿 |
| 覆盖率 | **86.1%**（排除生成物与示例） |
| 生产依赖 | 仅 `github.com/gorilla/websocket` |

---

## 特性

| 能力 | 说明 |
| --- | --- |
| 类型由 IR 生成 | 字段级 `since` / 默认值 / 可选性全部保留；协议升级只需 `make generate` |
| 宽容解码 | 未知事件、未知消息段、未知枚举值**原样透传**；数组中的非法元素跳过而不是整条失败 |
| 语义化 ID | `UserID` / `GroupID` / `Uin` / `MessageSeq` 独立类型，编译期阻断群号与 QQ 号串用 |
| 三种事件通道 | WebSocket（ping/pong + 读超时）/ SSE（多行 `data`、注释保活）/ WebHook（`http.Handler`） |
| 连接生命周期 | 状态机、指数退避重连、`Start` / `Shutdown` / `WaitReady`、握手 401 自动降级 `?access_token=` |
| 可插拔分发 | 并发 / 全局有序 / **按会话有序**（同群有序、跨群并行）、两种背压、逐订阅者 panic 隔离 |
| 解耦调用 | 65 个强类型方法挂在 5 个 service 上，`context` 全链路；未建模端点走 `Call` 逃生舱 |
| 可测试 | `mock` 包提供可编程假协议端（API + WS/SSE），无需真实 QQ 账号 |
| 可度量 | `make bench` / `make cover`；CI 校验 gofmt / vet / test / `-race` / 生成物一致性 |

---

## 安装

```bash
go get github.com/WinterChocolates/YukiChoco
```

要求 **Go 1.22+**。

---

## 快速开始

一个被 @ 时引用回复的群聊机器人：

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := YukiChoco.New(YukiChoco.Config{
		BaseURL:     os.Getenv("MILKY_BASE_URL"), // 例如 http://127.0.0.1:6700
		AccessToken: os.Getenv("MILKY_TOKEN"),
		EventSource: YukiChoco.EventSourceWS,
		// 同一群内串行处理，避免并发回复乱序。
		Dispatch: YukiChoco.DispatchConfig{Mode: YukiChoco.DispatchPerPeerSequential},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Shutdown(context.Background())

	if err := c.Init(ctx); err != nil { // 取登录信息与协议端信息
		log.Fatal(err)
	}

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
		return err
	})

	if err := c.Start(ctx); err != nil { // 启动事件循环（不阻塞）
		log.Fatal(err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := c.WaitReady(readyCtx); err != nil {
		log.Fatal(err)
	}
	log.Printf("已就绪：self_id=%d event_url=%s", c.SelfID().Int64(), c.Config().EventURL)

	<-ctx.Done()
}
```

> `Init` 必须在注册 `GroupRouter().AtMe(...)` 之前调用（谓词在注册时求值）；
> 用 `On` 时无需关心顺序，`c.SelfID()` 在回调执行时读取。

---

## 三种事件通道

| 常量 | 连接方向 | 适用场景 | 备注 |
| --- | --- | --- | --- |
| `EventSourceWS`（默认） | 应用端 → 协议端 `ws(s)://host/event` | 常规部署 | ping/pong + 读空闲超时，**不发**应用层心跳帧 |
| `EventSourceSSE` | 应用端 → 协议端 `http(s)://host/event` | WebSocket 被中间设备阻断 | 单向流，断线靠读空闲超时判定 |
| `EventSourceWebhook` | 协议端 → 应用端 `POST` | 应用端只开 80/443、内网协议端 | 需自行挂载 `c.EventHandler()`，无重连语义 |
| `EventSourceNone` | 无 | 只调用 API（定时推送等） | `Start` 立即就绪 |

WebHook 模式：

```go
c, _ := YukiChoco.New(YukiChoco.Config{
	BaseURL:     "http://10.0.0.2:6700",
	AccessToken: token,
	EventSource: YukiChoco.EventSourceWebhook,
	// 协议端不保证发送 Authorization 头，确认支持后再打开（默认关闭）
	WebHookVerifyToken: true,
})

mux := http.NewServeMux()
mux.Handle("/milky/event", c.EventHandler()) // 挂到任意路径
http.ListenAndServe(":8080", mux)
```

SSE 模式只需 `EventSource: YukiChoco.EventSourceSSE`，`EventURL` 会由 `BaseURL` 推导为
`http(s)://host/event`，也可用 `EventURL` 显式覆盖。

---

## 消息构造

`segment` 包负责拼消息，`types` 包里的结构体也可以直接构造（两者等价）：

```go
segs := types.OutgoingSegments{
	segment.Reply(msg.MessageSeq),
	segment.Text("你好，"),
	segment.Mention(msg.SenderID),
	segment.Image("file:///tmp/a.png"),
}
if _, err := c.Group().SendMessage(ctx, groupID, segs...); err != nil {
	return err
}
```

| 构造器 | 说明 |
| --- | --- |
| `Text` / `Mention` / `MentionAll` | 文本 / @某人 / @全体 |
| `Face` / `LargeFace` | 系统表情 / 超级表情 |
| `Reply` | 引用消息（需对方 `message_seq`） |
| `Image` / `Sticker` / `Record` / `Video` | 资源段，`uri` 支持 `file://` `http(s)://` `base64://` |
| `LightApp` | 小程序卡片 |
| `Forward` / `ForwardWithOptions` | 合并转发（可带 title/preview/summary/prompt） |

资源 URI 助手：

```go
segment.URI("/tmp/a.png")        // 本地路径 → file:///tmp/a.png（Windows: file:///C:/...）
segment.URI([]byte("..."))       // → base64://...
segment.URI(reader)              // ≤8 MiB 内联，超出返回 segment.ErrFileTooLarge
segment.ImageFromFile("/tmp/a.png")
segment.ImageFromBytes(data)
segment.ImageFromReader(reader)
segment.ImageFromURL("https://example.com/a.png")
```

> 超过 `DefaultMaxInlineBytes`（8 MiB）的资源请用 `file://` 或 URL，避免消息体被协议端拒绝；
> 超限**直接报错**，不会静默截断。

接收侧解析：

```go
msg.PlainText()                  // 拼接所有 text 段
msg.Segments.Mentioned(uid)      // 是否 @ 了某人
msg.Segments.MentionsAll()       // 是否 @全体
msg.Segments.ReplySeq()          // 引用段（第一个）
msg.Segments.Images()            // 所有图片段
msg.Segments.Types()             // 段类型序列
msg.GroupMember.DisplayName()    // 群名片优先，其次昵称
```

---

## 事件处理

### 订阅

```go
// 泛型订阅：事件类型由回调参数推导，无需手写 event_type 字符串
off := YukiChoco.On(c, func(ctx context.Context, ev *types.GroupMemberIncreaseEvent) error { ... })
off() // 注销

YukiChoco.Once(c, func(ctx context.Context, ev *types.BotOfflineEvent) error { ... }) // 只触发一次
c.OnEvent(func(ctx context.Context, ev types.Event) error { ... })                   // 所有事件
c.OnRaw(func(ctx context.Context, raw []byte) error { ... })                         // 原始 JSON（同步、需快速返回）
```

未知事件不会丢：SDK 会给出 `*types.UnknownEvent`，`Raw` 中是原始 JSON；
未知消息段同理（`*types.UnknownIncomingSegment`）。

### 分发策略

| `Dispatch.Mode` | 语义 | 实现 |
| --- | --- | --- |
| `DispatchConcurrent`（默认） | 不保证顺序，追求吞吐 | 单队列 + N worker |
| `DispatchSequential` | 全局有序 | 单队列 + 1 worker |
| `DispatchPerPeerSequential` | 同一会话有序、跨会话并行 | N 队列，每队列 1 worker（按群号/QQ 号分片） |

队列满时：`BackpressureDropNewest`（默认，计数 + WARN 日志）或 `BackpressureBlock`
（反压到事件源，可能被协议端判定超时）。panic 会被逐订阅者隔离，事件循环不受影响。

### 中间件

```go
c.Use(
	event.Recover(logger, nil),                    // 兜住中间件/回调的 panic
	event.Timeout(3*time.Second),                  // 每条事件的处理超时（回调需响应 ctx）
	event.Logging(logger, slog.LevelDebug),        // 处理耗时日志
)
```

### 谓词路由

```go
c.GroupRouter().
	AtMe(c.SelfID()).
	Admin().
	Prefix("/mute").
	Handle(func(ctx context.Context, msg *types.GroupMessage) error {
		_, err := c.ReplyGroup(ctx, msg.Group.GroupID, msg.MessageSeq, "已收到指令")
		return err
	})
```

可用谓词：`Group(id)` / `AtMe(uid)` / `MentionsAll()` / `Prefix(s)` / `Admin()` / `Where(fn)`；
每次调用返回新的 Router，因而可从一个基础路由器安全派生多条规则。
通用路由用 `c.Router().Type(...)` / `.Where(...)`。

### 连接状态与统计

```go
c.OnStateChange(func(s YukiChoco.State) { log.Println("state:", s) })
c.State()        // disconnected / dialing / connected / reconnecting / closed
c.Connected()
c.WaitReady(ctx) // 等待首次连接成功

stats := c.Events().Stats() // Handled / Failed / Dropped / Panicked / Queued / Workers / Mode
log.Printf("handled=%d dropped=%d queued=%d", stats.Handled, stats.Dropped, stats.Queued)
```

重连策略通过 `Reconnect` 配置：`InitialDelay`（1s）、`MaxDelay`（60s）、`Multiplier`（2）、
`Jitter`（0.2，负值关闭）、`MaxAttempts`（0 无限）、`Disable`。

---

## API 调用

65 个端点按 IR 分类挂在 5 个 service 上：

| Service | 数量 | 示例 |
| --- | --- | --- |
| `c.System()` | 17 | `GetLoginInfo(ctx)`、`GetFriendList(ctx, req)`、`GetGroupMemberList(ctx, req)` |
| `c.Message()` | 8 | `SendPrivateMessage(ctx, uid, segs...)`、`GetHistoryMessages(ctx, req)`、`RecallGroupMessage(ctx, req)` |
| `c.Friend()` | 6 | `SendFriendNudge`、`DeleteFriend`、`AcceptFriendRequest` |
| `c.Group()` | 22 | `SendMessage(ctx, gid, segs...)`、`SetMemberMute`、`SendAnnouncement`、`GetNotifications` |
| `c.File()` | 12 | `UploadGroupFile`、`GetGroupFiles`、`PersistGroupFile` |

约定：

- 有请求参数的端点使用 `<Endpoint>Request` 结构体，返回 `<Endpoint>Response`；
- 无参数的端点只有 `ctx`（如 `c.System().GetLoginInfo(ctx)`）；
- 发送类端点用变参消息段（`c.Group().SendMessage`、`c.Message().SendPrivateMessage`）；
- 请求结构体的可选字段是**指针 + omitempty**，带默认值的字段零值会被省略，由协议端应用默认值。

常用糖方法：

```go
c.SendGroupText(ctx, gid, "hi")
c.SendPrivateText(ctx, uid, "hi")
c.ReplyGroup(ctx, gid, seq, "hi")
c.ReplyPrivate(ctx, uid, seq, "hi")
c.SendGroupSegments(ctx, gid, segs...)
```

未建模的端点（或协议端自扩展能力）：

```go
var out struct {
	Cookies []string `json:"cookies"`
}
if err := c.Call(ctx, "get_cookies", map[string]any{"domain": "qun.qq.com"}, &out); err != nil {
	return err
}

raw, err := c.CallRaw(ctx, "get_login_info", nil) // 直接拿 data 的原始 JSON
if err != nil {
	return err
}
log.Println(string(raw))
```

---

## 文件

```go
// 上传：路径 / []byte / io.Reader / URL 都可以
if _, err := c.UploadGroupFileFrom(ctx, gid, "/tmp/a.txt", "a.txt"); err != nil {
	return err
}
if _, err := c.UploadGroupFileWith(ctx, gid, "/sub", "a.txt", reader); err != nil {
	return err
}
if _, err := c.UploadPrivateFileWith(ctx, uid, "a.txt", data); err != nil {
	return err
}

// 下载：先换临时链接，再流式读取（调用方负责 Close）
rc, err := c.DownloadResource(ctx, resourceID)
if err != nil {
	return err
}
defer rc.Close()

// 按 file_id 下载群文件；私聊文件把 groupID 传 0 并提供 TriSHA1 哈希
body, err := c.DownloadFile(ctx, gid, fileID, "")
if err != nil {
	return err
}
defer body.Close()
```

---

## 错误处理

用哨兵错误判别，**不要匹配文案**：

```go
if _, err := c.Group().SetMemberMute(ctx, req); err != nil {
	switch {
	case errors.Is(err, YukiChoco.ErrNotLoggedIn):        // retcode -403
	case errors.Is(err, YukiChoco.ErrBadRequest):         // retcode -400
	case errors.Is(err, YukiChoco.ErrNotFound):           // retcode -404（资源不存在）
	case errors.Is(err, YukiChoco.ErrUnauthorized):       // HTTP 401
	case errors.Is(err, YukiChoco.ErrEndpointNotFound):   // HTTP 404（端点不存在）
	case errors.Is(err, YukiChoco.ErrUnexpectedStatus):   // 其它 HTTP 状态（5xx 会重试）
	case errors.Is(err, YukiChoco.ErrClosed):             // 客户端已 Shutdown
	}
	var apiErr *YukiChoco.APIError
	if errors.As(err, &apiErr) {
		log.Println(apiErr.Endpoint, apiErr.Retcode, apiErr.Message)
	}
}
```

重试规则：**只有幂等端点**（`get_` `set_` `mark_` `delete_` `move_` `rename_` `accept_`
`reject_` `quit_` `persist_`）在网络错误/5xx 时自动重试（默认 3 次，指数退避）。
`send_*`、`create_*` **默认不重试**——Milky 没有幂等键，重试会重复发消息；
确需重试时设置 `Retry.RetryNonIdempotent = true`。

取消与超时：所有方法都接受 `ctx`；单次调用另有 `APITimeout`（默认 15s）。
限流用 `RateLimit.QPS/Burst`（默认关闭），令牌桶零依赖实现、无常驻 goroutine。

---

## 包结构

```
github.com/WinterChocolates/YukiChoco   package YukiChoco ← Client / Config / service / 生命周期
├── types          协议数据模型：实体 / 事件 / 消息段 / API 模型（*_gen.go 由 IR 生成）
├── event          分发器（3 种策略）/ 泛型 On[T] / 中间件 / 谓词路由
├── transport      事件源抽象 + WS / SSE / WebHook 实现
├── segment        消息段构造助手、资源 URI 归一化
├── mock           可编程假协议端（API + WS/SSE）
├── examples/      echo-bot / webhook-bot / sse-client / admin-bot
├── internal/jsonx 宽容整数、Tagged 编解码、跳过非法元素
├── internal/codegen  Milky IR → Go 的生成器
└── protocol/protocol-1.3.0.json  锁定的 Milky IR
```

依赖方向单向：`YukiChoco → event / transport / segment / types`，`types` 不依赖任何其它包，
可单独引用作为协议类型库。

---

## 开发

```bash
make generate     # IR → types/*_gen.go + service_*_gen.go
make check        # generate + fmt + vet + test + race
make test         # go test ./...
make race         # go test -race ./...
make cover        # 覆盖率（排除 *_gen.go 与 examples/）
make bench        # 基准测试
make spec-check   # 拉取上游最新 IR 并与本地锁定版本 diff
```

**约定**

- `types/*_gen.go`、`service_*_gen.go` 由 `internal/codegen` 生成，**禁止手改**。
  需要便捷方法时写到 `*_ext.go`（如 `segment_ext.go`）。
- 新增端点或枚举时，必须先在 `internal/codegen/tables.go` 登记命名（service、方法名、枚举名）；
  未登记会直接生成失败，避免出现 `State`、`Sex` 这类过泛的类型名。
- `TestGolden` 会把 IR 重新生成到临时目录后逐字节比对（忽略行尾差异），手改生成物或忘记
  `make generate` 都会失败。
- 协议字段演进遵循官方兼容性契约：请求侧只加可选/带默认值字段，响应侧只加字段。
- 设计与基准的详细文档保留在本地 `docs/`（不随仓库发布）。

---

## 示例

```bash
MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/echo-bot
MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx LISTEN=:8080 go run ./examples/webhook-bot
MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/sse-client
MILKY_BASE_URL=http://127.0.0.1:6700 MILKY_TOKEN=xxx go run ./examples/admin-bot
```

| 示例 | 演示内容 |
| --- | --- |
| `echo-bot` | WS + `Init` + `On` + 引用回复 |
| `webhook-bot` | `EventSourceWebhook` + `EventHandler` 挂载 + 优雅关闭 |
| `sse-client` | `EventSourceSSE`、中间件、状态回调、运行统计 |
| `admin-bot` | 谓词路由（`AtMe().Admin().Prefix()`）、禁言 / 撤回 / 公告 / 上传群文件 |


---

## 参考

- Milky 协议文档：<https://milky.ntqqrev.org/>
- 通信规范：<https://milky.ntqqrev.org/guide/communication>
- 兼容性规范：<https://milky.ntqqrev.org/guide/compatibility>
- JSON Schema：<https://milky.ntqqrev.org/raw/json-schema/schema.json>
- 协议端：Lagrange.Milky · <https://github.com/LagrangeDev/Lagrange.Core>
- 其它 Go 实现：[Szzrain/Milky-go-sdk](https://github.com/Szzrain/Milky-go-sdk)、[KomeiDiSanXian/remilia](https://github.com/komeidisanxian/remilia)

---

## License

[MIT](LICENSE) © 2026 WinterChocolates
