// Package YukiChoco 是 OneBot v11 的 Go SDK。
//
// # 定位
//
// 本包是 OneBot v11 的**应用端**实现：通过 HTTP 或 WebSocket 调用协议端 API，
// 通过正向 WebSocket / 反向 WebSocket / WebHook 接收事件。它不实现 QQ 协议，
// 也不依赖任何 bot 框架，可作为其它框架的底层协议库。
//
// # 导入
//
//	import "github.com/WinterChocolates/YukiChoco"
//
//	c, err := YukiChoco.New(YukiChoco.Config{
//	    Transport: YukiChoco.TransportForwardWS,
//	    ForwardWS: YukiChoco.ForwardWSConfig{URL: "ws://127.0.0.1:3001/"},
//	})
//
// # 协议基线
//
// 对齐 OneBot v11（11.0），并实现文档中标注为「扩展」的常用 go-cqhttp 动作。
// 未知事件、未知消息段、未知字段不会导致解码失败；发送消息默认使用数组格式，
// 接收同时支持数组与 CQ 码字符串。
//
// # 事件语义
//
// OneBot v11 没有 ack 与序号重放，事件通道断开期间的事件会丢失，SDK 不承诺重连补发。
package YukiChoco
