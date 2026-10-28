package YukiChoco

// State 是客户端事件连接的状态。
type State int32

const (
	// StateDisconnected 表示尚未启动，或连接已停止。
	StateDisconnected State = iota
	// StateDialing 表示正在建立事件连接。
	StateDialing
	// StateConnected 表示事件通道已建立（反向 WS 模式表示服务已在监听）。
	StateConnected
	// StateReconnecting 表示连接断开，正在按退避策略重连。
	StateReconnecting
	// StateClosed 表示客户端已关闭。
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
