package types

// 本文件定义 OneBot v11 动作的请求/响应类型。
//
// 只包含 SDK 已建模的动作；未覆盖的动作用 Client.Call / CallRaw 调用。

// 动作名称常量。
const (
	ActionGetLoginInfo          = "get_login_info"
	ActionGetStatus             = "get_status"
	ActionGetVersionInfo        = "get_version_info"
	ActionGetOnlineClients      = "get_online_clients"
	ActionGetStrangerInfo       = "get_stranger_info"
	ActionGetFriendList         = "get_friend_list"
	ActionGetUnidirectionalList = "get_unidirectional_friend_list"
	ActionGetGroupList          = "get_group_list"
	ActionGetGroupInfo          = "get_group_info"
	ActionGetGroupMemberInfo    = "get_group_member_info"
	ActionGetGroupMemberList    = "get_group_member_list"
	ActionGetGroupHonorInfo     = "get_group_honor_info"
	ActionGetGroupSystemMsg     = "get_group_system_msg"
	ActionGetGroupAtAllRemain   = "get_group_at_all_remain"
	ActionGetCookies            = "get_cookies"
	ActionGetCSRFToken          = "get_csrf_token"
	ActionGetCredentials        = "get_credentials"
	ActionGetRecord             = "get_record"
	ActionGetImage              = "get_image"
	ActionGetMsg                = "get_msg"
	ActionGetForwardMsg         = "get_forward_msg"
	ActionGetGroupMsgHistory    = "get_group_msg_history"
	ActionGetFriendMsgHistory   = "get_friend_msg_history"
	ActionGetGroupFileSystem    = "get_group_file_system_info"
	ActionGetGroupRootFiles     = "get_group_root_files"
	ActionGetGroupFilesByFolder = "get_group_files_by_folder"
	ActionGetGroupFileURL       = "get_group_file_url"
	ActionGetPrivateFileURL     = "get_private_file_url"
	ActionGetGroupNotice        = "get_group_notice"
	ActionGetEssenceMsgList     = "get_essence_msg_list"

	ActionSendPrivateMsg        = "send_private_msg"
	ActionSendGroupMsg          = "send_group_msg"
	ActionSendMsg               = "send_msg"
	ActionSendLike              = "send_like"
	ActionSendGroupForwardMsg   = "send_group_forward_msg"
	ActionSendPrivateForwardMsg = "send_private_forward_msg"
	ActionSendGroupNotice       = "send_group_notice"
	ActionDeleteMsg             = "delete_msg"
	ActionDeleteFriend          = "delete_friend"
	ActionMarkMsgAsRead         = "mark_msg_as_read"
	ActionSetFriendAddRequest   = "set_friend_add_request"
	ActionSetGroupAddRequest    = "set_group_add_request"
	ActionSetGroupName          = "set_group_name"
	ActionSetGroupPortrait      = "set_group_portrait"
	ActionSetGroupAdmin         = "set_group_admin"
	ActionSetGroupAnonymous     = "set_group_anonymous"
	ActionSetGroupCard          = "set_group_card"
	ActionSetGroupBan           = "set_group_ban"
	ActionSetGroupAnonymousBan  = "set_group_anonymous_ban"
	ActionSetGroupWholeBan      = "set_group_whole_ban"
	ActionSetGroupKick          = "set_group_kick"
	ActionSetGroupLeave         = "set_group_leave"
	ActionSetGroupSpecialTitle  = "set_group_special_title"
	ActionSetGroupRemark        = "set_group_remark"
	ActionSetQQProfile          = "set_qq_profile"
	ActionSetQQAvatar           = "set_qq_avatar"
	ActionSetRestart            = "set_restart"
	ActionSetEssenceMsg         = "set_essence_msg"
	ActionDeleteEssenceMsg      = "delete_essence_msg"

	ActionUploadGroupFile       = "upload_group_file"
	ActionUploadPrivateFile     = "upload_private_file"
	ActionDeleteGroupFile       = "delete_group_file"
	ActionCreateGroupFileFolder = "create_group_file_folder"
	ActionDeleteGroupFileFolder = "delete_group_file_folder"
	ActionDownloadFile          = "download_file"

	ActionCanSendImage  = "can_send_image"
	ActionCanSendRecord = "can_send_record"
)

// EmptyRequest 是没有参数的动作的请求。
type EmptyRequest struct{}

// SendMsgResult 是发送消息类动作的返回。
type SendMsgResult struct {
	MessageID MessageID `json:"message_id"`
}

// SendPrivateMsgRequest 是 send_private_msg 的参数。
type SendPrivateMsgRequest struct {
	UserID     UserID  `json:"user_id"`
	Message    Message `json:"message"`
	AutoEscape bool    `json:"auto_escape,omitempty"`
}

// SendGroupMsgRequest 是 send_group_msg 的参数。
type SendGroupMsgRequest struct {
	GroupID    GroupID `json:"group_id"`
	Message    Message `json:"message"`
	AutoEscape bool    `json:"auto_escape,omitempty"`
}

// SendMsgRequest 是 send_msg 的参数（按 message_type 区分私聊/群聊）。
type SendMsgRequest struct {
	MessageType MessageType `json:"message_type"`
	UserID      UserID      `json:"user_id,omitempty"`
	GroupID     GroupID     `json:"group_id,omitempty"`
	Message     Message     `json:"message"`
	AutoEscape  bool        `json:"auto_escape,omitempty"`
}

// SendGroupForwardMsgRequest 是 send_group_forward_msg 的参数。
//
// Messages 由 node 段组成，可用 segment.Node 构造。
type SendGroupForwardMsgRequest struct {
	GroupID  GroupID   `json:"group_id"`
	Messages []Segment `json:"messages"`
}

// SendPrivateForwardMsgRequest 是 send_private_forward_msg 的参数。
type SendPrivateForwardMsgRequest struct {
	UserID   UserID    `json:"user_id"`
	Messages []Segment `json:"messages"`
}

// SendLikeRequest 是 send_like 的参数。
type SendLikeRequest struct {
	UserID UserID `json:"user_id"`
	Times  int    `json:"times,omitempty"`
}

// DeleteMsgRequest 是 delete_msg 的参数。
type DeleteMsgRequest struct {
	MessageID MessageID `json:"message_id"`
}

// GetMsgRequest 是 get_msg 的参数。
type GetMsgRequest struct {
	MessageID MessageID `json:"message_id"`
}

// GetForwardMsgRequest 是 get_forward_msg 的参数。
type GetForwardMsgRequest struct {
	ID string `json:"id"`
}

// MarkMsgAsReadRequest 是 mark_msg_as_read 的参数。
type MarkMsgAsReadRequest struct {
	MessageID MessageID `json:"message_id"`
}

// GetGroupMsgHistoryRequest 是 get_group_msg_history 的参数。
type GetGroupMsgHistoryRequest struct {
	GroupID    GroupID `json:"group_id"`
	MessageSeq int64   `json:"message_seq,omitempty"`
	Count      int     `json:"count,omitempty"`
}

// GetFriendMsgHistoryRequest 是 get_friend_msg_history 的参数。
type GetFriendMsgHistoryRequest struct {
	UserID     UserID `json:"user_id"`
	MessageSeq int64  `json:"message_seq,omitempty"`
	Count      int    `json:"count,omitempty"`
}

// GetImageRequest 是 get_image 的参数。
type GetImageRequest struct {
	File string `json:"file"`
}

// GetRecordRequest 是 get_record 的参数。
type GetRecordRequest struct {
	File      string `json:"file"`
	OutFormat string `json:"out_format"`
}

// GetStrangerInfoRequest 是 get_stranger_info 的参数。
type GetStrangerInfoRequest struct {
	UserID  UserID `json:"user_id"`
	NoCache bool   `json:"no_cache,omitempty"`
}

// GetGroupInfoRequest 是 get_group_info 的参数。
type GetGroupInfoRequest struct {
	GroupID GroupID `json:"group_id"`
	NoCache bool    `json:"no_cache,omitempty"`
}

// GetGroupMemberInfoRequest 是 get_group_member_info 的参数。
type GetGroupMemberInfoRequest struct {
	GroupID GroupID `json:"group_id"`
	UserID  UserID  `json:"user_id"`
	NoCache bool    `json:"no_cache,omitempty"`
}

// GetGroupMemberListRequest 是 get_group_member_list 的参数。
type GetGroupMemberListRequest struct {
	GroupID GroupID `json:"group_id"`
}

// GetGroupHonorInfoRequest 是 get_group_honor_info 的参数。
type GetGroupHonorInfoRequest struct {
	GroupID GroupID   `json:"group_id"`
	Type    HonorType `json:"type"`
}

// GetGroupAtAllRemainRequest 是 get_group_at_all_remain 的参数。
type GetGroupAtAllRemainRequest struct {
	GroupID GroupID `json:"group_id"`
}

// GetOnlineClientsRequest 是 get_online_clients 的参数。
type GetOnlineClientsRequest struct {
	NoCache bool `json:"no_cache,omitempty"`
}

// GetCookiesRequest 是 get_cookies 的参数。
type GetCookiesRequest struct {
	Domain string `json:"domain,omitempty"`
}

// GetCredentialsRequest 是 get_credentials 的参数。
type GetCredentialsRequest struct {
	Domain string `json:"domain,omitempty"`
}

// GetCSRFTokenResult 是 get_csrf_token 的返回。
type GetCSRFTokenResult struct {
	Token int64 `json:"token"`
}

// SetFriendAddRequestRequest 是 set_friend_add_request 的参数。
type SetFriendAddRequestRequest struct {
	Flag    Flag   `json:"flag"`
	Approve bool   `json:"approve"`
	Remark  string `json:"remark,omitempty"`
}

// SetGroupAddRequestRequest 是 set_group_add_request 的参数。
type SetGroupAddRequestRequest struct {
	Flag    Flag   `json:"flag"`
	SubType string `json:"sub_type"`
	Approve bool   `json:"approve"`
	Reason  string `json:"reason,omitempty"`
}

// SetGroupNameRequest 是 set_group_name 的参数。
type SetGroupNameRequest struct {
	GroupID   GroupID `json:"group_id"`
	GroupName string  `json:"group_name"`
}

// SetGroupPortraitRequest 是 set_group_portrait 的参数。
type SetGroupPortraitRequest struct {
	GroupID GroupID `json:"group_id"`
	File    string  `json:"file"`
	Cache   int     `json:"cache,omitempty"`
}

// SetGroupAdminRequest 是 set_group_admin 的参数。
type SetGroupAdminRequest struct {
	GroupID GroupID `json:"group_id"`
	UserID  UserID  `json:"user_id"`
	Enable  bool    `json:"enable"`
}

// SetGroupAnonymousRequest 是 set_group_anonymous 的参数。
type SetGroupAnonymousRequest struct {
	GroupID GroupID `json:"group_id"`
	Enable  bool    `json:"enable"`
}

// SetGroupCardRequest 是 set_group_card 的参数。
type SetGroupCardRequest struct {
	GroupID GroupID `json:"group_id"`
	UserID  UserID  `json:"user_id"`
	Card    string  `json:"card"`
}

// SetGroupBanRequest 是 set_group_ban 的参数。
type SetGroupBanRequest struct {
	GroupID  GroupID `json:"group_id"`
	UserID   UserID  `json:"user_id"`
	Duration int64   `json:"duration"`
}

// SetGroupAnonymousBanRequest 是 set_group_anonymous_ban 的参数。
type SetGroupAnonymousBanRequest struct {
	GroupID   GroupID    `json:"group_id"`
	Anonymous *Anonymous `json:"anonymous,omitempty"`
	Flag      string     `json:"flag,omitempty"`
	Duration  int64      `json:"duration"`
}

// SetGroupWholeBanRequest 是 set_group_whole_ban 的参数。
type SetGroupWholeBanRequest struct {
	GroupID GroupID `json:"group_id"`
	Enable  bool    `json:"enable"`
}

// SetGroupKickRequest 是 set_group_kick 的参数。
type SetGroupKickRequest struct {
	GroupID          GroupID `json:"group_id"`
	UserID           UserID  `json:"user_id"`
	RejectAddRequest bool    `json:"reject_add_request,omitempty"`
}

// SetGroupLeaveRequest 是 set_group_leave 的参数。
type SetGroupLeaveRequest struct {
	GroupID   GroupID `json:"group_id"`
	IsDismiss bool    `json:"is_dismiss,omitempty"`
}

// SetGroupSpecialTitleRequest 是 set_group_special_title 的参数。
type SetGroupSpecialTitleRequest struct {
	GroupID      GroupID `json:"group_id"`
	UserID       UserID  `json:"user_id"`
	SpecialTitle string  `json:"special_title"`
	Duration     int64   `json:"duration,omitempty"`
}

// SetGroupRemarkRequest 是 set_group_remark 的参数（扩展）。
type SetGroupRemarkRequest struct {
	GroupID GroupID `json:"group_id"`
	Remark  string  `json:"remark"`
}

// SetQQProfileRequest 是 set_qq_profile 的参数（扩展）。
type SetQQProfileRequest struct {
	Nickname     string `json:"nickname,omitempty"`
	Company      string `json:"company,omitempty"`
	Email        string `json:"email,omitempty"`
	College      string `json:"college,omitempty"`
	PersonalNote string `json:"personal_note,omitempty"`
}

// SetQQAvatarRequest 是 set_qq_avatar 的参数（扩展）。
type SetQQAvatarRequest struct {
	File string `json:"file"`
}

// SetRestartRequest 是 set_restart 的参数。
type SetRestartRequest struct {
	Delay int `json:"delay,omitempty"`
}

// DownloadFileRequest 是 download_file 的参数（go-cqhttp 扩展）。
type DownloadFileRequest struct {
	URL         string `json:"url"`
	ThreadCount int    `json:"thread_count,omitempty"`
	Headers     string `json:"headers,omitempty"`
}

// UploadGroupFileRequest 是 upload_group_file 的参数。
type UploadGroupFileRequest struct {
	GroupID GroupID `json:"group_id"`
	File    string  `json:"file"`
	Name    string  `json:"name"`
	Folder  string  `json:"folder,omitempty"`
}

// UploadPrivateFileRequest 是 upload_private_file 的参数。
type UploadPrivateFileRequest struct {
	UserID UserID `json:"user_id"`
	File   string `json:"file"`
	Name   string `json:"name"`
}

// GetGroupFileSystemInfoRequest 是 get_group_file_system_info 的参数。
type GetGroupFileSystemInfoRequest struct {
	GroupID GroupID `json:"group_id"`
}

// GetGroupRootFilesRequest 是 get_group_root_files 的参数。
type GetGroupRootFilesRequest struct {
	GroupID GroupID `json:"group_id"`
}

// GetGroupFilesByFolderRequest 是 get_group_files_by_folder 的参数。
type GetGroupFilesByFolderRequest struct {
	GroupID  GroupID `json:"group_id"`
	FolderID string  `json:"folder_id"`
}

// GetGroupFileURLRequest 是 get_group_file_url 的参数。
type GetGroupFileURLRequest struct {
	GroupID GroupID `json:"group_id"`
	FileID  string  `json:"file_id"`
	BusID   int     `json:"busid,omitempty"`
}

// GetPrivateFileURLRequest 是 get_private_file_url 的参数。
type GetPrivateFileURLRequest struct {
	FileID string `json:"file_id"`
}

// FileURLResult 是 get_group_file_url / get_private_file_url 的返回。
type FileURLResult struct {
	URL string `json:"url"`
}

// DeleteGroupFileRequest 是 delete_group_file 的参数。
type DeleteGroupFileRequest struct {
	GroupID GroupID `json:"group_id"`
	FileID  string  `json:"file_id"`
	BusID   int     `json:"busid,omitempty"`
}

// CreateGroupFileFolderRequest 是 create_group_file_folder 的参数。
type CreateGroupFileFolderRequest struct {
	GroupID  GroupID `json:"group_id"`
	Name     string  `json:"name"`
	ParentID string  `json:"parent_id,omitempty"`
}

// DeleteGroupFileFolderRequest 是 delete_group_file_folder 的参数。
type DeleteGroupFileFolderRequest struct {
	GroupID  GroupID `json:"group_id"`
	FolderID string  `json:"folder_id"`
}

// GetGroupNoticeRequest 是 get_group_notice 的参数（扩展）。
type GetGroupNoticeRequest struct {
	GroupID GroupID `json:"group_id"`
}

// SendGroupNoticeRequest 是 _send_group_notice 的参数（扩展）。
type SendGroupNoticeRequest struct {
	GroupID GroupID `json:"group_id"`
	Content string  `json:"content"`
}

// GetEssenceMsgListRequest 是 get_essence_msg_list 的参数（扩展）。
type GetEssenceMsgListRequest struct {
	GroupID GroupID `json:"group_id"`
}

// SetEssenceMsgRequest 是 set_essence_msg 的参数（扩展）。
type SetEssenceMsgRequest struct {
	MessageID MessageID `json:"message_id"`
}

// DeleteEssenceMsgRequest 是 delete_essence_msg 的参数（扩展）。
type DeleteEssenceMsgRequest struct {
	MessageID MessageID `json:"message_id"`
}

// DeleteFriendRequest 是 delete_friend 的参数（扩展）。
type DeleteFriendRequest struct {
	UserID UserID `json:"user_id"`
}

// CanSendResult 是 can_send_image / can_send_record 的返回。
type CanSendResult struct {
	Yes bool `json:"yes"`
}
