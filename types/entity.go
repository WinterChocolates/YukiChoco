package types

// Sender 是消息发送者信息。
type Sender struct {
	UserID   UserID     `json:"user_id"`
	Nickname string     `json:"nickname,omitempty"`
	Card     string     `json:"card,omitempty"`
	Role     SenderRole `json:"role,omitempty"`
	Title    string     `json:"title,omitempty"`
	Level    string     `json:"level,omitempty"`
	Age      int        `json:"age,omitempty"`
	Sex      string     `json:"sex,omitempty"`
	Area     string     `json:"area,omitempty"`
}

// FriendInfo 是好友信息（get_friend_list）。
type FriendInfo struct {
	UserID   UserID `json:"user_id"`
	Nickname string `json:"nickname"`
	Remark   string `json:"remark"`
	Sex      string `json:"sex,omitempty"`
	Level    int    `json:"level,omitempty"`
	Age      int    `json:"age,omitempty"`
}

// GroupInfo 是群信息（get_group_list / get_group_info）。
type GroupInfo struct {
	GroupID        GroupID `json:"group_id"`
	GroupName      string  `json:"group_name"`
	MemberCount    int     `json:"member_count"`
	MaxMemberCount int     `json:"max_member_count"`
	Avatar         string  `json:"avatar,omitempty"`
	OwnerID        UserID  `json:"owner_id,omitempty"`
	CreateTime     int64   `json:"create_time,omitempty"`
	GroupMemo      string  `json:"group_memo,omitempty"`
}

// GroupMemberInfo 是群成员信息。
type GroupMemberInfo struct {
	GroupID         GroupID    `json:"group_id"`
	UserID          UserID     `json:"user_id"`
	Nickname        string     `json:"nickname"`
	Card            string     `json:"card"`
	Sex             string     `json:"sex,omitempty"`
	Age             int        `json:"age,omitempty"`
	Area            string     `json:"area,omitempty"`
	JoinTime        int64      `json:"join_time,omitempty"`
	LastSentTime    int64      `json:"last_sent_time,omitempty"`
	Level           string     `json:"level,omitempty"`
	Role            SenderRole `json:"role,omitempty"`
	Unfriendly      bool       `json:"unfriendly,omitempty"`
	Title           string     `json:"title,omitempty"`
	TitleExpireTime int64      `json:"title_expire_time,omitempty"`
	CardChangeable  bool       `json:"card_changeable,omitempty"`
	ShutUpTimestamp int64      `json:"shut_up_timestamp,omitempty"`
}

// StrangerInfo 是陌生人信息。
type StrangerInfo struct {
	UserID   UserID `json:"user_id"`
	Nickname string `json:"nickname"`
	Sex      string `json:"sex,omitempty"`
	Age      int    `json:"age,omitempty"`
	QID      string `json:"qid,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

// LoginInfo 是登录信息。
type LoginInfo struct {
	UserID   UserID  `json:"user_id"`
	Nickname string  `json:"nickname"`
	Status   *Status `json:"status,omitempty"`
}

// Status 是协议端运行状态。
type Status struct {
	Online bool           `json:"online"`
	Good   bool           `json:"good"`
	Stat   map[string]any `json:"stat,omitempty"`
}

// VersionInfo 是协议端版本信息。
type VersionInfo struct {
	AppName         string `json:"app_name"`
	AppVersion      string `json:"app_version"`
	ProtocolVersion string `json:"protocol_version"`
	AppFullName     string `json:"app_full_name,omitempty"`
	Version         string `json:"version,omitempty"`
	NTProtocol      string `json:"nt_protocol,omitempty"`
}

// OnlineClient 是在线客户端。
type OnlineClient struct {
	AppID      int64  `json:"app_id"`
	DeviceName string `json:"device_name"`
	DeviceKind string `json:"device_kind"`
}

// Cookies 是登录 Cookies。
type Cookies struct {
	Cookies string `json:"cookies"`
	B64     string `json:"b64,omitempty"`
}

// Credentials 是协议端凭证。
type Credentials struct {
	Cookies   string `json:"cookies"`
	CSRFToken string `json:"csrf_token"`
	Token     string `json:"token,omitempty"`
}

// UnidirectionalFriend 是单向好友。
type UnidirectionalFriend struct {
	UserID   UserID `json:"user_id"`
	Nickname string `json:"nickname"`
	Source   string `json:"source,omitempty"`
}

// GroupAtAllRemain 是 @全体成员剩余次数。
type GroupAtAllRemain struct {
	CanAtAll                 bool `json:"can_at_all"`
	RemainAtAllCountForGroup int  `json:"remain_at_all_count_for_group"`
	RemainAtAllCountForUin   int  `json:"remain_at_all_count_for_uin"`
}

// HonorMember 是群荣誉成员。
type HonorMember struct {
	UserID      UserID `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar,omitempty"`
	Description string `json:"description,omitempty"`
	DayCount    int    `json:"day_count,omitempty"`
}

// GroupHonorInfo 是群荣誉信息。
type GroupHonorInfo struct {
	GroupID          GroupID       `json:"group_id"`
	CurrentTalkative HonorMember   `json:"current_talkative"`
	TalkativeList    []HonorMember `json:"talkative_list,omitempty"`
	PerformerList    []HonorMember `json:"performer_list,omitempty"`
	LegendList       []HonorMember `json:"legend_list,omitempty"`
	StrongNewbieList []HonorMember `json:"strong_newbie_list,omitempty"`
	EmotionList      []HonorMember `json:"emotion_list,omitempty"`
}

// GroupSystemRequest 是群系统消息中的请求。
type GroupSystemRequest struct {
	GroupID   GroupID `json:"group_id"`
	GroupName string  `json:"group_name,omitempty"`
	RequestID int64   `json:"request_id,omitempty"`
	InviterID UserID  `json:"inviter_id,omitempty"`
	InviteeID UserID  `json:"invitee_id,omitempty"`
	UserID    UserID  `json:"user_id,omitempty"`
	Flag      Flag    `json:"flag"`
	Nickname  string  `json:"nickname,omitempty"`
	Message   string  `json:"message,omitempty"`
	Comment   string  `json:"comment,omitempty"`
	Checked   bool    `json:"checked,omitempty"`
	SubType   string  `json:"sub_type,omitempty"`
	Source    string  `json:"source,omitempty"`
	Time      int64   `json:"time,omitempty"`
}

// GroupSystemMessages 是群系统消息。
type GroupSystemMessages struct {
	InvitedRequests []GroupSystemRequest `json:"invited_requests"`
	JoinRequests    []GroupSystemRequest `json:"join_requests"`
}

// MessageDetail 是 get_msg 的返回。
type MessageDetail struct {
	MessageID   MessageID   `json:"message_id"`
	RealID      int64       `json:"real_id,omitempty"`
	MessageType MessageType `json:"message_type"`
	SubType     string      `json:"sub_type,omitempty"`
	Group       bool        `json:"group,omitempty"`
	GroupID     GroupID     `json:"group_id,omitempty"`
	UserID      UserID      `json:"user_id"`
	Time        int64       `json:"time"`
	Sender      Sender      `json:"sender"`
	Message     Message     `json:"message"`
	RawMessage  string      `json:"raw_message,omitempty"`
	Font        int         `json:"font,omitempty"`
}

// ForwardNode 是合并转发中的一条消息。
type ForwardNode struct {
	MessageID MessageID `json:"message_id,omitempty"`
	Sender    Sender    `json:"sender,omitempty"`
	UserID    UserID    `json:"user_id,omitempty"`
	Nickname  string    `json:"nickname,omitempty"`
	Time      int64     `json:"time,omitempty"`
	Message   Message   `json:"message,omitempty"`
	Content   Message   `json:"content,omitempty"`
}

// ForwardMessage 是 get_forward_msg 的返回。
type ForwardMessage struct {
	Messages []ForwardNode `json:"messages"`
}

// ImageInfo 是 get_image 的返回。
type ImageInfo struct {
	File     string `json:"file"`
	URL      string `json:"url,omitempty"`
	FileName string `json:"file_name,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// Record 是 get_record 的返回。
type Record struct {
	File     string `json:"file"`
	URL      string `json:"url,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// GroupFileSystemInfo 是群文件系统信息。
type GroupFileSystemInfo struct {
	FileCount  int `json:"file_count"`
	LimitCount int `json:"limit_count"`
	UsedSpace  int `json:"used_space"`
	TotalSpace int `json:"total_space"`
}

// GroupFile 是群文件。
type GroupFile struct {
	GroupID       GroupID `json:"group_id,omitempty"`
	FileID        string  `json:"file_id"`
	FileName      string  `json:"file_name"`
	BusID         int     `json:"busid,omitempty"`
	FileSize      int64   `json:"file_size,omitempty"`
	UploadTime    int64   `json:"upload_time,omitempty"`
	DeadTime      int64   `json:"dead_time,omitempty"`
	ModifyTime    int64   `json:"modify_time,omitempty"`
	DownloadTimes int     `json:"download_times,omitempty"`
	Uploader      UserID  `json:"uploader,omitempty"`
	UploaderName  string  `json:"uploader_name,omitempty"`
}

// GroupFolder 是群文件夹。
type GroupFolder struct {
	GroupID        GroupID `json:"group_id,omitempty"`
	FolderID       string  `json:"folder_id"`
	FolderName     string  `json:"folder_name"`
	CreateTime     int64   `json:"create_time,omitempty"`
	Creator        UserID  `json:"creator,omitempty"`
	CreatorName    string  `json:"creator_name,omitempty"`
	TotalFileCount int     `json:"total_file_count,omitempty"`
}

// GroupFiles 是群文件列表。
type GroupFiles struct {
	Files   []GroupFile   `json:"files"`
	Folders []GroupFolder `json:"folders"`
}

// Anonymous 是匿名用户信息。
type Anonymous struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Flag string `json:"flag,omitempty"`
}

// EssenceMessage 是群精华消息。
type EssenceMessage struct {
	SenderID     UserID    `json:"sender_id"`
	SenderNick   string    `json:"sender_nick"`
	SenderTime   int64     `json:"sender_time,omitempty"`
	OperatorID   UserID    `json:"operator_id"`
	OperatorNick string    `json:"operator_nick"`
	OperatorTime int64     `json:"operator_time,omitempty"`
	MessageID    MessageID `json:"message_id"`
	MessageSeq   int64     `json:"message_seq,omitempty"`
	MessageTime  int64     `json:"message_time,omitempty"`
	Content      string    `json:"content,omitempty"`
}

// EmojiLike 是消息表情点赞。
type EmojiLike struct {
	EmojiID string `json:"emoji_id"`
	Count   int    `json:"count"`
}

// GroupNoticeImage 是群公告图片。
type GroupNoticeImage struct {
	Height string `json:"height,omitempty"`
	Width  string `json:"width,omitempty"`
	ID     string `json:"id,omitempty"`
}

// GroupNoticeMessage 是群公告正文。
type GroupNoticeMessage struct {
	Text   string             `json:"text,omitempty"`
	Images []GroupNoticeImage `json:"images,omitempty"`
}

// GroupNotice 是群公告。
type GroupNotice struct {
	NoticeID    string             `json:"notice_id"`
	SenderID    UserID             `json:"sender_id"`
	PublishTime int64              `json:"publish_time"`
	Message     GroupNoticeMessage `json:"message"`
}

// MessageHistory 是 get_group_msg_history / get_friend_msg_history 的返回。
type MessageHistory struct {
	Messages []MessageDetail `json:"messages"`
}
