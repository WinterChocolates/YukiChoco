# YukiChoco

[![CI](https://github.com/WinterChocolates/YukiChoco/actions/workflows/ci.yml/badge.svg)](https://github.com/WinterChocolates/YukiChoco/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![OneBot](https://img.shields.io/badge/protocol-OneBot%20v11-12B7F5)](https://github.com/botuniverse/onebot-11)

**OneBot v11 的 Go SDK**：强类型动作与事件、三套传输、按会话有序的事件分发，
以及一个可编程的假协议端。

```go
import "github.com/WinterChocolates/YukiChoco"
```

- **协议端**（你连接的对象）：NapCat、LLOneBot、Lagrange.OneBot、go-cqhttp 等
- **应用端**（本 SDK）：调用 OneBot v11 API、接收 `message` / `notice` / `request` / `meta_event`
- **不负责**：QQ 协议与登录、bot 框架（插件/权限/命令系统）、OneBot v12 兼容

| 项 | 数量 / 状态 |
| --- | --- |
| 协议 | OneBot **v11（11.0）** + 标注为扩展的常用 go-cqhttp 动作 |
| 强类型动作 | **69** 个（System 13 / Message 12 / Friend 5 / Group 36 / File 3） |
| 事件类型 | **20** 个具体事件 + `UnknownEvent` |
| 消息段 | **26** 种 |
| 传输 | HTTP + WebHook / 正向 WS / 反向 WS / 纯 HTTP |
| 测试 | 72 个顶层用例（含子测试），`-race` 全绿 |
| 生产依赖 | 仅 `github.com/gorilla/websocket` |

> 0.2.0 是从 Milky 协议到 OneBot v11 的**破坏性重构**，迁移映射见文末。

---

## 特性

| 能力 | 说明 |
| --- | --- |
| 强类型 API | 5 个 service 共 69 个方法；未建模动作走 `Call` / `CallInto` |
| 三套传输 | 正向 WS（通用单连接或 `/api` + `/event` 分离）、反向 WS、HTTP + WebHook |
| `echo` 关联 | WS 上并发调用通过唯一 `echo` 关联响应，支持超时与取消 |
| 双消息格式 | 接收自动识别数组与 CQ 码；发送使用数组格式；提供完整 CQ 编解码 |
| 宽容解码 | 未知事件/消息段/字段不失败；坏数组元素跳过；`UnknownEvent.Raw` 透传 |
| 事件分发 | 并发 / 全局有序 / 按会话有序；丢弃或阻塞背压；逐订阅者 panic 隔离 |
| 路由原语 | 群/私聊路由器：`AtMe` / `Prefix` / `Regex` / `Admin` / `Owner` 等 |
| 生产可用 | 自动重连、令牌桶限流、幂等重试、优雅关闭、脱敏日志 |
| 可测试 | `mock` 假协议端支持 HTTP、正向 WS、反向 WS，无需真实 QQ 账号 |

---

## 安装

```bash
go get github.com/WinterChocolates/YukiChoco
```

要求 **Go 1.22+**。

---

## 快速开始

### 1. 正向 WebSocket（通用单连接）

```go
c, err := YukiChoco.New(YukiChoco.Config{
    Transport: YukiChoco.TransportForwardWS,
    ForwardWS: YukiChoco.ForwardWSConfig{
        URL: "ws://127.0.0.1:3001/", // 事件与 API 共用
    },
})
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// 必须先注册 handler，再 Start，避免漏掉连接后立刻到达的事件。
event.On(c.Events(), func(ctx context.Context, msg *types.GroupMessageEvent) error {
    if !msg.AtMe(c.SelfID()) {
        return nil
    }
    _, err := c.SendGroupMessage(ctx, msg.GroupID, types.Message{
        segment.Reply(msg.MessageID),
        segment.Text("收到：" + msg.PlainText()),
    })
    return err
})

if err := c.Start(ctx); err != nil {
    log.Fatal(err)
}
if err := c.WaitReady(ctx); err != nil {
    log.Fatal(err)
}
_ = c.Init(ctx) // 填充 SelfID / 版本信息，可选
```

分离模式（协议端分别暴露 `/api` 与 `/event`）：

```go
YukiChoco.Config{
    Transport: YukiChoco.TransportForwardWS,
    ForwardWS: YukiChoco.ForwardWSConfig{
        URL:      "ws://127.0.0.1:6700/api",
        EventURL: "ws://127.0.0.1:6700/event",
    },
}
```

若同时设置 `HTTP.APIBaseURL`，API 调用改走 HTTP，WS 只收事件。

### 2. 反向 WebSocket

SDK 监听，协议端主动连接（适合协议端在内网、bot 在公网的场景）：

```go
c, _ := YukiChoco.New(YukiChoco.Config{
    Transport:   YukiChoco.TransportReverseWS,
    AccessToken: "secret",
    ReverseWS: YukiChoco.ReverseWSConfig{
        Listen: "0.0.0.0:8080",
        Path:   "/onebot/v11/ws",
    },
})
defer c.Close()

if err := c.Start(ctx); err != nil { // 启动监听
    log.Fatal(err)
}
```

协议端配置反向 WS 地址 `ws://<bot-host>:8080/onebot/v11/ws`，
可选 `Authorization: Bearer secret` 或 `?access_token=secret`。

### 3. HTTP API + WebHook

```go
c, _ := YukiChoco.New(YukiChoco.Config{
    Transport:   YukiChoco.TransportHTTP,
    AccessToken: "secret",
    HTTP:        YukiChoco.HTTPConfig{APIBaseURL: "http://127.0.0.1:5700"},
})
defer c.Close()
_ = c.Start(context.Background())

http.Handle("/onebot/v11/events", c.WebhookHandler())
log.Fatal(http.ListenAndServe(":8081", nil))
```

只发送、不接收事件时使用 `TransportNone`；`WebhookHandler()` 返回 `nil`。

---

## 配置

| 字段 | 说明 | 默认 |
| --- | --- | --- |
| `Transport` | `http` / `forward-ws` / `reverse-ws` / `none` | `http` |
| `AccessToken` | HTTP / WS / 反向 WS / WebHook 鉴权 | 空 |
| `HTTP.APIBaseURL` | HTTP API 根地址 | HTTP/None 模式为 `http://127.0.0.1:5700` |
| `ForwardWS.URL` | 正向 WS 地址；`/api` 结尾会自动推导 `/event` | — |
| `ForwardWS.EventURL` | 单独的事件连接；留空为通用单连接 | 空 |
| `ForwardWS.Reconnect` | 初始/最大延迟、倍数、抖动、最大尝试 | 1s / 60s / 2 / 0.2 / 无限 |
| `ReverseWS.Listen` | 反向 WS 监听地址 | `127.0.0.1:8080` |
| `ReverseWS.Path` | 反向 WS 路径 | `/onebot/v11/ws` |
| `Webhook.Secret` | 校验 `X-Signature: sha1=...`（go-cqhttp 约定） | 空 |
| `Webhook.VerifyToken` | 校验 `Authorization`；设置 token 时自动开启 | 自动 |
| `APITimeout` | 单次 API 调用超时 | 15s |
| `Retry` | 重试次数/退避；默认只重试 `get_*`、`can_send_*` | 3 / 200ms / 3s |
| `RateLimit` | 令牌桶 QPS / Burst | 关闭 |
| `Dispatch` | 分发模式、worker、队列、背压、handler 超时 | 并发 + 丢弃 |
| `Logger` | `*slog.Logger` | 丢弃 |
| `DebugDump` | 写入请求/响应调试行 | 空 |
| `MaxResponseBytes` | 响应/单条事件上限 | 16 MiB |

`Config.String()` 与 `LogValue()` 会自动脱敏 token 和 URL 查询参数。

---

## API 调用

```go
id, err := c.Group().SendMessage(ctx, groupID, types.Message{
    segment.At(userID), segment.Text(" 你好"),
})

info, err := c.Group().GetMemberInfo(ctx, groupID, userID, false)
files, err := c.Group().GetRootFiles(ctx, groupID)
url, err := c.Group().GetFileURL(ctx, groupID, fileID, 0)
```

常用组合（`api_ext.go`）：

```go
c.SendGroupText(ctx, groupID, "hi")
c.SendPrivateText(ctx, userID, "hi")
c.ReplyGroup(ctx, groupID, messageID, "收到")
c.ReplyGroupMessage(ctx, msgEvent, "收到")
c.SendGroupImage(ctx, groupID, "/tmp/a.png") // 路径 / URL / []byte / io.Reader
c.UploadGroupFileFrom(ctx, groupID, "/tmp/a.txt", "a.txt")
```

未建模或协议端私有动作：

```go
var out struct {
    Count int `json:"count"`
}
err := c.CallInto(ctx, "get_group_member_count", map[string]any{"group_id": 123}, &out)

raw, err := c.Call(ctx, "some_extension_action", map[string]any{"x": 1})
```

### 错误处理

```go
var apiErr *YukiChoco.APIError
if errors.As(err, &apiErr) {
    log.Printf("retcode=%d status=%s", apiErr.Retcode, apiErr.Status)
}

switch {
case errors.Is(err, YukiChoco.ErrUnauthorized):
case errors.Is(err, YukiChoco.ErrBadRequest):
case errors.Is(err, YukiChoco.ErrNotFound):
case errors.Is(err, YukiChoco.ErrForbidden):
case errors.Is(err, YukiChoco.ErrNotConnected):
}
```

- `retcode 0` 成功，`retcode 1` 异步受理（`delete_msg` 等）。
- `100/102` → `ErrBadRequest`，`103` → `ErrNotFound`，`104` → `ErrUnauthorized`，
  `200` 段 → `ErrForbidden`。
- `send_*` 默认不重试；需要时设置 `Retry.RetryNonIdempotent = true`。

---

## 事件

```go
event.On(c.Events(), func(ctx context.Context, ev *types.GroupMessageEvent) error { ... })
event.On(c.Events(), func(ctx context.Context, ev *types.GroupRequestEvent) error { ... })
event.On(c.Events(), func(ctx context.Context, ev *types.FriendAddEvent) error { ... })
```

可订阅的具体事件：

```
message.private / message.group
notice.group_upload / group_admin / group_decrease / group_increase / group_ban
notice.friend_add / group_recall / friend_recall / notify / group_card
notice.offline_file / client_status / essence / group_msg_emoji_like
request.friend / request.group
meta.lifecycle / meta.heartbeat
```

`notify` 用 `SubType` 区分 `honor` / `poke` / `lucky_king`，并提供
`IsPoke()` / `IsHonor()` / `IsLuckyKing()`。

### 路由

```go
c.GroupRouter().
    Group(groupID).
    AtMe(c.SelfID()).
    Prefix("/echo").
    Handle(func(ctx context.Context, msg *types.GroupMessageEvent) error {
        return c.ReplyGroupMessage(ctx, msg, msg.PlainText())
    })

c.PrivateRouter().From(userID).Contains("ping").Handle(...)
```

群路由还支持 `AtAll()` / `From` / `Suffix` / `Regex` / `Role` / `Admin` / `Owner`。

### 分发策略

```go
YukiChoco.Config{
    Dispatch: YukiChoco.DispatchConfig{
        Mode:           YukiChoco.DispatchPerPeerSequential, // 同群有序、跨群并行
        Workers:        8,
        Buffer:         1024,
        Backpressure:   YukiChoco.BackpressureDropNewest,
        HandlerTimeout: 30 * time.Second,
    },
}
```

中间件：`event.Recover`、`event.Timeout`、`event.Logging`、`event.Filter`，
用 `c.Events().Use(...)` 安装。

---

## 消息与 CQ 码

```go
msg := types.Message{
    segment.Reply(123),
    segment.At(456),
    segment.Text(" 你好"),
    segment.Image("/tmp/a.png"),
}
```

- 发送始终使用数组格式（OneBot 协议端普遍接受）。
- 接收同时支持数组与 CQ 字符串；`msg.PlainText()` 取纯文本。
- CQ 工具：

```go
cq := msg.CQString()
back, err := types.ParseCQ("[CQ:at,qq=123] hi")
```

- 媒体来源：路径 / `http(s)://` / `base64://` / `[]byte` / `io.Reader`。
  超过 8 MiB 的 `[]byte`/Reader 会返回 `segment.ErrFileTooLarge`，请改用 URL 或路径。
- 合并转发节点用 `segment.Node(userID, nickname, content...)`。

---

## 生命周期与并发

```go
c.Start(ctx)      // HTTP/None/反向 WS 立即就绪；正向 WS 后台拨号
c.WaitReady(ctx)  // 等待事件通道就绪
c.State()         // disconnected / dialing / connected / reconnecting / closed
c.StateChanges()  // 状态变化 channel
c.Shutdown(ctx)   // 停止重连、关闭连接、等待 handler 排空
c.Close()         // 最多等待 5s 的 Shutdown
```

- 所有导出方法可并发调用。
- 多账号 = 多个 `*Client`；反向 WS 多连接共享 `echo` 关联，API 调用使用其中一条连接。
- OneBot 没有 ack/重放，断线期间的事件会丢失。

---

## 测试

`mock` 提供可编程假协议端：

```go
p := mock.New(10000)
defer p.Close()

c, _ := YukiChoco.New(YukiChoco.Config{
    Transport: YukiChoco.TransportForwardWS,
    ForwardWS: YukiChoco.ForwardWSConfig{URL: p.WSURL()},
})
defer c.Close()

c.Start(ctx)
c.WaitReady(ctx)
p.Push(&types.GroupMessageEvent{...})

call, _ := p.WaitCall(ctx, types.ActionSendGroupMsg)
```

也支持 `p.ConnectReverse(ctx, url, token)` 驱动反向 WS。

```bash
make test     # go test ./...
make race     # go test -race ./...
make check    # fmt + vet + test + race
make cover    # 覆盖率（排除 examples）
```

---

## 范围与限制

- 未实现 OneBot v12、Satori、Milky 适配层。
- 扩展动作（`get_group_notice`、`_send_group_notice`、精华消息、`download_file`、
  `set_group_remark` 等）以注释标注，协议端不支持时返回 `ErrActionFailed`。
- 发送统一数组格式；极少数只接受 CQ 字符串的旧实现需要自行调用
  `Call` 并传 `msg.CQString()`。
- 反向 WS 的多账号路由未建模：多个连接共享 pending map，API 调用使用任意一条连接。
- 不内置插件、命令、权限系统。

---

## 从 0.1.x（Milky）迁移

| Milky（0.1.x） | OneBot v11（0.2.x） |
| --- | --- |
| `SendGroupMessage(ctx, group, segs...)` | `c.Group().SendMessage(ctx, group, msg)` |
| `SendPrivateMessage(ctx, user, segs...)` | `c.Message().SendPrivate(ctx, user, msg)` |
| `MessageSeq` | `types.MessageID` |
| `OutgoingSegment` | `types.Segment` / `types.Message` |
| `Config.EventSource = ws/sse/webhook` | `Config.Transport = forward-ws/reverse-ws/http` |
| `get_impl_info` | `get_version_info` |
| `recall_*` | `delete_msg` |
| `types.GetGroupInfoResponse` | `types.GroupInfo` |

`internal/codegen`、`protocol/protocol-1.3.0.json`、SSE 传输已移除。

---

## 目录

```
.
├── client.go / config.go / apicall.go / errors.go   客户端、配置、调用与错误
├── service_{system,message,friend,group,file}.go    强类型动作
├── api_ext.go                                       常用组合与文件上传
├── types/      ID、消息段、CQ、实体、事件、动作
├── segment/    消息段构造器与媒体来源处理
├── event/      分发器、路由、中间件、泛型订阅
├── transport/  正向 WS、反向 WS、WebHook
├── mock/       可编程假协议端
└── examples/   echo-bot / http-bot / reverse-ws-bot
```

## License

MIT
