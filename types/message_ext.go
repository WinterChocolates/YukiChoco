package types

// GroupMessage 把 message_receive 事件当作群消息解析。
func (e *MessageReceiveEvent) GroupMessage() (*GroupMessage, bool) {
	m, ok := e.Data.(*GroupMessage)
	return m, ok
}

// FriendMessage 把 message_receive 事件当作好友消息解析。
func (e *MessageReceiveEvent) FriendMessage() (*FriendMessage, bool) {
	m, ok := e.Data.(*FriendMessage)
	return m, ok
}

// TempMessage 把 message_receive 事件当作临时会话消息解析。
func (e *MessageReceiveEvent) TempMessage() (*TempMessage, bool) {
	m, ok := e.Data.(*TempMessage)
	return m, ok
}

// PlainText 返回群消息的纯文本内容。
func (m *GroupMessage) PlainText() string { return m.Segments.PlainText() }

// PlainText 返回好友消息的纯文本内容。
func (m *FriendMessage) PlainText() string { return m.Segments.PlainText() }

// PlainText 返回临时会话消息的纯文本内容。
func (m *TempMessage) PlainText() string { return m.Segments.PlainText() }

// DisplayName 返回发送者的显示名：优先群名片，其次昵称。
func (e *GroupMemberEntity) DisplayName() string {
	if e.Card != "" {
		return e.Card
	}
	return e.Nickname
}

// DisplayName 返回好友的显示名：优先备注，其次昵称。
func (e *FriendEntity) DisplayName() string {
	if e.Remark != "" {
		return e.Remark
	}
	return e.Nickname
}
