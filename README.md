# YukiChoco

**Milky 协议的 Go SDK** — 面向 QQ 机器人应用端，对齐 [Milky 1.3](https://milky.ntqqrev.org/)，类型由官方 [Milky IR](https://www.npmjs.com/package/@saltify/milky-protocol) 代码生成。

```go
import "github.com/WinterChocolates/YukiChoco"
```

- **协议端**：Lagrange.Milky、Yogurt、LuckyLilliaBot 等
- **应用端**：本 SDK（通过 HTTP 调 `/api/{endpoint}`，通过 WebSocket / SSE / WebHook 收事件）
- **不是**：QQ 协议实现、bot 框架（无插件/权限系统）、OneBot 兼容层

> 当前状态：**M0–M1 已完成** —— 协议数据模型（IR 生成）、HTTP API 调用层（65 个强类型方法 + `Call` 逃生舱）、幂等重试、限流、脱敏日志与契约测试。事件订阅（WS/SSE/WebHook）在 M2。见 [Roadmap](#roadmap)。

---

## 特性

| 能力 | 说明 |
| --- | --- |
| 协议完整 | Milky 1.3：**65** 个端点、**21** 个事件、**24** 种消息段（接收 14 / 发送 10）、16 个公共结构体 |
| 类型由 IR 生成 | 不存在手抄字段；协议升级只需 `make generate`，字段级 `since` / 默认值全部保留 |
| 宽容解码 | 未知事件、未知消息段、未知枚举值**原样透传**，不因协议端升级而崩；数组中的非法元素被跳过 |
| 语义化 ID | `UserID` / `GroupID` / `Uin` / `MessageSeq` 各自独立类型，编译期阻断群号与 QQ 号串用 |
| 零第三方依赖 | 当前仅标准库；WebSocket 在 M2 引入 `gorilla/websocket` |
| API 调用层 | 65 个强类型方法挂在 5 个 service 上（`c.Group().SendMessage(...)`），`context` 全链路，统一信封解析与 `retcode` 归类 |
| 可校验 | golden 测试锁定生成物、AST 校验方法覆盖度、负例测试阻断协议漂移、`-race` 全绿 |

---

## 安装

```bash
go get github.com/WinterChocolates/YukiChoco
```

要求 **Go 1.22+**。

---

## 快速开始

### 1. 解码事件（现在即可用）

```go
package main

import (
	"fmt"

	"github.com/WinterChocolates/YukiChoco/types"
)

// payload 是从 WebSocket / SSE / WebHook 收到的原始事件 JSON。
func handle(payload []byte) error {
	ev, err := types.UnmarshalEvent(payload)
	if err != nil {
		return err
	}

	fmt.Println("event:", ev.EventType(), "self:", ev.Meta().SelfID)

	recv, ok := ev.(*types.MessageReceiveEvent)
	if !ok {
		return nil
	}
	msg, ok := recv.GroupMessage()
	if !ok {
		return nil
	}

	fmt.Printf("[%s] %s: %s\n", msg.Group.GroupName, msg.GroupMember.DisplayName(), msg.PlainText())

	if msg.Segments.Mentioned(ev.Meta().SelfID) {
		if seq, ok := msg.Segments.ReplySeq(); ok {
			fmt.Println("引用了消息", seq)
		}
	}
	return nil
}

// 未识别的事件不会被丢弃：ev 会是 *types.UnknownEvent，Raw 里是原始 JSON。
```

### 2. 构造发送消息（现在即可用）

```go
segs := types.OutgoingSegments{
	&types.OutgoingReplySegment{MessageSeq: 23333},
	&types.OutgoingTextSegment{Text: "你好，"},
	&types.OutgoingMentionSegment{UserID: 111222333},
	&types.OutgoingImageSegment{URI: "file:///tmp/a.png"},
}

raw, err := json.Marshal(segs)
// [{"type":"reply","data":{"message_seq":23333}},
//  {"type":"text","data":{"text":"你好，"}},
//  {"type":"mention","data":{"user_id":111222333}},
//  {"type":"image","data":{"uri":"file:///tmp/a.png"}}]
```

`uri` 支持 `file://`、`http(s)://`、`base64://` 三种形式（M3 会提供 `segment.ImageFromFile` 等助手）。

### 3. 调用 API（M1 起可用）

```go
import (
	"context"
	"fmt"

	"github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/types"
)

c, err := YukiChoco.New(YukiChoco.Config{
	BaseURL:     "http://127.0.0.1:6700",
	AccessToken: "your-token", // 也可用 YukiChoco.Config{}.AccessToken 留空表示不鉴权
})
if err != nil {
	panic(err)
}
defer c.Close()

ctx := context.Background()

// 拉取协议端与登录信息（此后 SelfID / ProtocolInfo 可用）
if err := c.Init(ctx); err != nil {
	panic(err)
}
fmt.Println("登录为", c.SelfID())

// 发送群消息：service 子对象 + 变参消息段
res, err := c.Group().SendMessage(ctx, types.GroupID(987654321),
	&types.OutgoingReplySegment{MessageSeq: 23333},
	&types.OutgoingTextSegment{Text: "你好"},
)
if err != nil {
	panic(err)
}
fmt.Println("消息序列号", res.MessageSeq)

// 其余端点使用请求结构体（service 划分与 Milky IR 的分类一致）
friends, err := c.System().GetFriendList(ctx, &types.GetFriendListRequest{NoCache: true})
group, err := c.System().GetGroupInfo(ctx, &types.GetGroupInfoRequest{GroupID: 987654321})
members, err := c.System().GetGroupMemberList(ctx, &types.GetGroupMemberListRequest{GroupID: 987654321})
_, _, _, _ = friends, group, members, err

// 未建模的端点走逃生舱
var raw struct {
	Cookies []string `json:"cookies"`
}
err = c.Call(ctx, "get_cookies", map[string]any{"domain": "qun.qq.com"}, &raw)
```

API 方法挂在 **service 子对象**上：`c.System()` / `c.Message()` / `c.Friend()` / `c.Group()` / `c.File()`。
群相关操作聚合在 `Group()`（因此是 `c.Group().SendMessage(...)` 而不是 `SendGroupMessage`），私聊发送在 `c.Message().SendPrivateMessage(...)`。
常用组合还有手写便捷方法：`c.SendGroupText`、`c.ReplyGroup`、`c.SendGroupSegments`。

**错误处理**用哨兵错误判别，而不是文案：

```go
if _, err := c.Group().Quit(ctx, &types.QuitGroupRequest{GroupID: 1}); err != nil {
	switch {
	case errors.Is(err, YukiChoco.ErrNotLoggedIn):   // retcode -403
	case errors.Is(err, YukiChoco.ErrBadRequest):    // retcode -400
	case errors.Is(err, YukiChoco.ErrNotFound):      // retcode -404
	case errors.Is(err, YukiChoco.ErrUnauthorized):  // HTTP 401
	}
	var apiErr *YukiChoco.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Endpoint, apiErr.Retcode)
	}
}
```

### 4. 事件订阅（M2 落地后可用）

```go
c, _ := YukiChoco.New(YukiChoco.Config{
	BaseURL:     "http://127.0.0.1:6700",
	AccessToken: "your-token",
	EventSource: YukiChoco.EventSourceWS, // webhook / sse / none
})

self := c.SelfID()
c.OnMessageReceive(func(ctx context.Context, ev *types.MessageReceiveEvent) error {
	msg, ok := ev.GroupMessage()
	if !ok || !msg.Segments.Mentioned(self) {
		return nil
	}
	_, err := c.Group().SendMessage(ctx, msg.Group.GroupID,
		&types.OutgoingReplySegment{MessageSeq: msg.MessageSeq},
		&types.OutgoingTextSegment{Text: "你好"},
	)
	return err
})

if err := c.Start(context.Background()); err != nil {
	panic(err)
}
defer c.Shutdown(context.Background())
```

---

## 包结构

```
github.com/WinterChocolates/YukiChoco     package YukiChoco  ← Client / Config / 5 个 service / 事件分发（M2）
├── types                                 协议数据模型：实体 / 事件 / 消息段 / API 模型（已可用）
├── event                                 事件分发、泛型 On[T]、中间件、路由（M2）
├── segment                               消息段构造 DSL、媒体 uri 助手（M3）
├── mock                                  可编程假协议端，用于测试（M3）
├── internal/jsonx                        宽容整数、Tagged 编解码、跳过非法元素
├── internal/codegen                      Milky IR → Go（生成器本身）
└── protocol/protocol-1.3.0.json          锁定的 Milky IR（sha256 2df17982…）
```

导入路径与包名一致，因此直接写 `YukiChoco.New(...)`。子包入口是 `types`（首个可用包）与后续的 `event` / `segment`。

---

## 开发

```bash
make generate     # 由 protocol/protocol-1.3.0.json 重新生成 types/
make check        # generate + fmt + vet + test + race + 生成物 diff
make spec-check   # 拉取上游最新 IR 并与本地锁定版本 diff
make test         # go test ./...
make race         # go test -race ./...
```

**约定**

- `types/*_gen.go` 全部由 `internal/codegen` 生成，**禁止手改**。需要的便捷方法写在 `*_ext.go`（如 `segment_ext.go`、`message_ext.go`）。
- 新增端点或枚举时，生成器会先要求在 `internal/codegen/tables.go` 里登记命名；未登记直接生成失败，避免出现 `State`、`Sex` 这类过泛的类型名。
- `TestGolden` 会重新生成到临时目录并逐字节比对，手改生成物或忘记 `make generate` 都会失败。
- 协议字段新增属于**向后兼容**：请求侧只加可选/带默认值字段，响应侧只加字段。

**测试**

```bash
go test ./...          # 事件解码、段往返、未知内容透传、非法元素跳过、
                       # ID 宽容解析、omitempty 语义、生成物一致性
go test -race ./...
```

---

## 兼容性注意事项

这几条是从协议规范推导出的**硬约束**，也是与其它实现的行为差异点：

| 事项 | 行为 |
| --- | --- |
| 事件丢失 | Milky 无 ack、无序号重放，事件源（WS/SSE/WebHook）断开期间的事件会丢失，SDK **不承诺**重连补发 |
| 发送不自动重试 | 无幂等键，`send_*` 重试会造成重复消息；仅幂等端点（`get_*` / `set_*` 等）在网络错误时重试 |
| 可选字段 | 指针 + `omitempty`；带默认值的字段用值类型 + `omitempty`，让协议端应用自己的默认值（如 `parent_folder_id="/"`、`limit=20`） |
| ID 精度 | 一律 `int64` 语义化类型；解码接受数字或数字字符串，拒绝浮点；编码始终输出 JSON 数字 |
| 未知内容 | `UnknownEvent` / `UnknownIncomingSegment` / `UnknownIncomingMessage` 保留原始 JSON，序列化原样回填 |
| 数组容错 | 消息段列表逐元素解码，坏元素上报后跳过，不影响整条消息 |
| 心跳 | 不使用应用层心跳帧（Milky 协议里没有），只用 WebSocket ping/pong + 读超时 |
| 鉴权 | `Authorization: Bearer <token>`；事件通道可用 `?access_token=`；token 不会出现在日志与错误信息中 |

**协议端适配**：`get_impl_info` 返回的 `milky_version` 与 `types.APISince` 可用于能力探测（例如 `persist_group_file` 需要 1.3）。

---

## Roadmap

| 里程碑 | 内容 | 状态 |
| --- | --- | --- |
| **M0** | 单模块骨架；IR 锁定；`internal/jsonx`；`internal/codegen`；`types` 全量生成；契约与行为测试 | ✅ 完成 |
| **M1** | `Client` / `Config`；`apicall.go`（信封解析、幂等重试、限流、脱敏日志、`Call` 逃生舱）；5 个 service（65 方法）生成；`errors.go`；32 个测试用例 | ✅ 完成 |
| **M2** | WebSocket 事件源、连接状态机与自动重连、事件分发（并发 / 全局有序 / 会话有序）、中间件、泛型 `On[T]` | 计划 |
| **M3** | SSE、WebHook（`http.Handler`）、`segment` DSL、文件上传下载、`mock` 协议端、`examples/` | 计划 |
| **M4** | 与 Lagrange.Milky 真机 e2e、benchmark、发布 v0.1.0 | 计划 |

详细需求、验收标准与设计取舍见 **[docs/design-requirements.md](docs/design-requirements.md)**（8 组功能需求 FR-1~FR-8、9 条非功能需求、10 条 ADR）。

---

## 参考

- Milky 协议文档：<https://milky.ntqqrev.org/>
- 通信规范：<https://milky.ntqqrev.org/guide/communication>
- 兼容性规范：<https://milky.ntqqrev.org/guide/compatibility>
- JSON Schema：<https://milky.ntqqrev.org/raw/json-schema/schema.json>
- 协议端（C#）：Lagrange.Milky · <https://github.com/LagrangeDev/Lagrange.Core>
- 其它 Go 实现：[Szzrain/Milky-go-sdk](https://github.com/Szzrain/Milky-go-sdk)、[KomeiDiSanXian/remilia](https://github.com/komeidisanxian/remilia)

---

## 贡献

1. 改协议数据？改 `protocol/protocol-1.3.0.json`（或升级版本）→ `make generate` → `make check`。
2. 新增协议端点的 SDK 方法？在 `internal/codegen/tables.go` 登记 service 与方法名。
3. 提交前请确保 `make check` 全绿（含 `-race` 与生成物 diff）。

---

## License

尚未添加 LICENSE 文件（发布 v0.1.0 前确定）。在此之前请勿用于商业分发。
