package types

// PostType 是事件的大类。
type PostType string

// OneBot v11 事件大类。
const (
	PostTypeMessage   PostType = "message"
	PostTypeNotice    PostType = "notice"
	PostTypeRequest   PostType = "request"
	PostTypeMetaEvent PostType = "meta_event"
)

// MessageType 是消息类型。
type MessageType string

// 消息类型。
const (
	MessageTypePrivate MessageType = "private"
	MessageTypeGroup   MessageType = "group"
)

// SenderRole 是群成员角色。
type SenderRole string

// 群成员角色。
const (
	RoleOwner  SenderRole = "owner"
	RoleAdmin  SenderRole = "admin"
	RoleMember SenderRole = "member"
)

// HonorType 是群荣誉类型。
type HonorType string

// 群荣誉类型。
const (
	HonorTalkative    HonorType = "talkative"
	HonorPerformer    HonorType = "performer"
	HonorLegend       HonorType = "legend"
	HonorStrongNewbie HonorType = "strong_newbie"
	HonorEmotion      HonorType = "emotion"
	HonorAll          HonorType = "all"
)

// EventType 是 SDK 内部使用的事件类型标识。
//
// 采用扁平命名（`notice.group_recall`），便于订阅与日志检索；与 OneBot 的
// `post_type` / `*_type` 组合一一对应。
type EventType string

// 事件类型。
const (
	EventTypeUnknown EventType = "unknown"

	EventTypePrivateMessage EventType = "message.private"
	EventTypeGroupMessage   EventType = "message.group"

	EventTypeGroupUpload       EventType = "notice.group_upload"
	EventTypeGroupAdmin        EventType = "notice.group_admin"
	EventTypeGroupDecrease     EventType = "notice.group_decrease"
	EventTypeGroupIncrease     EventType = "notice.group_increase"
	EventTypeGroupBan          EventType = "notice.group_ban"
	EventTypeFriendAdd         EventType = "notice.friend_add"
	EventTypeGroupRecall       EventType = "notice.group_recall"
	EventTypeFriendRecall      EventType = "notice.friend_recall"
	EventTypeNotify            EventType = "notice.notify"
	EventTypeGroupCard         EventType = "notice.group_card"
	EventTypeOfflineFile       EventType = "notice.offline_file"
	EventTypeClientStatus      EventType = "notice.client_status"
	EventTypeEssence           EventType = "notice.essence"
	EventTypeGroupMsgEmojiLike EventType = "notice.group_msg_emoji_like"

	EventTypeFriendRequest EventType = "request.friend"
	EventTypeGroupRequest  EventType = "request.group"

	EventTypeLifecycle EventType = "meta.lifecycle"
	EventTypeHeartbeat EventType = "meta.heartbeat"
)

// 通知子类型常量。
const (
	SubTypeGroupAdminSet   = "set"
	SubTypeGroupAdminUnset = "unset"

	SubTypeGroupDecreaseLeave  = "leave"
	SubTypeGroupDecreaseKick   = "kick"
	SubTypeGroupDecreaseKickMe = "kick_me"

	SubTypeGroupIncreaseApprove = "approve"
	SubTypeGroupIncreaseInvite  = "invite"

	SubTypeGroupBan           = "ban"
	SubTypeGroupLiftBan       = "lift_ban"
	SubTypeGroupCard          = "group_card"
	SubTypeEssenceAdd         = "add"
	SubTypeEssenceDelete      = "delete"
	SubTypeNotifyHonor        = "honor"
	SubTypeNotifyPoke         = "poke"
	SubTypeNotifyLuckyKing    = "lucky_king"
	SubTypeGroupRequestAdd    = "add"
	SubTypeGroupRequestInvite = "invite"
)
