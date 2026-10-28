// Package types 包含 Milky 协议（1.3）的数据模型。
//
// 除 ids.go、segment_ext.go、message_ext.go 等少数手写文件外，其余 *_gen.go
// 均由 internal/codegen 依据官方 Milky IR 生成，请勿手工修改；协议升级时执行
// `make generate`。
//
// 编码规则（对应协议兼容性契约，详见 docs/design-requirements.md 第 3.2 节）：
//
//   - 可选字段使用指针 + omitempty，避免把 Go 零值当成显式值上送；
//   - 带默认值的字段使用值类型 + omitempty，让协议端应用自己的默认值；
//   - 联合类型（事件、消息段、消息、群通知）都提供 Unknown 分支并保留原始 JSON，
//     协议端新增类型不会让 SDK 解码失败；
//   - 消息段列表解码时跳过非法元素而非整体失败。
package types
