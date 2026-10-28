# 基准测试

对齐设计文档 NFR-1 的验证记录。数据由下列命令在开发机采集，随硬件与 Go 版本波动，
**只用于观察趋势与回归**，不是绝对承诺。

```bash
make bench          # 等价于：
go test -run '^$' -bench . -benchmem -benchtime=500ms ./ ./event ./types
```

采集环境：`linux/amd64`、32 逻辑核、Go 1.25.9、`-count=1`。

## 事件分发（event）

| 基准 | ns/op | B/op | allocs/op | 说明 |
| --- | --- | --- | --- | --- |
| `DispatchSequential` | **112.6** | 8 | 1 | 单队列 + 1 worker，全局有序 |
| `DispatchPerPeer` | **177.3** | 23 | 2 | 8 分片，同一会话有序 |
| `DispatchConcurrent` | **231.8** | 8 | 1 | 单队列 + 8 worker |
| `DispatchDropWhenFull` | 1200 | 470 | 3 | 队列满丢弃路径（含 WARN 日志） |
| `DefaultKeyFunc` | 37.0 | 16 | 2 | 会话分片键计算 |

结论：投递路径本身约 **110–230 ns**，单队列顺序模式下吞吐 ≈ **8.9M evt/s**，
远高于 NFR-1 要求的 5,000 evt/s。并发模式单次更贵（锁竞争），但端到端延迟更低。

## 完整链路（解码 + 分发）

| 基准 | ns/op | 吞吐 | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `DecodeAndDispatch`（含嵌套 reply 段、群/成员实体） | **27,846** | 18.7 MB/s | 8,008 | 165 |

折算 ≈ **35,900 事件/秒**（单 goroutine 顺序分发，空 handler），满足 NFR-1。

## 事件解码（types）

| 基准 | ns/op | 吞吐 | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| `UnmarshalEvent` | **45,517** | 20.5 MB/s | 15,288 | 275 |
| `UnmarshalEventAndRead`（额外取 PlainText/Mentioned） | 45,510 | — | 15,352 | 276 |
| `UnmarshalIncomingSegments`（4 段，含嵌套 reply） | 14,431 | — | 7,064 | 141 |
| `MarshalOutgoingSegments`（4 段） | 2,145 | — | 778 | 20 |
| `IDSemanticTypes`（`"123456789"` → UserID） | 196 | — | 184 | 4 |

解码是当前最重的一环，主要开销来自：

1. `jsonx.TaggedPeek` 用 `map[string]json.RawMessage` 读取判别字段（每次分配一个 map）；
2. 事件信封与 `data` 分两次 `json.Unmarshal`；
3. 语义化 ID 的自定义 `UnmarshalJSON`（`[]byte` → `string` → `strconv.ParseInt`）。

优化方向（未实施，v0.1 不以性能为发布门槛）：用 `json.Decoder` 单次流式解码、
判别字段改用 `bytes.Index` 定位、ID 用无分配的数字扫描。基准可作为回归基线。

## HTTP API 调用（loopback）

| 基准 | ns/op | B/op | allocs/op | 说明 |
| --- | --- | --- | --- | --- |
| `APICallMeasure` | **176,597** | 11,732 | 138 | 强类型方法 `GetLoginInfo` |
| `CallEscapeHatch` | 185,153 | 12,406 | 142 | 泛化 `Call`（map 入参） |
| `SendGroupMessage` | 197,367 | 12,645 | 153 | 3 段消息 + 信封解析 |

约 **5,600 次调用/秒**（单 goroutine）。该数字主要由 loopback HTTP 往返与
`net/http` 的请求/响应分配决定，SDK 自身开销（信封解析、脱敏、日志判断）在
微秒级；NFR-1 的「框架开销 p99 < 200µs」在未计入网络时仍有大量余量。

## 覆盖率

```bash
make cover   # 排除 *_gen.go 与 examples/ 后的总行覆盖率
```

当前：**88.1%**（NFR-7 要求 ≥ 85%）。

## 如何避免性能回归

- 改动分发器、解码器或信封解析时跑一次 `make bench`，对比本文件的数字；
- 新增热路径代码时用 `-benchmem` 关注 allocs/op，避免无谓分配；
- CI 暂不跑基准（机器差异大），以本地/发布前记录为准。
