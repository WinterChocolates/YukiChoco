package YukiChoco

// State 是事件连接状态。
//
// 使用 EventSourceNone（纯 API 模式）时，Start 会直接进入 StateConnected，
// 表示客户端已可用（但没有事件通道）。
type State int32

const (
	// StateDisconnected 表示尚未启动，或事件通道已停止。
	StateDisconnected State = iota
	// StateDialing 表示正在建立事件连接。
	StateDialing
	// StateConnected 表示事件通道已建立（收到的事件可交付）。
	StateConnected
	// StateReconnecting 表示连接已断开，正在按退避策略重连。
	StateReconnecting
	// StateClosed 表示客户端已 Shutdown。
	StateClosed
)

// String 返回状态名。
func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateDialing:
		return "dialing"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}
