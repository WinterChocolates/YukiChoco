# YukiChoco · Milky Go SDK 设计需求文档

| 项 | 内容 |
| --- | --- |
| 文档版本 | v0.1（草案，待评审） |
| 目标协议版本 | **Milky 1.3**（`1.3.0`） |
| 目标 Go 版本 | 1.22+（使用泛型、`log/slog`、`slices`） |
| 模块形态 | 单模块，代码位于仓库根目录（已决议，不再使用子目录/`go.work`） |
| 模块名 | `github.com/WinterChocolates/YukiChoco`，根包名 `YukiChoco` |
| 参考实现 | `github.com/Szzrain/Milky-go-sdk`、`github.com/komeidisanxian/remilia`（`platform/milky`）、`LagrangeDev/Lagrange.Milky`（协议端） |
| 状态 | 需求已评审通过；M0（骨架 + 代码生成）已完成并全绿 |

---

## 0. 文档目的

本文定义 Milky 协议 Go SDK 的**功能性需求、非功能性需求、架构约束与验收标准**，作为后续编码、评审与测试的依据。

阅读顺序建议：第 1–3 章（背景/目标/基线）→ 第 5 章（功能需求）→ 第 7–8 章（架构与公开 API 草案）→ 第 11–12 章（里程碑与测试）。第 9 章记录关键取舍，第 13 章列出待决议问题。

---

## 1. 背景

### 1.1 Milky 协议现状

Milky 是基于 NTQQ 的 QQ 机器人应用接口标准，当前版本 **1.3.0**（2026-08-07 发布）。其通信模型非常简单：

- **API**：应用端 → 协议端，`POST /api/{endpoint}`，JSON body，统一响应信封 `{status, retcode, message, data}`。
- **事件**：协议端 → 应用端，三种通道：**WebSocket**（`GET /event` 升级）、**SSE**（`GET /event`，`text/event-stream`）、**WebHook**（协议端 POST 到应用端配置的地址）。
- **鉴权**：`Authorization: Bearer {access_token}`；事件通道还允许 `?access_token=` 查询参数。
- **无请求 ID、无 ack、无序号重放、无应用层心跳**。HTTP 请求/响应自身即请求-响应配对。

官方同时发布机器可读规范：

- **Milky IR**：npm `@saltify/milky-protocol` 的 `dist/protocol.json`（本仓库已锁定 1.3.0 副本，约 144 KB），含 65 个 API、16 个公共结构体、字段级 `since`/`defaultValue`/`isOptional`。
- **JSON Schema**：`https://milky.ntqqrev.org/raw/json-schema/schema.json`。
- 代码生成器 `milkygen`（MIT）可基于 IR 生成多语言类型定义。

### 1.2 现有 Go 实现的缺口

| 维度 | Milky-go-sdk | remilia `platform/milky` |
| --- | --- | --- |
| 定位 | 独立 SDK（DiscordGo 风格） | 框架内嵌 adapter，与 remilia 的 `Bot/Sender/Context` 强耦合，不可独立复用 |
| 协议版本 | 停留在 1.0/1.2（缺 `persist_group_file`、`set_avatar`、`delete_friend`、`markdown` 段、`group_disband` 等） | 覆盖较新，但类型散落于框架抽象层 |
| 事件通道 | 仅 WebSocket | WebSocket（+ 少量 HTTP 轮询式辅助） |
| 心跳 | 移植了 Discord 的 `op:1` 心跳帧（Milky 协议中不存在） | 无 |
| context | 无（`context.Context` 不可注入） | 有 |
| 错误 | 字符串 | `apiError`（较好） |
| 生成 | 手写 | 手写 |
| 未知事件/段 | 直接丢弃或报错 | 部分透传 |

**结论**：需要一个**独立、可生成、协议对齐、context-first、对未知内容宽容**的 Go SDK。

### 1.3 协议端参考（Lagrange.Milky）

`Lagrange.Milky` 基于 Lagrange.Core V2 实现 Milky，配置项含 `Milky.AccessToken`、`Milky.HttpServer.Host`、`Milky.WebSocket` 等。它是本 SDK 的**首选端到端联调对端**（官方 IR 的"标准实现"之一），另需兼容 Yogurt、LuckyLilliaBot。

---

## 2. 目标与非目标

### 2.1 目标（Goals）

- **G1 协议完整**：覆盖 Milky 1.3 全部 65 个 API、21 个事件、14 种接收段、10 种发送段。
- **G2 强类型且可演进**：类型由官方 IR 生成；未知事件/未知消息段/未知响应字段不导致失败，可透传。
- **G3 三种事件通道**：WebSocket（默认）、SSE（受限网络降级）、WebHook（服务端模式）。
- **G4 生产可用**：自动重连、有限并发、优雅关闭、结构化日志、可注入指标、`-race` 干净。
- **G5 context-first**：所有网络操作接受 `context.Context`，可取消、可超时、可全链路 trace。
- **G6 独立可复用**：不依赖任何 bot 框架；既可直接写 bot，也可作为 remilia 等框架的底层协议库。
- **G7 可验证**：单元测试 + 契约测试 + 针对真机协议端的 e2e（可选启用）+ 基准测试。

### 2.2 非目标（Non-Goals）

- **N1** 不实现 QQ 协议/登录（本 SDK 是应用端，不写协议端）。
- **N2** 不实现 OneBot v11/v12 兼容层（如需，另建 adapter 包）。
- **N3** 不实现插件系统、命令注册表、权限系统（属框架职责，见 remilia；SDK 只提供最薄的路由原语）。
- **N4** 不做全局单例/包级状态；多账号 = 多个 `*Client` 实例。
- **N5** 不承诺断线期间事件不丢失（协议无 ack/重放，见 FR-1.6）。

---

## 3. 协议基线、兼容性契约与需求来源

### 3.1 需求来源优先级（冲突时以此为准）

1. `@saltify/milky-protocol@1.3.0/dist/protocol.json`（**Milky IR，唯一真相源**，含 `since`/`defaultValue`）。
2. 官网 JSON Schema（`/raw/json-schema/schema.json`）。
3. 官网文档（通信、兼容性、API、结构体页面）。
4. `Lagrange.Milky` 实际行为（联调时以真机行为为准，若与 IR 冲突则开 issue 记录差异）。
5. 参考 Go 实现（仅借鉴工程结构，**不作为字段依据**——两者均落后于 1.3）。

> ⚠️ 注：`Szzrain/Milky` 仓库内的 `packages/types/src/constants.ts` 仍声明 `milkyVersion = '1.0'`，且 1.3 已移除 `@saltify/milky-types` 发布。**因此类型必须从 IR 生成，不能从该 TS 源码拷贝。**

### 3.2 官方兼容性契约（对 SDK 的硬约束）

官方规定：

- **请求结构体**：只允许「增加可选或有默认值的字段」「删除字段」「给现有字段加默认值」；不修改现有字段、不新增必填字段。
- **响应结构体**：只允许「增加字段」；不删除、不修改现有字段。
- 数值：`int32`/`int64` 无上界、下界为 0；QQ 号/群号用 `Uin` 语义。
- 数组：存在"忽略非法元素而不整体报错"的健壮数组语义（IR 中的 robust array）。

由此推导出 SDK 的**实现级约束**（写入编码规范，评审必查）：

| 约束 | 具体要求 |
| --- | --- |
| C1 | 响应/实体结构体的**所有字段都必须是可缺省的**；反序列化必须忽略未知字段（`encoding/json` 默认行为，禁止 `DisallowUnknownFields`）。 |
| C2 | 请求结构体中 `isOptional: true` 或带 `defaultValue` 的字段一律用**指针 + `omitempty`**，避免把 Go 零值当成显式值发出去（否则会把 `is_set=false`、`duration=0` 误发）。 |
| C3 | 数组字段使用 `internal/jsonx.RobustSlice[T]`：单元素反序列化失败时跳过该元素并记 WARN，不使整个事件失败。 |
| C4 | 联合类型（`Event`/`IncomingSegment`/`OutgoingSegment`/`GroupNotification`）必须提供 `Unknown` 分支，保留 `json.RawMessage`。 |
| C5 | 生成的枚举必须保留原始字符串（`type Sex string` + `Unknown` 容忍），不因未知值报错。 |
| C6 | 不得对 `since` 高于当前协议端版本的能力做硬依赖；调用前可用 `Capabilities()` 判断。 |

### 3.3 版本矩阵

| 协议端 `milky_version` | SDK 行为 |
| --- | --- |
| 1.0 – 1.1 | 全量基础能力可用；1.1 新增 API（`set_avatar` 等）调用会返回 `retcode` 错误，SDK 原样上抛 |
| 1.2 | 增加 `get_peer_pins`/`set_peer_pin`、`reaction_type` 等 |
| 1.3 | 增加 `persist_group_file`、`markdown` 段、`group_disband` 事件 |
| 未知（>1.3） | 正常工作；未知事件/段透传；`Capabilities().Unknown = true` 并打一次 INFO 日志 |

`Client` 在首次连接成功后调用 `get_impl_info` 记录协议端信息（`impl_name`/`impl_version`/`qq_protocol_type`/`milky_version`），供诊断与降级。

---

## 4. 用户与场景

| 编号 | 场景 | 关键诉求 |
| --- | --- | --- |
| S1 | 单群管理机器人（关键词回复、禁言、公告） | 上手快：10 行代码跑起来；消息构造 DSL 顺手 |
| S2 | 多账号 / 多协议端并行（10+ 实例） | 无全局状态；连接与 goroutine 可收敛；退出干净 |
| S3 | 协议端部署在内网另一台机器，应用端在公网且只开放 80/443 | 支持 WebHook 接入 + 鉴权；支持 HTTP 代理 |
| S4 | 自研框架把 SDK 当底层库 | 接口可 mock、可注入 Transport、可拿原始 JSON |
| S5 | 调试协议端兼容性 | 能 dump 原始请求/响应/事件；能查询 `get_impl_info` |

---

## 5. 功能需求

> 编号规则 `FR-<模块>.<序号>`；每条含**验收标准（AC）**，AC 必须可被自动化测试或人工 checklist 验证。

### FR-1 连接与生命周期

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-1.1 | 支持三种事件源：`ws`（默认）、`sse`、`webhook`，通过配置切换；允许 `none`（纯 API 模式，用于只发消息的场景） | 三种模式各有一个可运行 example；`none` 模式下 `Start` 不建立事件连接且不发任何请求 |
| FR-1.2 | WebSocket 事件源：连接 `ws(s)://host/event`，鉴权优先用 `Authorization` 头，失败时降级为 `?access_token=` 查询参数 | 契约测试用 `httptest` 模拟服务端校验两种方式 |
| FR-1.3 | SSE 事件源：解析 `event: milky_event` + 多行 `data:`，空行为消息结束边界；支持 `\n\n` 与 `\r\n\r\n` | 单元测试覆盖分片到达（跨 TCP 包）与多行 data |
| FR-1.4 | WebHook 事件源：提供 `http.Handler`（`client.EventHandler()`），用户自行挂载；支持可选鉴权校验（`Authorization` 头，1.1+）与 IP 白名单 | 用 `httptest.Server` 投递事件，断言 handler 分发生效、错误 token 返回 401 |
| FR-1.5 | 自动重连：指数退避 + 抖动（默认 1s→60s，jitter ±20%），可配置最大次数或无限；重连期间状态可观测、可被 `ctx` 取消 | 单测：模拟服务端连续拒绝 3 次后成功，断言重连次数与退避序列 |
| FR-1.6 | **存活检测**：WS 使用 ping/pong 帧 + 读超时（不用应用层心跳帧，Milky 无心跳 op）；SSE 使用读超时 + 服务器注释行 | 单测：服务端不响应 ping 时在 `IdleTimeout` 内触发重连；断言不会向协议端发送非事件 JSON |
| FR-1.7 | 状态机：`Disconnected → Dialing → Connected → Reconnecting → Closed`，提供 `State()`、`OnStateChange(func(State))`、`WaitReady(ctx)` | 状态迁移有单测表驱动；回调不持锁执行且可重入调用 `State()` |
| FR-1.8 | 优雅关闭 `Shutdown(ctx)`：停止接受新调用 → 关闭事件源 → 取消在途 API 请求 → 等待 handler 收尾（受 ctx 约束）；可重复调用幂等；关闭后调用 API 返回 `ErrClosed` | 单测：Shutdown 后所有 API 返回 `ErrClosed`，`Shutdown` 二次调用无 panic |
| FR-1.9 | **明确文档化事件丢失语义**：无 ack/重放，断线期间事件可能丢失；重连后不补发 | README 与 godoc 中必须有该说明；不得实现"重连补发"的伪保证 |
| FR-1.10 | 断线不影响 API：事件源断开时 API 调用仍可用（除非协议端整体不可达） | 单测：断开 WS 后 `SendGroupMessage` 仍成功 |

### FR-2 HTTP API 调用

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-2.1 | `POST {BaseURL}/api/{endpoint}`，`Content-Type: application/json`，无参 API 也必须发送 `{}` | 契约测试断言无参 API 请求体为 `{}` |
| FR-2.2 | 统一信封解析：`status=="ok" && retcode==0` 为成功；否则返回 `*APIError{Endpoint,Retcode,Message}` | 表驱动测试覆盖 `ok/-400/-403/-404` 四种响应 |
| FR-2.3 | HTTP 语义错误独立建模：`401`→`ErrUnauthorized`、`404`→`ErrEndpointNotFound`、`415`→`ErrUnsupportedMediaType`、其它非 200 → `ErrUnexpectedStatus` | 单测逐一断言 `errors.Is` |
| FR-2.4 | **context-first**：所有 API 方法首参为 `context.Context`；内部限制为 `min(ctx deadline, APITimeout)` | 单测：`ctx` 立即取消 → 返回 `context.Canceled` 且不发请求 |
| FR-2.5 | `status` 字段为 `""`（协议端未返回）但 `data` 可解析时，按失败处理并保留原始 body 到错误中 | 单测覆盖畸形响应，断言错误包含原始 body 截断 |
| FR-2.6 | 重试策略：默认**仅对幂等端点**（`get_*`、`set_*`、`mark_*`、`delete_*`、`accept_*`、`reject_*`、`quit_*`、`move_*`、`rename_*`）在「网络错误/5xx/读写超时」时重试，默认 2 次；`send_*`/`upload_*` 默认**不重试**，需 `WithRetryNonIdempotent(true)` 显式开启 | 单测：`send_group_message` 遇网络错误只发 1 次；`get_group_list` 失败重试 2 次 |
| FR-2.7 | 可选客户端限流：令牌桶，默认关闭；支持全局 QPS 与按端点覆盖 | 单测：配置 2 QPS 时 5 次调用耗时 ≥ 1.5s（允许误差） |
| FR-2.8 | 逃生舱：`Call(ctx, endpoint string, in, out any) error` 与 `CallRaw(ctx, endpoint string, body []byte) (json.RawMessage, error)`，允许调用未建模 API | 单测：用 `Call` 调用任意 endpoint 成功 |
| FR-2.9 | 65 个 API 全部提供强类型方法（生成），命名规则 `snake_case → PascalCase`，请求/响应结构体命名 `<Method>Request` / `<Method>Response` | 生成物与 IR 的 endpoint 集合做**集合相等断言**测试 |
| FR-2.10 | 便捷层（手写）：`SendGroupText`、`SendPrivateText`、`ReplyGroup`、`ReplyPrivate`、`SendGroupMessageAndWait`（返回 seq）、批量转发构造等 | 每个便捷方法有单测并断言最终 JSON 与手工构造一致 |
| FR-2.11 | HTTP 传输可替换：默认 `*http.Client`（连接复用、可注入 `http.Transport` 以支持代理/TLS）；支持 HTTP/2 | 单测：注入带 `Proxy` 的 `http.Transport` 可用（文档给出 WSL/NAT 下 `proxy=http://host:7897` 示例） |

### FR-3 类型与数据模型

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-3.1 | `types` 包 100% 由 IR 生成：16 个公共结构体、65 组 API 请求/响应、21 个事件、消息段联合 | `go generate` 后 `git diff --exit-code` 为空 |
| FR-3.2 | **语义化 ID 类型**：`type UserID int64`、`GroupID int64`、`MessageSeq int64`、`MessageID = MessageSeq`，禁止跨类型赋值（编译期防串号） | 编译期测试（`go build` 一个反例文件应失败，用 `//go:build ignore` 保留） |
| FR-3.3 | ID 反序列化**宽容**（接受 JSON number 与数字字符串），序列化**严格**（输出 number）；单测覆盖 `"12345"`、`12345`、`1.0`（拒绝） | 表驱动单测 |
| FR-3.4 | 可选字段以指针表达；`Unmarshal` 时对 IR 有 `defaultValue` 的字段填充默认值（如 `limit=20`、`parent_folder_id="/"`、`reaction_type="face"`） | 单测：缺省字段反序列化后等于 IR 默认值 |
| FR-3.5 | 枚举生成 `type XxxEnum string` + `String()` + `Valid()`，未知值保留并可读 | 单测：未知枚举值不报错 |
| FR-3.6 | 提供 nil-safe 访问器与派生属性（`msg.Scene()`、`msg.Peer()`、`msg.GroupID()`、`member.Role()`、`entity.DisplayName()`——群名片优先昵称） | 单测覆盖 nil 接收者不 panic |
| FR-3.7 | 所有导出符号有 godoc（含中文摘要 + 协议文档链接锚点） | `golangci-lint` 开启 `revive` 导出注释检查 |

### FR-4 消息段

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-4.1 | 接收段与发送段**分类型**（字段集不同：接收 `image` 有 `resource_id/temp_url/width/height`，发送 `image` 只有 `uri`），不可混用 | 编译期：`SendGroupMessage(ctx, gid, <接收段>)` 应编译失败 |
| FR-4.2 | 构造 DSL：`Text`、`Mention`、`MentionAll`、`Face`、`Reply`、`Image`、`Record`、`Video`、`Forward`、`LightApp`、`Markdown`（接收侧）等，均为纯函数返回接口值 | 每个构造器有单测并断言 JSON 形状与官方示例一致 |
| FR-4.3 | URI 抽象（`file://`、`http(s)://`、`base64://`）：<br>`segment.ImageFromFile("/p/a.png")`、`ImageFromBytes`、`ImageFromURL`、`ImageFromReader`（≤50MB 内存读，超过则拒绝并提示用 `file://`） | 单测：四种构造产出的 `uri` 字符串符合规范；超限返回 `ErrFileTooLarge` |
| FR-4.4 | 解析辅助：`Segments.PlainText()`（拼接 text）、`Mentioned(uid)`、`FirstReplySeq()`、`Filter[T]()`、`HasSegment(type)`、`String()`（调试用人类可读） | 单测：多段消息断言拼接结果与 `@` 识别 |
| FR-4.5 | 未知段：`UnknownSegment{Type, Raw}`，反序列化保留原始 JSON，序列化原样回填 | 单测：`{"type":"future_thing","data":{...}}` 往返一致 |
| FR-4.6 | 发送前 preflight（可选，默认开启轻校验）：空文本段、空消息数组、超 100 段、`mention_all`、`reply` 缺 `message_seq` | 单测：空消息返回 `ErrEmptyMessage` 且不发请求 |
| FR-4.7 | 转发消息构造：`Forward(ForwardedMessage{UserID, SenderName, Segments, Time?}...)`，支持嵌套（1.3 `time` 可选） | 单测：两层嵌套转发 JSON 结构正确 |
| FR-4.8 | 与 1.2/1.3 新增字段对齐：`face.is_large`、`mention.name`、`reply.{sender_id,sender_name,time,segments}`、`market_face.{emoji_package_id,emoji_id,key,summary}`、发送 `forward.{title,preview,summary,prompt}`、发送 `light_app.json_payload`、接收 `markdown.content` | 生成物字段清单与 IR 逐字段 diff 为空 |

### FR-5 事件分发

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-5.1 | 强类型订阅：`On[T any](h func(ctx, *T))`（泛型，自动匹配 `event_type`）；另提供 `OnEvent`（任意事件）、`OnRaw`（原始 JSON） | 单测：注册 3 个不同类型的 handler，断言各只收到自己的事件 |
| FR-5.2 | 取消订阅：返回 `func()` 用于注销；并支持 `Once` 变体 | 单测：注销后不再收到 |
| FR-5.3 | 分发模式：<br>a) `DispatchConcurrent`（默认，worker pool 大小 = CPU 数）<br>b) `DispatchSequential`（单 goroutine，全局有序）<br>c) `DispatchPerPeerSequential`（按 `peer_id`/`group_id` 哈希分片，同一会话内有序、跨会话并行） | 单测：PerPeer 模式下同一群 100 条消息顺序严格递增，跨群并行度 > 1 |
| FR-5.4 | panic 隔离：handler panic 必须被 recover，记录 ERROR + 堆栈，不影响其它 handler 与事件源 | 单测：panic handler 之后仍能收到事件 |
| FR-5.5 | 中间件链：`Use(mw)`，签名 `func(next Handler) Handler`；内置 `Recover`、`Timeout`、`Logger`、`Metrics`、`Dedup`（可选按 `message_seq` 去重）、`ACL`（群/用户白名单） | 单测：中间件执行顺序、短路行为 |
| FR-5.6 | 事件路由谓词（薄）：`event.Router{}.GroupMessage().AtMe(selfID).Prefix("/cmd").Handle(...)` | 单测：4 类谓词组合命中/不命中 |
| FR-5.7 | 背压：事件队列有界（默认 1024）；满时默认 `DropNewest` 并计数告警，可配置 `Block`（阻塞事件源，可能触发协议端断开） | 单测：容量 1 时投递 3 条，断言 drop 计数为 2 |
| FR-5.8 | 事件对象只读语义：handler 收到指针但文档说明不得修改（分段/切片共享底层数组）；提供 `Clone()` 给需要保存者 | godoc 说明 + 单测断言 `Clone` 深拷贝 |
| FR-5.9 | 生命周期钩子：`OnStart/OnStop/OnError`（连接错误）、`OnProtocolMismatch` | 单测覆盖 |
| FR-5.10 | 21 个事件全部强类型建模（含 1.2 `peer_pin_change`、1.3 `group_disband`），未知 `event_type` → `UnknownEvent` | 生成物集合断言 |

### FR-6 文件与资源

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-6.1 | `UploadGroupFile/UploadPrivateFile` 接受 `io.Reader`/`[]byte`/本地路径/URL，内部转成 `file_uri`（`base64://` 仅用于 ≤ 阈值的小文件，默认 8MB） | 单测：四种来源产出正确 `file_uri` |
| FR-6.2 | 大文件不整读进内存：超过阈值时要求调用方传 `file://` 路径或 `http(s)://` URL，否则返回 `ErrFileTooLarge` | 单测 |
| FR-6.3 | 资源下载：`GetResourceTempURL` + `DownloadResource(ctx, resourceID)` 返回 `io.ReadCloser`（流式），并暴露 `TempURL` 供上层自行下载 | 单测：httptest 返回 200 + body，断言流式读取内容 |
| FR-6.4 | 群文件全量操作：列表/移动/重命名/删除/建目录/重命名目录/删除目录/转存永久（1.3 `persist_group_file`） | 生成物覆盖 + 每方法 1 条契约测试 |

### FR-7 可观测性

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-7.1 | 结构化日志：默认 `log/slog`；字段约定 `endpoint`、`retcode`、`elapsed_ms`、`event_type`、`peer_id`、`seq`、`impl_version` | 单测：用 `slog.JSONHandler` 抓取并断言字段 |
| FR-7.2 | **脱敏**：`access_token` 不得出现在日志、错误信息、`%+v` 输出中（自定义 `String()`/`LogValuer` 返回 `Bearer ***`） | 单测：打印全部配置与错误，断言不含真实 token |
| FR-7.3 | 指标接口：`Metrics` 接口（`Counter`/`Histogram`/`Gauge`），内置 `noop` 与 `expvar` 实现；埋点：API 调用数/延迟/错误码、事件数/丢弃数、重连次数、队列深度 | 单测：调用后计数正确 |
| FR-7.4 | 调试开关：`WithDebugDump(w io.Writer)` 以 DEBUG 级别输出原始请求/响应/事件 JSON（默认关闭，仍脱敏） | 单测：断言 dump 内容为合法 JSON 行 |
| FR-7.5 | Trace 钩子：`WithTracer(Tracer)`，在 API 调用与事件处理两端提供 `Start/Fnish` 轻量接口（不引入 OpenTelemetry 依赖） | 单测：span 名与属性正确 |

### FR-8 代码生成与规范同步

| 编号 | 需求 | 验收标准 |
| --- | --- | --- |
| FR-8.1 | `internal/codegen` 读取 `protocol.json` 生成：`types/*_gen.go`、`api_*_gen.go`、`event_gen.go`、`segment_gen.go`、`endpoints_gen.go`、`capabilities_gen.go`（`since` 表） | `go generate ./...` 幂等 |
| FR-8.2 | 生成器支持的特性：`fieldType ∈ {scalar, ref, enum}`、`isArray`、`isOptional`、`defaultValue`、`since`、`unionType ∈ {plain, withData}`、`dataType: uin` | 生成器单测用 IR 的**真实**片段做 golden 测试 |
| FR-8.3 | IR 文件入库锁定（`protocol/protocol-1.3.0.json`），并记录来源 URL + sha256；CI 提供 `make spec-check` 拉取最新版本并 diff 报告 | CI job 存在且失败时输出字段级 diff |
| FR-8.4 | 生成物文件头 `// Code generated by internal/codegen. DO NOT EDIT.`；手写扩展一律放 `*_ext.go`，且不得修改生成文件 | `git diff` 检查脚本 + review 规则 |
| FR-8.5 | `make generate && make check` 一键完成：生成 → `gofmt` → `go vet` → 单元测试 → 生成物 cleanliness 校验 | Makefile 目标可运行 |

---

## 6. 非功能性需求

| 编号 | 类别 | 要求 | 验证方式 |
| --- | --- | --- | --- |
| NFR-1 | 性能 | 空 handler 下事件分发吞吐 ≥ 5,000 evt/s（单机 4C8G）；API 调用框架开销 p99 < 200µs（不含网络）；事件路径热路径尽量少分配（目标 ≤ 3 alloc/evt） | `go test -bench` + `-benchmem`，写入 `docs/bench.md` |
| NFR-2 | 并发安全 | 所有导出方法并发安全；`go test -race ./...` 全绿；无锁顺序反转死锁 | CI 强制 `-race` |
| NFR-3 | 资源有界 | goroutine 数、事件队列、连接池、上传缓冲均有上界且可配置；`Shutdown` 后 goroutine 归零（用 `goleak` 断言） | `go.uber.org/goleak` 在 TestMain 校验 |
| NFR-4 | 可移植 | Go 1.22+；linux/amd64、linux/arm64、windows/amd64、darwin/arm64 构建通过；**无 cgo** | CI 交叉编译矩阵 |
| NFR-5 | 依赖最小 | 生产依赖 ≤ 2 个（WebSocket：`github.com/coder/websocket` 或 `gorilla/websocket`）；SSE 自实现（约 150 行）；JSON 用标准库（保留可替换 codec 接口） | `go list -deps` 检查 |
| NFR-6 | 安全 | 默认校验 TLS；不提供 `InsecureSkipVerify` 一键开关（需显式自定义 `*tls.Config` 并打 WARN）；WebHook token 使用 `crypto/subtle` 常量时间比较；文档警示协议端应部署内网 + 强 token | 安全 checklist review |
| NFR-7 | 可测试性 | `Transport`、`Clock`、`Logger`、`Metrics` 均可注入；提供 `mock` 包（假协议端：可编排响应与事件脚本） | 覆盖率 ≥ 85%（`types` 生成物排除） |
| NFR-8 | 文档 | 每个导出符号有 godoc；README 含 5 分钟快速开始；`examples/` 至少：`echo-bot`、`webhook-bot`、`sse-client`、`admin-bot` | `go run ./examples/...` 可编译 |
| NFR-9 | 稳定性承诺 | v1.0 起遵循 semver；协议字段新增视为 PATCH（生成物扩展，不破坏源码兼容）；Go API 破坏性变更需 MAJOR + 迁移指南 | CHANGELOG 规范 |

---

## 7. 架构设计

### 7.1 分层

```
            ┌───────────────────────────────────────────────┐
 用户代码 → │ YukiChoco (root)  Client / Options / Errors    │
            │  ┌───────────┬────────────┬────────────────┐  │
            │  │ api_*_gen │ event 分发  │ segment DSL    │  │
            │  └───────────┴────────────┴────────────────┘  │
            └───────────────┬───────────────────────────────┘
                            │ 依赖单向向下
            ┌───────────────▼───────────────┐
            │ types       （生成，无逻辑）    │
            │  entities / events / segments │
            └───────────────▲───────────────┘
            ┌───────────────┴───────────────┐
            │ transport   （可插拔）          │
            │  http.go  ws.go  sse.go  webhook.go │
            └───────────────┬───────────────┘
            ┌───────────────▼───────────────┐
            │ internal/jsonx  宽松数值/robust/raw │
            └───────────────────────────────┘
```

依赖方向：`milky → types / transport / event`；`event → types`；`transport → internal`；`types → internal`。**禁止反向依赖**，`types` 不 import 根包（保证可单独引用）。

### 7.2 包结构

```
.
├── go.mod                          # module github.com/WinterChocolates/YukiChoco
├── doc.go                          # 包文档：package YukiChoco
├── Makefile                        # generate / check / spec-check
├── milky.go                        # Client、New、Start/WaitReady/Shutdown、状态机
├── options.go                      # Config + WithXxx
├── apicall.go                      # 统一 call()：信封解析、重试、限流、指标、脱敏日志
├── service.go                      # Services 聚合：System/Message/Friend/Group/File
├── service_system_gen.go           # 17 个系统 API 方法（生成）
├── service_message_gen.go          # 9（生成）
├── service_friend_gen.go           # 6（生成）
├── service_group_gen.go            # 21（生成）
├── service_file_gen.go             # 12（生成）
├── api_ext.go                      # 手写便捷方法（SendGroupText/ReplyGroup 等）
├── errors.go                       # APIError / 哨兵错误 / Retcode 分类
├── capabilities.go                 # 版本比较、since 能力表、ProtocolInfo
├── version.go                      # SDK 版本常量
├── protocol/
│   └── protocol-1.3.0.json         # 锁定的 Milky IR（sha256 见 §15）
├── internal/
│   ├── codegen/                    # IR → Go（ir.go/tables.go/gen.go/emit*.go）
│   └── jsonx/                      # 宽容数值、Tagged 编解码、非法元素上报
├── types/
│   ├── ids.go                      # 手写：UserID/GroupID/Uin/MessageSeq（宽容反序列化）
│   ├── doc.go                      # 手写：编码规则说明
│   ├── segment_ext.go              # 手写：PlainText/Mentioned/ReplySeq…
│   ├── message_ext.go              # 手写：GroupMessage()/DisplayName()…
│   ├── enum_gen.go                 # 生成：8 个字段枚举 + 5 个判别枚举
│   ├── entity_gen.go               # 生成：8 个简单结构体
│   ├── message_gen.go              # 生成：24 消息段 + IncomingMessage + GroupNotification
│   ├── event_gen.go                # 生成：EventType + EventMeta + Event 接口 + 21 事件
│   ├── api_model_gen.go            # 生成：65 组请求/响应
│   └── endpoint_gen.go             # 生成：端点常量 + APISince + EndpointService
├── client_event.go                 # Start/Shutdown/状态机/重连/原始钩子
├── state.go                        # State 枚举
├── event/
│   ├── dispatcher.go               # 队列、worker、3 种顺序策略、背压、统计
│   ├── typed.go                    # 泛型 On[T] / Once[T]
│   ├── middleware.go               # Recover / Timeout / Logging
│   └── router.go                   # Router / GroupRouter 谓词路由
├── transport/
│   ├── transport.go                # Source / Session 抽象
│   ├── ws.go                       # WebSocket 事件源（ping/pong + 读超时）
│   ├── sse.go                      # SSE 事件源（多行 data、注释保活）
│   └── webhook.go                  # WebHook http.Handler（可选 Authorization 校验）
├── segment/                        # 段构造助手（返回 types 接口）
│   ├── segment.go                  # Text/Mention/Reply/Image/Forward…
│   └── media.go                    # URI 归一化、base64、大小限制
├── mock/                           # 可编程假协议端（API + WS/SSE）
├── examples/                       # echo-bot / webhook-bot / sse-client / admin-bot
└── docs/                           # 设计与文档
```

### 7.3 关键接口（草案）

```go
// transport/transport.go
type Event struct {
    Type string          // event_type
    Raw  json.RawMessage // 原始 JSON（未知事件也保留）
}

type EventSource interface {
    // Run 阻塞直到 ctx 取消或不可恢复错误；可重连的错误通过回调上报。
    Run(ctx context.Context, sink func(Event)) error
    Name() string // "ws" | "sse" | "webhook"
}

type HTTPTransport interface {
    Do(ctx context.Context, endpoint string, body []byte) (status int, resp []byte, err error)
}

// event/handler.go
type Handler func(ctx context.Context, ev types.Event) error

type Middleware func(next Handler) Handler

// 泛型订阅（root 包再包一层糖）
func On[T types.EventPayload](d *Dispatcher, h func(context.Context, *T) error) (off func())
```

### 7.4 客户端状态机

```
        New()
          │
   ┌──────▼──────┐ Start()      ┌─────────┐ 握手成功  ┌───────────┐
   │ Disconnected├─────────────►│ Dialing ├──────────►│ Connected │
   └──────┬──────┘              └────┬────┘            └─────┬─────┘
          │                          │ 失败/超时              │ 断开/读超时
          │                          ▼                       ▼
          │                    ┌─────────────┐        ┌──────────────┐
          │                    │ Reconnecting│◄───────┤ Reconnecting │
          │                    └──────┬──────┘        └──────────────┘
          │ Shutdown()                │ ctx 取消 / 重试耗尽
          ▼                           ▼
      ┌───────┐                  ┌───────┐
      │ Closed│◄─────────────────┤ Closed│
      └───────┘                  └───────┘
```

### 7.5 错误模型

```go
// 协议层错误（HTTP 200 + status=failed）
type APIError struct {
    Endpoint string
    Retcode  int
    Message  string
    RawBody  []byte // 截断保留，便于排障
}

// 传输层/使用层哨兵
var (
    ErrClosed, ErrUnauthorized, ErrEndpointNotFound,
    ErrUnsupportedMediaType, ErrUnexpectedStatus,
    ErrEmptyMessage, ErrFileTooLarge, ErrNotConnected, ErrProtocolMismatch
)
```

`Retcode` 分类（便于 `errors.Is` 与告警分级）：

| retcode | 含义 | 归因 | SDK 处理 |
| --- | --- | --- | --- |
| 0 | 成功 | — | 正常返回 |
| -400 | 参数解析/校验失败 | 调用方 bug | 不重试，包含协议端原文 |
| -403 | 协议端未登录 | 环境问题 | 不重试，触发 `OnError` |
| -404 | 资源不存在（好友/群/消息） | 业务 | 不重试 |
| 其它负数 | 协议端自定义 | 未知 | 不重试，原样上抛 |

### 7.6 配置项（`Config`）

| 字段 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `BaseURL` | string | — | 必填，`http(s)://host:port`，不带尾斜杠 |
| `AccessToken` | string | "" | Bearer token；空表示不鉴权 |
| `EventSource` | enum | `ws` | `ws`/`sse`/`webhook`/`none` |
| `HTTPClient` | *http.Client | 内置 | 可注入代理、TLS、HTTP/2 |
| `APITimeout` | duration | 15s | 单次 API 超时 |
| `DialTimeout` | duration | 10s | 事件源拨号超时 |
| `IdleTimeout` | duration | 90s | 读空闲超时（触发重连） |
| `Retry` | struct | 2 次 / 幂等 | 见 FR-2.6 |
| `Reconnect` | struct | 1s→60s, ∞, jitter 20% | 退避参数 |
| `Dispatcher` | struct | workers=NumCPU, buf=1024, per-peer 有序 | FR-5.3 |
| `Backpressure` | enum | `DropNewest` | `DropNewest`/`Block` |
| `Logger` | *slog.Logger | discard | 结构化日志 |
| `Metrics` | Metrics | noop | 指标 |
| `Tracer` | Tracer | noop | trace 钩子 |
| `UserAgent` | string | `YukiChoco-milky/<ver> (Milky 1.3)` | |
| `MaxUploadMemory` | int64 | 8 MiB | 超过则要求 `file://` |
| `Preflight` | bool | true | 发送前轻校验 |
| `DebugDump` | io.Writer | nil | 原始 JSON dump |

---

## 8. 公开 API 草案

### 8.1 最小可用

> 下面的 `Start` / `OnMessageReceive` 属于 M2 的事件层；M1 已可用的 HTTP 部分见 README「调用 API」。

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/WinterChocolates/YukiChoco"
    "github.com/WinterChocolates/YukiChoco/types"
)

func main() {
    c, err := YukiChoco.New(YukiChoco.Config{
        BaseURL:     "http://127.0.0.1:6700",
        AccessToken: os.Getenv("MILKY_TOKEN"),
        EventSource: YukiChoco.EventSourceWS,
    })
    if err != nil { log.Fatal(err) }

    self := c.SelfID()
    c.OnMessageReceive(func(ctx context.Context, ev *types.MessageReceiveEvent) error {
        msg, ok := ev.GroupMessage()
        if !ok || !msg.Segments.Mentioned(self) { return nil }
        res, err := c.Group().SendMessage(ctx, msg.Group.GroupID,
            &types.OutgoingReplySegment{MessageSeq: msg.MessageSeq},
            &types.OutgoingTextSegment{Text: "你好，"},
            &types.OutgoingMentionSegment{UserID: msg.SenderID},
        )
        if err != nil { return err }
        log.Printf("sent seq=%d", res.MessageSeq)
        return nil
    })

    ctx := context.Background()
    if err := c.Start(ctx); err != nil { log.Fatal(err) }
    defer c.Shutdown(ctx)
    <-ctx.Done()
}
```

### 8.2 可测试性

```go
// 依赖注入：面向接口而非具体 Client
type Sender interface {
    SendGroupMessage(ctx context.Context, group types.GroupID, segs ...types.OutgoingSegment) (*types.SendMessageResponse, error)
}
// mock 包提供可编排实现
```

### 8.3 WebHook 模式

```go
c, _ := YukiChoco.New(YukiChoco.Config{
    BaseURL:     "http://10.0.0.2:6700",
    AccessToken: token,
    EventSource: YukiChoco.EventSourceWebhook,
    WebhookAuth: true, // 校验 Authorization（Milky 1.1+）
})
mux.Handle("/milky/event", c.EventHandler())
```

### 8.4 逃生舱

```go
var out struct{ Friends []types.FriendEntity `json:"friends"` }
err := c.Call(ctx, "get_friend_list", map[string]any{}, &out)
```

---

## 9. 关键设计决策（ADR 摘要）

| # | 决策 | 理由 | 放弃的方案 |
| --- | --- | --- | --- |
| D1 | context-first | 可取消/超时/链路追踪是生产刚需；参考实现缺失 | 无 ctx（DiscordGo 风格） |
| D2 | 类型从 **Milky IR 生成** | 65 API × 字段 × `since` 手写不可维护；1.3 已移除 TS types 包 | 手写结构体；从 TS 源码翻译 |
| D3 | 保留 `Unknown` 分支 + robust slice | 官方兼容性契约允许新增字段/类型；SDK 不能因协议端升级而崩 | 严格校验并报错 |
| D4 | **不发送应用层心跳** | Milky 无 op 帧；WS 自带 ping/pong | 照搬 Discord 心跳（Milky-go-sdk 的问题） |
| D5 | `send_*` 默认不重试 | 无幂等键，重试会重复发消息 | 全端点自动重试 |
| D6 | 语义化 ID 类型 | 防 `group_id`/`user_id`/`seq` 串用 | 统一 int64 |
| D7 | API 方法挂在 **service 子对象**上（`c.Group().SendMessage()`），另保留泛型 `Call` 出口 | 群聊分类下可省掉 `group` 前缀（`c.Group().SendMessage` 而非 `SendGroupMessage`），消息分类下保留全名以免歧义；service 为 interface，便于 mock | 全部平铺在 `*Client`（DiscordGo 风格，65 个方法噪声大） |
| D8 | 三种事件通道全覆盖 | WebHook 是企业网络常见部署；SSE 是 WS 被禁时的降级 | 仅 WS（Milky-go-sdk） |
| D9 | 支持 `per-peer` 有序分发 | bot 逻辑天然按会话串行，避免用户自己加锁 | 只提供全并发 |
| D10 | 根包 re-export 常用类型别名 | 用户多数只 import `milky` 一个包 | 强制两层 import |

---

## 10. 与参考实现的对比

| 能力 | Milky-go-sdk | remilia/milky | 本 SDK |
| --- | --- | --- | --- |
| 协议版本 | 1.0/1.2（缺 1.3） | ~1.2+ | **1.3（IR 生成）** |
| 事件通道 | WS | WS | **WS / SSE / WebHook / none** |
| 心跳处理 | Discord `op:1`（协议不存在） | 无 | ping/pong + 读超时 |
| context | ✗ | ✓ | ✓ |
| 重连 | 有 | 有（指数退避） | ✓ + 状态机 + 指标 |
| 错误模型 | 字符串 | `apiError` | `APIError` + 哨兵 + `errors.Is` |
| 未知事件/段 | 丢弃 | 部分 | **透传 + Unknown 类型** |
| 消息段构造 | 手写 `MarshalJSON` | 框架内 | 生成 + 薄 DSL |
| 并发分发 | 无策略 | worker pool | 并发 / 全局有序 / 会话有序 + 背压 |
| 中间件 | ✗ | ✗ | ✓ |
| 代码生成 | ✗ | ✗ | ✓（IR → Go，幂等 + CI 校验） |
| 独立可复用 | ✓ | ✗（框架耦合） | ✓（`types` 可单独引用） |

---

## 11. 里程碑与验收

| 里程碑 | 内容 | 交付物 | 预估 |
| --- | --- | --- | --- |
| **M0 骨架 + 代码生成** | 仓库脚手架、CI（lint/vet/race）、IR 锁定、`internal/codegen` 跑通 `types` 全量生成、golden 测试 | 可生成 16 结构体 + 65 API 模型 + 21 事件 + 24 段 | 2 d |
| **M1 API 层** | `call()` 信封解析、错误模型、重试/限流、65 个强类型方法、便捷方法、httptest 契约测试 | `api_*_gen.go` + `api_call.go` + 覆盖 ≥ 85% | 3 d |
| **M2 事件层** | WS 事件源、状态机、重连、dispatcher（3 种顺序策略）、中间件、泛型 `On[T]`、goleak | `event/`、`transport/ws.go` | 3 d |
| **M3 补全** | SSE、WebHook、segment DSL、router、文件上传下载、mock 协议端、4 个 example | `transport/sse.go`、`transport/webhook.go`、`segment/`、`mock/` | 2.5 d |
| **M4 加固与发布** | Lagrange.Milky 真机 e2e、benchmark、文档/CHANGELOG/迁移指南、v0.1.0 tag | `docs/bench.md`、`examples/`、release | 2.5 d |

**整体 DoD（v0.1.0）**：
1. `go test -race ./...` 全绿；覆盖率 ≥ 85%（排除生成物）。
2. 65 API 与 21 事件的集合与 IR 完全一致（自动化断言）。
3. 对 `Lagrange.Milky` 完成 e2e：登录信息、发群/私聊消息（文本+图片+回复+@）、撤回、群成员列表、群公告、上传群文件、接收 5 类事件。
4. 三种事件通道各有一个可运行 example。
5. `golangci-lint` 无告警；4 平台交叉编译通过。
6. `goleak` 无泄漏；benchmark 达标（NFR-1）。

---

## 12. 测试策略

| 层级 | 手段 | 范围 |
| --- | --- | --- |
| 单元 | 表驱动 + `httptest` | 信封解析、错误映射、重试、限流、SSE 分片解析、段构造/解析、robust slice、ID 宽容解析 |
| 契约 | 以 IR 为期望：字段名/类型/可选性/默认值逐一断言 | 生成物正确性（防生成器回归） |
| 集成 | `mock` 包可编程假协议端（脚本化响应与事件流） | 连接生命周期、重连、背压、分发顺序、Shutdown |
| 并发 | `-race` + 随机事件流压力 | 各导出方法并发调用 |
| 模糊 | `go test -fuzz` 对 `Event`/`IncomingSegment`/SSE 解析 | 不许 panic，未知内容不报错 |
| 基准 | `-bench -benchmem`，基准纳入 CI 趋势记录 | 事件分发、API 编解码 |
| E2E | env `MILKY_E2E_BASE_URL` + docker `Lagrange.Milky`，默认 skip | 真实协议端全链路 |
| 兼容 | 起多版本协议端（1.0/1.2/1.3 若可得）跑同一套 e2e | 版本矩阵 |

---

## 13. 风险与待决议问题

### 13.1 风险

| # | 风险 | 影响 | 缓解 |
| --- | --- | --- | --- |
| R1 | 各协议端实现与 IR 有偏差（Lagrange / Yogurt / LuckyLilliaBot） | 字段缺失或类型不一致导致反序列化失败 | robust 反序列化 + `Unknown` 透传 + `docs/compatibility.md` 记录差异 |
| R2 | 官方协议仍在快速演进（1 年 3 个 minor） | 类型过期 | 生成器 + `make spec-check` + 语义化版本策略 |
| R3 | WebHook 无签名机制（仅 1.1+ 可选 Bearer） | 伪造事件 | 文档强制建议 IP 白名单 + 强 token + 反代层校验 |
| R4 | 事件无序、可能丢失 | 业务状态不一致 | 明确语义 + 提供 `message_seq` 去重中间件 + 建议用 `get_history_messages` 补偿 |
| R5 | 无幂等键，重试可能导致重复消息 | 用户困扰 | 默认不重试 `send_*`；文档说明 |
| R6 | 协议端可能把 `int64` 序列化为字符串（非 IR 行为） | 反序列化失败 | FR-3.3 宽容解析；CI 用真实抓包做 fixture |
| R7 | `@saltify/milky-types` 已移除，社区 TS 定义可能滞后 | 误导 | 只信 IR；文档标注来源 |

### 13.2 已决议（2026-10-28 评审通过）

| # | 议题 | 决议 | 落地影响 |
| --- | --- | --- | --- |
| 1 | 仓库形态 | **单模块，代码直接在仓库根**（不使用 `milky/` 子目录，不使用 `go.work`） | 模块名 `github.com/WinterChocolates/YukiChoco`，根包名 `YukiChoco`（导入路径与包名一致） |
| 2 | Go 版本下限 | **1.22** | 可用泛型、`slices`、`log/slog`；CI 矩阵从 1.22 起 |
| 3 | WebSocket 库 | **`gorilla/websocket`** | 与协议端参考实现一致、生态成熟；`transport/ws.go` 用 `websocket.Dialer`（可注入 `Proxy`/`NetDialContext`），只发 ping 帧、不发应用层心跳 |
| 4 | API 方法形态 | **service 子对象**：`c.System()/Message()/Friend()/Group()/File()` | 每个 service 生成为 interface + 实现。`send_group_message` 归入 `Group()` 并省略前缀（`Group().SendMessage`，变参消息段），`send_private_message` 在 `Message()`（`Message().SendPrivateMessage`）；其余方法名与 REST 端点一一对应 |
| 5 | 代码生成 | **自研 `internal/codegen`** | 已实现：IR → `types/*_gen.go`；命名表 `tables.go` 强制登记（新增端点/枚举未登记即生成失败）；golden 测试锁定生成物 |
| 6 | remilia 适配层 | **暂不做** | SDK 保持框架无关；`types` 可单独引用，未来如需适配不必改 SDK |
| 7 | E2E 入 CI | **暂不做** | e2e 保留为本地/发布前 checklist（`MILKY_E2E_BASE_URL` 开启），CI 只跑单元/契约/race |

### 13.3 原待决议记录（已关闭）

1. **模块路径与仓库形态**：`github.com/YukiChoco/milky`（独立仓库）还是本仓库内 `milky/` 子目录（多模块 `go.work`）？
2. **Go 版本下限**：1.22（泛型 + `slices` + `slog`）是否可接受，还是需兼容 1.21？
3. **WebSocket 库选型**：`coder/websocket`（简洁、context 友好、无 cgo）vs `gorilla/websocket`（生态广、参考实现已在用）。
4. **API 方法形态**：直接挂在 `*Client`（本文推荐）还是通过 service 子对象（`c.Group().SendMessage()`）？
5. **是否需要 `milkygen` 官方生成器**：先用自研 `internal/codegen`（可完全掌控 Go 习惯），还是复用官方 CLI 并贡献 Go target？
6. **是否提供 remilia 适配层**（把本 SDK 包成 `platform/milky` 的替代实现），还是保持两者并行？
7. **E2E 是否纳入 CI**：需要 docker + Lagrange.Milky 镜像；若资源不允许，改为发布前手动 checklist。

---

## 15. 实现进度

### M0（已完成）

| 项 | 状态 |
| --- | --- |
| 仓库骨架（根 `go.mod`，无 `go.work`） | ✅ |
| 锁定 IR：`protocol/protocol-1.3.0.json`（sha256 `2df17982077b07c20220f7cdac18d80f424ce1efd35fd5f75d71f607582c6836`） | ✅ |
| `internal/jsonx`（Tagged 编解码、宽容整数、非法元素上报） | ✅ |
| `internal/codegen`（IR → types，含命名/值域/引用校验与去重） | ✅ |
| `types` 全量生成：16 结构体 / 21 事件 / 24 消息段 / 65 API 模型 / 65 端点常量 | ✅ |
| 契约测试：golden 生成物、协议基线（65/21/16）、未登记端点与未命名枚举必须报错 | ✅ |
| 行为测试：事件解码、段往返、未知事件/段透传、非法元素跳过、ID 宽容、omitempty 语义 | ✅ |
| `go build` / `go vet` / `go test -race` / `gofmt` 全绿 | ✅ |

已确认的生成物语义（M1 直接复用）：

- 消息段：`IncomingXxxSegment` / `OutgoingXxxSegment` 分离，`Segments` 列表容错解码，未知段 `UnknownIncomingSegment{Raw}` 原样回填。
- 事件：`Event` 接口 + `UnmarshalEvent`；每事件形如 `MessageReceiveEvent{EventMeta; Data IncomingMessage}`；未知事件 `UnknownEvent{Raw}` 原样回填。
- 请求模型：可选字段指针 + `omitempty`，带默认值字段值类型 + `omitempty`（`parent_folder_id`、`limit`、`duration`、`is_mute` 等零值不发送）。

### M1（已完成）

| 项 | 状态 |
| --- | --- |
| `config.go`：`Config` / `RetryConfig` / `RateLimitConfig`，默认值填充、BaseURL 校验与尾斜杠规范化、`String()`/`LogValue()` 脱敏 | ✅ |
| `errors.go`：9 个哨兵错误 + `APIError{Endpoint,Retcode,Message,Snippet}`，按 retcode `-400/-403/-404` 归类，`errors.Is/As` 可用 | ✅ |
| `apicall.go`：统一信封解析、HTTP 401/404/415/5xx 映射、`{}` 空请求体、超时分层、指数退避 + 抖动、`Call` / `CallRaw` 逃生舱 | ✅ |
| 幂等重试：仅 `get_/set_/mark_/delete_/move_/rename_/accept_/reject_/quit_/persist_` 前缀重试；`send_*` 默认不重试（可用 `RetryNonIdempotent` 显式开启） | ✅ |
| 限流：零依赖令牌桶（惰性补充、无后台 goroutine），`RateLimit.QPS<=0` 即关闭 | ✅ |
| 脱敏：日志、错误信息、`Config.String()`、`DebugDump` 均不出现 access_token（测试断言 `***` 出现、原值不出现） | ✅ |
| `client.go`：`New` / `Init`（缓存 `SelfID` 与 `ProtocolInfo` 并校验协议版本）/ `Close`（幂等、`ErrClosed`）/ 5 个 service 访问器 | ✅ |
| service 生成：5 个 interface + 实现共 65 个方法，`send_group_message` → `Group().SendMessage(ctx, groupID, segments...)`（变参）、`send_private_message` → `Message().SendPrivateMessage(...)` | ✅ |
| `api_ext.go`：`SendGroupText` / `SendPrivateText` / `ReplyGroup` / `ReplyPrivate` / `SendGroupSegments` / `SendPrivateSegments` | ✅ |
| 测试：32 个用例（请求形状、信封/retcode、HTTP 状态、幂等重试边界、ctx 取消、限流、脱敏、`Init` 缓存、关闭幂等、配置校验、退避边界）+ AST 方法覆盖度对齐 IR | ✅ |

### M2（已完成）

| 项 | 状态 |
| --- | --- |
| `transport`：`Source`/`Session` 抽象（一次连接一个 Session，重连策略上移），WS 实现用 `gorilla/websocket` | ✅ |
| WS 存活检测：ping/pong + 每次读前续期 `IdleTimeout` 读超时；**不发送应用层心跳帧**（Milky 无此概念） | ✅ |
| 鉴权：优先 `Authorization: Bearer`，握手 401 时自动降级 `?access_token=` 重试一次（对应 FR-1.2） | ✅ |
| 连接状态机：`Disconnected → Dialing → Connected → Reconnecting → Closed`，`State()` / `OnStateChange` / `WaitReady` | ✅ |
| 自动重连：指数退避 + 抖动（默认 1s→60s、±20%）、`MaxAttempts`/`Disable` 可配、`ctx` 可中断 | ✅ |
| 事件分发：`concurrent`（单队列 + N worker）/ `sequential`（全局有序）/ `per-peer`（N 队列 + 每队列 1 worker，同会话有序） | ✅ |
| 背压：`drop-newest`（默认，计数 + WARN）/ `block`；`Close` 先等投递结束再关队列，杜绝 send on closed channel | ✅ |
| panic 隔离：每个订阅者单独 recover（不影响同事件其它订阅者），Dispatcher 最外层再兜一层（覆盖中间件） | ✅ |
| 中间件链：`Use(...)`，内置 `Recover` / `Timeout` / `Logging`；`HandlerTimeout` 支持每事件超时 | ✅ |
| 泛型订阅 `On[T]` / `Once[T]`（事件类型由零值推导）；`OnEvent`、`OnRaw` 原始 JSON 钩子 | ✅ |
| 生命周期：`Start`（不阻塞）/ `Shutdown(ctx)`（可重复、超时可控、重试会继续等待排空）/ `ErrAlreadyStarted` / `ErrClosed` | ✅ |
| 容错：单条事件解码失败只跳过该条，不中断连接；`EventSourceNone` 纯 API 模式 | ✅ |
| 测试：分发器 13 个用例（3 种顺序、2 种背压、panic、中间件顺序/短路、注销、Once、超时、排空）、transport 8 个（事件读取、401 降级、空闲检测、Close 解阻塞、ctx 取消、连接失败）、集成 10 个（typed 投递、Raw、重连、坏 JSON、状态序列、Shutdown 超时重试、goroutine 回收） | ✅ |

实现期间修正的两个真实缺陷（已由测试锁定）：

1. `concurrent` 模式最初为每个 worker 建独立队列，但投递只走 `queues[0]`，导致实际完全串行 → 改为单队列 + N worker。
2. `invokeSubscribers` 中一个订阅者 panic 会跳过同事件的其它订阅者 → 改为逐订阅者 recover。
3. 状态回调最初每个通知起一个 goroutine，导致回调到达顺序与迁移顺序不一致（测试出现 `dialing → disconnected → connected`）→ 改为单一消费 goroutine 串行派发。

### M3（已完成）

| 项 | 状态 |
| --- | --- |
| `transport/sse.go`：SSE 事件源（`event:`/多行 `data:`/注释保活/`\r\n`），401 降级 query token，读空闲超时靠"读取协程 + Next 侧计时器"实现 | ✅ |
| `transport/webhook.go`：`Source` + `http.Handler`；`Authorization` 常量时间比较（`WebHookVerifyToken` 默认关）、405/415/429/503 语义、队列满计数 | ✅ |
| `Client.EventHandler()` / `WebhookDropped()`；`Config` 新增 `EventSourceSSE` / `EventSourceWebhook` / `WebHookVerifyToken` / `WebHookBuffer`；EventURL 按事件源分别推导（ws / http(s) / 空） | ✅ |
| `segment` 包：`Text`/`Mention`/`MentionAll`/`Face`/`LargeFace`/`Reply`/`Image`/`Sticker`/`Record`/`Video`/`LightApp`/`Forward`/`ForwardWithOptions` | ✅ |
| `segment.URI`：本地路径 → `file://` 绝对路径；`[]byte`/`io.Reader` → `base64://`；`file://`/`http(s)://`/`base64://` 原样透传；8 MiB 内联上限（`ErrFileTooLarge`，超限不静默截断） | ✅ |
| 上传/下载助手：`UploadGroupFileFrom/With`、`UploadPrivateFileFrom/With`、`DownloadResource`（临时链接 → 流式 body）、`DownloadFile` | ✅ |
| `event.Router` / `event.GroupRouter`：`Type` / `Where` / `Group` / `AtMe` / `MentionsAll` / `Prefix` / `Admin` 谓词，谓词不可变（返回新 Router），支持 `Handle` / `HandleEvent` | ✅ |
| `mock` 包：可编程假协议端（API 注册表 + 调用记录 + WS/SSE 推送 + `?access_token=`/`Authorization` 校验 + `WaitConnected`），未注册端点默认返回空成功 | ✅ |
| `examples/`：`echo-bot`、`webhook-bot`、`sse-client`、`admin-bot`（含谓词路由与群管理操作） | ✅ |
| 测试：segment 5 组（构造器 JSON、file/base64/reader、上限、Forward 元信息）、transport SSE 7 个 + WebHook 6 个、router 4 个、mock 端到端 5 个（WS/SSE/WebHook/上传下载/未启用 501） | ✅ |

### M4（已完成，v0.1.0）

| 项 | 状态 |
| --- | --- |
| 真机 e2e：`e2e_test.go`，`MILKY_E2E_BASE_URL` 未设置时 `t.Skip`；覆盖 `Init` / 好友与群列表 / WS 就绪 / 发消息 → 事件回显 → 撤回 / 分发统计 | ✅ |
| 基准：`event/bench_test.go`、`types/bench_test.go`、`bench_test.go`（分发三模式、队列满丢弃、完整解码+分发链路、事件解码、段编解码、ID 解析、API 调用）；结果记录在 `docs/bench.md` | ✅ |
| 关键指标：分发 112–232 ns/op（顺序 ≈ 8.9M evt/s）、解码+分发 27.8 µs/evt（≈ 35.9k evt/s，NFR-1 要求 5k）、API 调用 ≈ 176 µs/次（loopback） | ✅ |
| 覆盖率：`make cover` 排除 `*_gen.go` 与 `examples/` 后 **86.2%**（NFR-7 要求 ≥ 85%） | ✅ |
| CI：GitHub Actions —— gofmt / vet / test / `-race`，Go 1.22+1.23 × Linux+Windows 矩阵；独立 job 校验「重新生成后无 diff」与 golden 测试；独立 job 跑覆盖率 | ✅ |
| 发布准备：`LICENSE`（MIT）、`CHANGELOG.md`（0.1.0）、`version.go` = `0.1.0`、README 收尾（示例、e2e、bench、许可证） | ✅ |

**DoD 对照（v0.1.0）**

| 设计文档 DoD | 结果 |
| --- | --- |
| `go test -race ./...` 全绿、覆盖率 ≥ 85%（排除生成物） | ✅ 86.2% |
| 65 API / 21 事件与 IR 集合完全一致（自动化断言） | ✅ `TestIRBaseline` + `TestServiceCoverageMatchesIR` + `TestGolden` |
| 三种事件通道各有可运行示例 | ✅ WS/SSE/WebHook 端到端测试 + 4 个 example |
| 4 平台交叉编译、`golangci-lint` 无告警 | ⚠️ 交叉编译由 CI 矩阵覆盖（Linux/Windows）；golangci-lint 未接入（以 `go vet` + `gofmt` 代替），留待后续 |
| 真机 e2e | ⏳ 测试已就绪，需提供 `MILKY_E2E_BASE_URL` 的协议端实例执行 |

---

## 附录 A · API 全量清单（65）

| 分类 | 端点 |
| --- | --- |
| 系统（17） | `get_login_info` `get_impl_info` `get_user_profile` `get_friend_list` `get_friend_info` `get_group_list` `get_group_info` `get_group_member_list` `get_group_member_info` `get_peer_pins` `set_peer_pin` `set_avatar` `set_nickname` `set_bio` `get_custom_face_url_list` `get_cookies` `get_csrf_token` |
| 消息（9） | `send_private_message` `send_group_message` `recall_private_message` `recall_group_message` `get_message` `get_history_messages` `get_resource_temp_url` `get_forwarded_messages` `mark_message_as_read` |
| 好友（6） | `send_friend_nudge` `send_profile_like` `delete_friend` `get_friend_requests` `accept_friend_request` `reject_friend_request` |
| 群聊（21） | `set_group_name` `set_group_avatar` `set_group_member_card` `set_group_member_special_title` `set_group_member_admin` `set_group_member_mute` `set_group_whole_mute` `kick_group_member` `get_group_announcements` `send_group_announcement` `delete_group_announcement` `get_group_essence_messages` `set_group_essence_message` `quit_group` `send_group_message_reaction` `send_group_nudge` `get_group_notifications` `accept_group_request` `reject_group_request` `accept_group_invitation` `reject_group_invitation` |
| 文件（12） | `upload_private_file` `upload_group_file` `get_private_file_download_url` `get_group_file_download_url` `get_group_files` `move_group_file` `rename_group_file` `delete_group_file` `persist_group_file` `create_group_folder` `rename_group_folder` `delete_group_folder` |

> 注意：端点为 `get_group_announcements`（非旧实现的 `get_group_announcement_list`）；1.3 起 `get_private_file_download_url` 增加 `is_self_send`。

## 附录 B · 事件清单（21）

| since | event_type |
| --- | --- |
| 1.0 | `bot_offline` `message_receive` `message_recall` `friend_request` `group_join_request` `group_invited_join_request` `group_invitation` `friend_nudge` `friend_file_upload` `group_admin_change` `group_essence_message_change` `group_member_increase` `group_member_decrease` `group_name_change` `group_message_reaction` `group_mute` `group_whole_mute` `group_nudge` `group_file_upload` |
| 1.2 | `peer_pin_change` |
| 1.3 | `group_disband` |

## 附录 C · 消息段清单

| 方向 | 段类型 |
| --- | --- |
| 接收（14） | `text` `mention` `mention_all` `face` `reply` `image` `record` `video` `file` `forward` `market_face` `light_app` `xml` `markdown` |
| 发送（10） | `text` `mention` `mention_all` `face` `reply` `image` `record` `video` `forward` `light_app` |

> 接收侧特有 `file`/`market_face`/`xml`/`markdown`（不可发送）；发送 `image` 用 `uri`，接收 `image` 用 `resource_id`+`temp_url`。

## 附录 D · 参考资料

| 资源 | 地址 |
| --- | --- |
| Milky 文档 | https://milky.ntqqrev.org/ |
| 通信规范 | https://milky.ntqqrev.org/guide/communication |
| 兼容性规范 | https://milky.ntqqrev.org/guide/compatibility |
| 更新日志 | https://milky.ntqqrev.org/changelog |
| JSON Schema | https://milky.ntqqrev.org/raw/json-schema/schema.json |
| Milky IR | npm `@saltify/milky-protocol@1.3.0/dist/protocol.json` |
| 官方 TS SDK | `Szzrain/Milky` → `packages/node-sdk`、`packages/types`（已过时，仅参考） |
| Go 参考实现 1 | https://github.com/Szzrain/Milky-go-sdk |
| Go 参考实现 2 | https://github.com/komeidisanxian/remilia |
| 协议端 | https://github.com/LagrangeDev/Lagrange.Core （`Lagrange.Milky`） |
| 协议端文档 | https://lagrangedev.github.io/Lagrange.Milky.Document/ |
