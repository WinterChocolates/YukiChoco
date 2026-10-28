package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event 是所有事件的公共接口。
type Event interface {
	// EventType 返回 SDK 的扁平事件类型，例如 EventTypeGroupMessage。
	EventType() EventType
	// OccurredAt 返回事件发生时间。
	OccurredAt() time.Time
	// EventSelfID 返回收到该事件的机器人 QQ 号。
	EventSelfID() UserID
}

// EventBase 是所有事件的公共字段。
type EventBase struct {
	Time     int64    `json:"time"`
	SelfID   UserID   `json:"self_id"`
	PostType PostType `json:"post_type"`

	raw json.RawMessage
}

// OccurredAt 返回事件发生时间。
func (b EventBase) OccurredAt() time.Time { return time.Unix(b.Time, 0) }

// EventSelfID 返回机器人 QQ 号。
func (b EventBase) EventSelfID() UserID { return b.SelfID }

// RawJSON 返回事件的原始 JSON（由 ParseEvent 填充）。
func (b EventBase) RawJSON() json.RawMessage { return b.raw }

func (b *EventBase) setRaw(raw json.RawMessage) { b.raw = raw }

// MessageEvent 是消息事件的公共字段。
type MessageEvent struct {
	EventBase
	MessageType MessageType `json:"message_type"`
	SubType     string      `json:"sub_type,omitempty"`
	MessageID   MessageID   `json:"message_id"`
	UserID      UserID      `json:"user_id"`
	GroupID     GroupID     `json:"group_id,omitempty"`
	Message     Message     `json:"message"`
	RawMessage  string      `json:"raw_message,omitempty"`
	Font        int         `json:"font,omitempty"`
	Sender      Sender      `json:"sender,omitempty"`
	TempSource  int         `json:"temp_source,omitempty"`
}

// GroupMessageEvent 是群消息事件。
type GroupMessageEvent struct {
	MessageEvent
}

// EventType 实现 Event。
func (e *GroupMessageEvent) EventType() EventType { return EventTypeGroupMessage }

// AtMe 报告消息是否 @ 了指定用户。
func (e *GroupMessageEvent) AtMe(self UserID) bool { return e.Message.Mentioned(self) }

// AtAll 报告消息是否 @ 全体成员。
func (e *GroupMessageEvent) AtAll() bool { return e.Message.MentionsAll() }

// PlainText 返回消息中的纯文本。
func (e *GroupMessageEvent) PlainText() string { return e.Message.PlainText() }

// ReplyID 返回引用回复的目标消息 ID。
func (e *GroupMessageEvent) ReplyID() MessageID { return e.Message.ReplyID() }

// PrivateMessageEvent 是私聊消息事件。
type PrivateMessageEvent struct {
	MessageEvent
}

// EventType 实现 Event。
func (e *PrivateMessageEvent) EventType() EventType { return EventTypePrivateMessage }

// PlainText 返回消息中的纯文本。
func (e *PrivateMessageEvent) PlainText() string { return e.Message.PlainText() }

// ReplyID 返回引用回复的目标消息 ID。
func (e *PrivateMessageEvent) ReplyID() MessageID { return e.Message.ReplyID() }

// NoticeEvent 是通知事件的公共字段。
type NoticeEvent struct {
	EventBase
	NoticeType string  `json:"notice_type"`
	GroupID    GroupID `json:"group_id,omitempty"`
	UserID     UserID  `json:"user_id,omitempty"`
	OperatorID UserID  `json:"operator_id,omitempty"`
}

// UploadFile 是群文件上传信息。
type UploadFile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	BusID int    `json:"busid,omitempty"`
}

// GroupUploadEvent 是群文件上传事件。
type GroupUploadEvent struct {
	NoticeEvent
	File UploadFile `json:"file"`
}

// EventType 实现 Event。
func (e *GroupUploadEvent) EventType() EventType { return EventTypeGroupUpload }

// GroupAdminEvent 是群管理员变更事件（sub_type=set/unset）。
type GroupAdminEvent struct {
	NoticeEvent
	SubType string `json:"sub_type"`
}

// EventType 实现 Event。
func (e *GroupAdminEvent) EventType() EventType { return EventTypeGroupAdmin }

// GroupDecreaseEvent 是群成员减少事件（leave/kick/kick_me）。
type GroupDecreaseEvent struct {
	NoticeEvent
	SubType string `json:"sub_type"`
}

// EventType 实现 Event。
func (e *GroupDecreaseEvent) EventType() EventType { return EventTypeGroupDecrease }

// GroupIncreaseEvent 是群成员增加事件（approve/invite）。
type GroupIncreaseEvent struct {
	NoticeEvent
	SubType string `json:"sub_type"`
}

// EventType 实现 Event。
func (e *GroupIncreaseEvent) EventType() EventType { return EventTypeGroupIncrease }

// GroupBanEvent 是群禁言事件（ban/lift_ban）。
type GroupBanEvent struct {
	NoticeEvent
	SubType  string `json:"sub_type"`
	Duration int64  `json:"duration"`
}

// EventType 实现 Event。
func (e *GroupBanEvent) EventType() EventType { return EventTypeGroupBan }

// FriendAddEvent 是好友添加事件。
type FriendAddEvent struct {
	NoticeEvent
}

// EventType 实现 Event。
func (e *FriendAddEvent) EventType() EventType { return EventTypeFriendAdd }

// GroupRecallEvent 是群消息撤回事件。
type GroupRecallEvent struct {
	NoticeEvent
	MessageID MessageID `json:"message_id"`
}

// EventType 实现 Event。
func (e *GroupRecallEvent) EventType() EventType { return EventTypeGroupRecall }

// FriendRecallEvent 是私聊消息撤回事件。
type FriendRecallEvent struct {
	NoticeEvent
	MessageID MessageID `json:"message_id"`
}

// EventType 实现 Event。
func (e *FriendRecallEvent) EventType() EventType { return EventTypeFriendRecall }

// NotifyEvent 是 notify 通知（荣誉、戳一戳、运气王）。
type NotifyEvent struct {
	NoticeEvent
	SubType   string `json:"sub_type"`
	TargetID  UserID `json:"target_id,omitempty"`
	HonorType string `json:"honor_type,omitempty"`
}

// EventType 实现 Event。
func (e *NotifyEvent) EventType() EventType { return EventTypeNotify }

// IsPoke 报告是否是戳一戳。
func (e *NotifyEvent) IsPoke() bool { return e.SubType == SubTypeNotifyPoke }

// IsHonor 报告是否是群荣誉。
func (e *NotifyEvent) IsHonor() bool { return e.SubType == SubTypeNotifyHonor }

// IsLuckyKing 报告是否是运气王。
func (e *NotifyEvent) IsLuckyKing() bool { return e.SubType == SubTypeNotifyLuckyKing }

// GroupCardEvent 是群名片变更事件。
type GroupCardEvent struct {
	NoticeEvent
	CardNew string `json:"card_new"`
	CardOld string `json:"card_old"`
}

// EventType 实现 Event。
func (e *GroupCardEvent) EventType() EventType { return EventTypeGroupCard }

// OfflineFileEvent 是离线文件事件。
type OfflineFileEvent struct {
	NoticeEvent
	Name     string `json:"name"`
	FileSize int64  `json:"file_size"`
	URL      string `json:"url,omitempty"`
}

// EventType 实现 Event。
func (e *OfflineFileEvent) EventType() EventType { return EventTypeOfflineFile }

// ClientStatusEvent 是客户端在线状态事件。
type ClientStatusEvent struct {
	NoticeEvent
	Online bool `json:"online"`
	Client int  `json:"client"`
}

// EventType 实现 Event。
func (e *ClientStatusEvent) EventType() EventType { return EventTypeClientStatus }

// EssenceEvent 是精华消息变更事件。
type EssenceEvent struct {
	NoticeEvent
	SubType   string    `json:"sub_type"`
	MessageID MessageID `json:"message_id"`
}

// EventType 实现 Event。
func (e *EssenceEvent) EventType() EventType { return EventTypeEssence }

// GroupMsgEmojiLikeEvent 是群消息表情点赞事件（扩展）。
type GroupMsgEmojiLikeEvent struct {
	NoticeEvent
	MessageID MessageID   `json:"message_id"`
	Likes     []EmojiLike `json:"likes"`
	IsAdd     bool        `json:"is_add,omitempty"`
}

// EventType 实现 Event。
func (e *GroupMsgEmojiLikeEvent) EventType() EventType { return EventTypeGroupMsgEmojiLike }

// FriendRequestEvent 是好友请求事件。
type FriendRequestEvent struct {
	EventBase
	RequestType string `json:"request_type"`
	UserID      UserID `json:"user_id"`
	Comment     string `json:"comment"`
	Flag        Flag   `json:"flag"`
}

// EventType 实现 Event。
func (e *FriendRequestEvent) EventType() EventType { return EventTypeFriendRequest }

// GroupRequestEvent 是群请求事件（加群/邀请）。
type GroupRequestEvent struct {
	EventBase
	RequestType string  `json:"request_type"`
	SubType     string  `json:"sub_type"`
	GroupID     GroupID `json:"group_id"`
	UserID      UserID  `json:"user_id"`
	Comment     string  `json:"comment"`
	Flag        Flag    `json:"flag"`
}

// EventType 实现 Event。
func (e *GroupRequestEvent) EventType() EventType { return EventTypeGroupRequest }

// MetaEvent 是元事件的公共字段。
type MetaEvent struct {
	EventBase
	MetaEventType string `json:"meta_event_type"`
}

// LifecycleEvent 是生命周期事件。
type LifecycleEvent struct {
	MetaEvent
	SubType string `json:"sub_type"`
}

// EventType 实现 Event。
func (e *LifecycleEvent) EventType() EventType { return EventTypeLifecycle }

// HeartbeatEvent 是心跳事件。
type HeartbeatEvent struct {
	MetaEvent
	Status   *Status `json:"status,omitempty"`
	Interval int64   `json:"interval,omitempty"`
}

// EventType 实现 Event。
func (e *HeartbeatEvent) EventType() EventType { return EventTypeHeartbeat }

// UnknownEvent 是 SDK 尚未建模的事件。
//
// Raw 保留完整 JSON，调用方可以自行解析，不会因为新增事件类型而丢事件。
type UnknownEvent struct {
	EventBase
	Raw json.RawMessage `json:"-"`
}

// EventType 实现 Event。
func (e *UnknownEvent) EventType() EventType { return EventTypeUnknown }

// ParseEvent 把一条 OneBot v11 事件 JSON 解析为具体事件类型。
//
// 无法识别的事件返回 *UnknownEvent，而不是错误；格式错误的 JSON 才返回错误。
func ParseEvent(data []byte) (Event, error) {
	var head struct {
		PostType      PostType `json:"post_type"`
		MessageType   string   `json:"message_type"`
		NoticeType    string   `json:"notice_type"`
		RequestType   string   `json:"request_type"`
		MetaEventType string   `json:"meta_event_type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("types: 解析事件失败: %w", err)
	}

	var ev Event
	switch head.PostType {
	case PostTypeMessage:
		switch MessageType(head.MessageType) {
		case MessageTypeGroup:
			v := &GroupMessageEvent{}
			if err := json.Unmarshal(data, v); err != nil {
				return unknownEvent(data), nil
			}
			fillFromRawMessage(&v.Message, v.RawMessage)
			ev = v
		case MessageTypePrivate:
			v := &PrivateMessageEvent{}
			if err := json.Unmarshal(data, v); err != nil {
				return unknownEvent(data), nil
			}
			fillFromRawMessage(&v.Message, v.RawMessage)
			ev = v
		}
	case PostTypeNotice:
		switch head.NoticeType {
		case "group_upload":
			ev = decodeInto[GroupUploadEvent](data)
		case "group_admin":
			ev = decodeInto[GroupAdminEvent](data)
		case "group_decrease":
			ev = decodeInto[GroupDecreaseEvent](data)
		case "group_increase":
			ev = decodeInto[GroupIncreaseEvent](data)
		case "group_ban":
			ev = decodeInto[GroupBanEvent](data)
		case "friend_add":
			ev = decodeInto[FriendAddEvent](data)
		case "group_recall":
			ev = decodeInto[GroupRecallEvent](data)
		case "friend_recall":
			ev = decodeInto[FriendRecallEvent](data)
		case "notify":
			ev = decodeInto[NotifyEvent](data)
		case "group_card":
			ev = decodeInto[GroupCardEvent](data)
		case "offline_file":
			ev = decodeInto[OfflineFileEvent](data)
		case "client_status":
			ev = decodeInto[ClientStatusEvent](data)
		case "essence":
			ev = decodeInto[EssenceEvent](data)
		case "group_msg_emoji_like":
			ev = decodeInto[GroupMsgEmojiLikeEvent](data)
		}
	case PostTypeRequest:
		switch head.RequestType {
		case "friend":
			ev = decodeInto[FriendRequestEvent](data)
		case "group":
			ev = decodeInto[GroupRequestEvent](data)
		}
	case PostTypeMetaEvent:
		switch head.MetaEventType {
		case "lifecycle":
			ev = decodeInto[LifecycleEvent](data)
		case "heartbeat":
			ev = decodeInto[HeartbeatEvent](data)
		}
	}

	if ev == nil {
		ev = unknownEvent(data)
	}
	if setter, ok := ev.(interface{ setRaw(json.RawMessage) }); ok {
		setter.setRaw(append(json.RawMessage(nil), data...))
	}
	return ev, nil
}

// fillFromRawMessage 在协议端只提供 raw_message 时补全消息段。
func fillFromRawMessage(msg *Message, raw string) {
	if len(*msg) != 0 || raw == "" {
		return
	}
	if parsed, err := ParseCQ(raw); err == nil {
		*msg = parsed
	}
}

// unknownEvent 构造保留原始 JSON 的 UnknownEvent。
func unknownEvent(data []byte) Event {
	raw := append(json.RawMessage(nil), data...)
	unknown := &UnknownEvent{Raw: raw}
	_ = json.Unmarshal(data, unknown)
	unknown.setRaw(raw)
	return unknown
}

// decodeInto 解码具体事件；失败时返回带原始数据的最低限度事件。
func decodeInto[T any](data []byte) Event {
	v := new(T)
	if err := json.Unmarshal(data, v); err != nil {
		return unknownEvent(data)
	}
	if e, ok := any(v).(Event); ok {
		return e
	}
	return unknownEvent(data)
}
