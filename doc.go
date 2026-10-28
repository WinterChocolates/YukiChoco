// Package YukiChoco 是 Milky 协议（1.3）的 Go SDK。
//
// # 定位
//
// 本包是 Milky 协议的**应用端**实现，负责与协议端（如 Lagrange.Milky、Yogurt、
// LuckyLilliaBot）通信：通过 HTTP 调用 /api/{endpoint}，通过 WebSocket / SSE /
// WebHook 接收事件。它不实现 QQ 协议本身、不依赖任何 bot 框架，可作为其它框架的
// 底层协议库。
//
// # 导入
//
// 模块路径与包名一致，导入后按包名调用：
//
//	import "github.com/WinterChocolates/YukiChoco"
//
//	c, err := YukiChoco.New(YukiChoco.Config{BaseURL: "http://127.0.0.1:6700"})
//
// # 协议基线
//
// 对齐 Milky 1.3（1.3.0）。协议数据模型（types 子包）由官方 Milky IR 生成，
// 非手写；协议升级时执行 `make generate` 重新生成。
//
// # 兼容性
//
// 依据官方兼容性契约，SDK 对协议端的演进是宽容的：未知事件、未知消息段、
// 未知枚举值都保留原始 JSON 而不是解码失败；消息段列表中的非法元素会被跳过。
//
// 事件语义需注意：Milky 没有 ack 与序号重放，事件源断开期间的事件会丢失，
// 不能依赖重连补发。
package YukiChoco
