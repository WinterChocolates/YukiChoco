# Changelog

本项目的所有重要变更都记录在此文件，格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.2.0] - 2026-10-28

**破坏性重构：协议基线由 Milky 1.3 切换为 OneBot v11。** 0.1.x 的 `types`、
`segment`、`Config.EventSource`、SSE 传输与 `internal/codegen` 均已移除。

### 新增

- **协议数据模型（`types`）**：语义化 `UserID` / `GroupID` / `MessageID`（兼容数字与
  数字字符串）、26 种消息段、消息数组与 CQ 码双格式解码、20 个具体事件 +
  `UnknownEvent`、69 个动作的请求/响应类型。
- **三套传输**：正向 WebSocket（通用单连接或 `/api` + `/event` 分离）、反向
  WebSocket（SDK 监听，支持 `Authorization` / `?access_token=` 与 `X-Self-ID`）、
  HTTP + WebHook（`Authorization` 与 `X-Signature` HMAC-SHA1）。
- **WS `echo` 关联**：并发调用通过唯一 `echo` 关联响应，支持 `context` 超时与取消；
  断线重连后 pending 调用返回明确错误。
- **强类型 service**：System / Message / Friend / Group / File 共 69 个方法，
  含常用 go-cqhttp 扩展；未建模动作走 `Call` / `CallInto`。
- **消息构造（`segment`）**：文本、@、图片、语音、视频、回复、合并转发节点、
  JSON/XML 卡片、戳一戳等；媒体来源统一处理路径 / URL / `base64://` / 字节 / Reader。
- **CQ 码工具**：`Message.CQString()`、`types.ParseCQ()`，完整转义与宽容解析。
- **事件分发（`event`）**：并发 / 全局有序 / 按会话有序、两种背压、panic 隔离、
  中间件、泛型 `On` / `Once`、群与私聊路由谓词。
- **可靠性**：正向 WS 指数退避重连、HTTP/WS 统一超时、令牌桶限流、
  幂等动作自动重试、`Start` / `WaitReady` / `Shutdown` / `StateChanges`。
- **可测试性（`mock`）**：可编程假协议端，支持 HTTP、正向 WS 与反向 WS 连接。
- **示例**：`echo-bot`（正向 WS）、`http-bot`（HTTP + WebHook）、
  `reverse-ws-bot`（反向 WS）。

### 变更

- 发送消息统一使用数组格式；接收同时支持数组与 CQ 字符串。
- 错误模型改为 OneBot `status` / `retcode`：`100/102` → `ErrBadRequest`，
  `103` → `ErrNotFound`，`104` → `ErrUnauthorized`，`200` 段 → `ErrForbidden`。
- `Config.String()` / `LogValue()` 对 token 与 URL 查询参数脱敏。
- 测试与 CI 移除协议生成物校验，新增 WebHook 签名、反向 WS、CQ 编解码、
  HTTP retcode 映射与端到端集成用例。

### 移除

- Milky 专用端点、实体、消息段与 `get_impl_info`。
- SSE 事件通道（OneBot v11 无此传输）。
- `internal/codegen`、`protocol/protocol-1.3.0.json`、`make generate` / `make spec-check`。
- 0.1.x 的 `service_*_gen.go` 与 `types/*_gen.go`。

### 迁移

见 README「从 0.1.x（Milky）迁移」。

## [0.1.0] - 2026-10-28

首个版本：对齐 Milky 1.3（`1.3.0`），覆盖 65 个端点、21 个事件、24 种消息段；
提供 HTTP API、WebSocket / SSE / WebHook 事件通道、事件分发、mock 与四个示例。

[0.2.0]: https://github.com/WinterChocolates/YukiChoco/releases/tag/v0.2.0
[0.1.0]: https://github.com/WinterChocolates/YukiChoco/releases/tag/v0.1.0
