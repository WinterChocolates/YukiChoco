# Changelog

本项目的所有重要变更都记录在此文件，格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.1.0] - 2026-10-28

首个可用版本：对齐 Milky 1.3（`1.3.0`），覆盖 65 个端点、21 个事件、24 种消息段。

### 新增

- **协议数据模型（`types`）**：由官方 Milky IR 代码生成，含字段级 `since` 与默认值；
  语义化 ID（`UserID`/`GroupID`/`Uin`/`MessageSeq`）；未知事件/消息段/枚举值原样透传。
- **HTTP API 层**：5 个 service（`System`/`Message`/`Friend`/`Group`/`File`）共 65 个强类型方法，
  统一信封解析、`retcode` 归类（`ErrBadRequest`/`ErrNotLoggedIn`/`ErrNotFound`）、
  幂等端点自动重试、令牌桶限流、`Call`/`CallRaw` 通用调用。
- **事件通道**：WebSocket（ping/pong + 读超时，无应用层心跳）、SSE（多行 `data`、注释保活）、
  WebHook（`http.Handler`，可选 `Authorization` 校验，常量时间比较）。
- **连接生命周期**：`Start`/`Shutdown`/`WaitReady`、状态机（`disconnected → dialing →
  connected → reconnecting → closed`）、指数退避 + 抖动重连、握手 401 时降级 `?access_token=`。
- **事件分发（`event`）**：并发 / 全局有序 / 按会话有序三种策略，`drop-newest` 与 `block` 两种背压，
  逐订阅者 panic 隔离，中间件链（`Recover`/`Timeout`/`Logging`），泛型 `On[T]`/`Once[T]`，
  谓词路由（`AtMe`/`Prefix`/`Admin`…）。
- **消息构造（`segment`）**：`Text`/`Mention`/`Reply`/`Image`/`Forward` 等构造器，
  `file://`/`base64://`/`http(s)://` 统一归一化，8 MiB 内联上限。
- **文件助手**：上传（路径/字节/`io.Reader`/URL）、按 `resource_id` 或 `file_id` 流式下载。
- **可测试性（`mock`）**：可编程假协议端（API 注册表 + 调用记录 + WS/SSE 推送），
  无需真实 QQ 账号即可跑集成测试。
- **示例**：`echo-bot`、`webhook-bot`、`sse-client`、`admin-bot`。

### 说明

- 事件语义：Milky 无 ack 与序号重放，事件通道断开期间的事件会丢失，SDK 不承诺重连补发。
- `send_*` 默认不重试（无幂等键），需要时用 `RetryConfig.RetryNonIdempotent` 显式开启。

[Unreleased]: https://github.com/WinterChocolates/YukiChoco/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/WinterChocolates/YukiChoco/releases/tag/v0.1.0
