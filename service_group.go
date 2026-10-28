package YukiChoco

import (
	"context"

	"github.com/WinterChocolates/YukiChoco/types"
)

// GroupAPI 是群类动作。
type GroupAPI struct{ c *Client }

// GetList 获取群列表。
func (s *GroupAPI) GetList(ctx context.Context) ([]types.GroupInfo, error) {
	var out []types.GroupInfo
	err := s.c.callInto(ctx, types.ActionGetGroupList, &types.EmptyRequest{}, &out)
	return out, err
}

// GetInfo 获取群信息。
func (s *GroupAPI) GetInfo(ctx context.Context, groupID types.GroupID, noCache bool) (*types.GroupInfo, error) {
	return callTyped[types.GetGroupInfoRequest, types.GroupInfo](ctx, s.c, types.ActionGetGroupInfo,
		&types.GetGroupInfoRequest{GroupID: groupID, NoCache: noCache})
}

// GetMemberInfo 获取群成员信息。
func (s *GroupAPI) GetMemberInfo(ctx context.Context, groupID types.GroupID, userID types.UserID, noCache bool) (*types.GroupMemberInfo, error) {
	return callTyped[types.GetGroupMemberInfoRequest, types.GroupMemberInfo](ctx, s.c, types.ActionGetGroupMemberInfo,
		&types.GetGroupMemberInfoRequest{GroupID: groupID, UserID: userID, NoCache: noCache})
}

// GetMemberList 获取群成员列表。
func (s *GroupAPI) GetMemberList(ctx context.Context, groupID types.GroupID) ([]types.GroupMemberInfo, error) {
	var out []types.GroupMemberInfo
	err := s.c.callInto(ctx, types.ActionGetGroupMemberList, &types.GetGroupMemberListRequest{GroupID: groupID}, &out)
	return out, err
}

// GetHonorInfo 获取群荣誉信息。
func (s *GroupAPI) GetHonorInfo(ctx context.Context, groupID types.GroupID, honorType types.HonorType) (*types.GroupHonorInfo, error) {
	return callTyped[types.GetGroupHonorInfoRequest, types.GroupHonorInfo](ctx, s.c, types.ActionGetGroupHonorInfo,
		&types.GetGroupHonorInfoRequest{GroupID: groupID, Type: honorType})
}

// GetSystemMsg 获取群系统消息。
func (s *GroupAPI) GetSystemMsg(ctx context.Context) (*types.GroupSystemMessages, error) {
	return callTyped[types.EmptyRequest, types.GroupSystemMessages](ctx, s.c, types.ActionGetGroupSystemMsg, &types.EmptyRequest{})
}

// GetAtAllRemain 获取 @全体成员 剩余次数（扩展）。
func (s *GroupAPI) GetAtAllRemain(ctx context.Context, groupID types.GroupID) (*types.GroupAtAllRemain, error) {
	return callTyped[types.GetGroupAtAllRemainRequest, types.GroupAtAllRemain](ctx, s.c, types.ActionGetGroupAtAllRemain,
		&types.GetGroupAtAllRemainRequest{GroupID: groupID})
}

// SetName 设置群名称。
func (s *GroupAPI) SetName(ctx context.Context, groupID types.GroupID, name string) error {
	return callVoid(ctx, s.c, types.ActionSetGroupName, &types.SetGroupNameRequest{GroupID: groupID, GroupName: name})
}

// SetPortrait 设置群头像（扩展）。
func (s *GroupAPI) SetPortrait(ctx context.Context, groupID types.GroupID, file string, cache int) error {
	return callVoid(ctx, s.c, types.ActionSetGroupPortrait, &types.SetGroupPortraitRequest{
		GroupID: groupID, File: file, Cache: cache,
	})
}

// SetAdmin 设置/取消群管理员。
func (s *GroupAPI) SetAdmin(ctx context.Context, groupID types.GroupID, userID types.UserID, enable bool) error {
	return callVoid(ctx, s.c, types.ActionSetGroupAdmin, &types.SetGroupAdminRequest{
		GroupID: groupID, UserID: userID, Enable: enable,
	})
}

// SetAnonymous 开启/关闭匿名聊天。
func (s *GroupAPI) SetAnonymous(ctx context.Context, groupID types.GroupID, enable bool) error {
	return callVoid(ctx, s.c, types.ActionSetGroupAnonymous, &types.SetGroupAnonymousRequest{
		GroupID: groupID, Enable: enable,
	})
}

// SetCard 设置群名片。
func (s *GroupAPI) SetCard(ctx context.Context, groupID types.GroupID, userID types.UserID, card string) error {
	return callVoid(ctx, s.c, types.ActionSetGroupCard, &types.SetGroupCardRequest{
		GroupID: groupID, UserID: userID, Card: card,
	})
}

// SetBan 禁言群成员；duration 为秒，0 表示解除禁言。
func (s *GroupAPI) SetBan(ctx context.Context, groupID types.GroupID, userID types.UserID, duration int64) error {
	return callVoid(ctx, s.c, types.ActionSetGroupBan, &types.SetGroupBanRequest{
		GroupID: groupID, UserID: userID, Duration: duration,
	})
}

// SetAnonymousBan 禁言匿名用户。
func (s *GroupAPI) SetAnonymousBan(ctx context.Context, req *types.SetGroupAnonymousBanRequest) error {
	return callVoid(ctx, s.c, types.ActionSetGroupAnonymousBan, req)
}

// SetWholeBan 全员禁言/解除。
func (s *GroupAPI) SetWholeBan(ctx context.Context, groupID types.GroupID, enable bool) error {
	return callVoid(ctx, s.c, types.ActionSetGroupWholeBan, &types.SetGroupWholeBanRequest{
		GroupID: groupID, Enable: enable,
	})
}

// Kick 踢出群成员。
func (s *GroupAPI) Kick(ctx context.Context, groupID types.GroupID, userID types.UserID, rejectAddRequest bool) error {
	return callVoid(ctx, s.c, types.ActionSetGroupKick, &types.SetGroupKickRequest{
		GroupID: groupID, UserID: userID, RejectAddRequest: rejectAddRequest,
	})
}

// Leave 退出群；dismiss 为 true 时解散群（仅群主）。
func (s *GroupAPI) Leave(ctx context.Context, groupID types.GroupID, dismiss bool) error {
	return callVoid(ctx, s.c, types.ActionSetGroupLeave, &types.SetGroupLeaveRequest{
		GroupID: groupID, IsDismiss: dismiss,
	})
}

// SetSpecialTitle 设置群头衔。
func (s *GroupAPI) SetSpecialTitle(ctx context.Context, groupID types.GroupID, userID types.UserID, title string, duration int64) error {
	return callVoid(ctx, s.c, types.ActionSetGroupSpecialTitle, &types.SetGroupSpecialTitleRequest{
		GroupID: groupID, UserID: userID, SpecialTitle: title, Duration: duration,
	})
}

// SetRemark 设置群备注（扩展）。
func (s *GroupAPI) SetRemark(ctx context.Context, groupID types.GroupID, remark string) error {
	return callVoid(ctx, s.c, types.ActionSetGroupRemark, &types.SetGroupRemarkRequest{
		GroupID: groupID, Remark: remark,
	})
}

// SetAddRequest 处理加群/邀请请求。
func (s *GroupAPI) SetAddRequest(ctx context.Context, flag types.Flag, subType string, approve bool, reason string) error {
	return callVoid(ctx, s.c, types.ActionSetGroupAddRequest, &types.SetGroupAddRequestRequest{
		Flag: flag, SubType: subType, Approve: approve, Reason: reason,
	})
}

// SendMessage 发送群消息（等价于 Client.Message().SendGroup）。
func (s *GroupAPI) SendMessage(ctx context.Context, groupID types.GroupID, msg types.Message) (types.MessageID, error) {
	return s.c.message.SendGroup(ctx, groupID, msg)
}

// SendForward 发送群合并转发。
func (s *GroupAPI) SendForward(ctx context.Context, groupID types.GroupID, nodes ...types.Segment) (types.MessageID, error) {
	res, err := callTyped[types.SendGroupForwardMsgRequest, types.SendMsgResult](ctx, s.c, types.ActionSendGroupForwardMsg,
		&types.SendGroupForwardMsgRequest{GroupID: groupID, Messages: nodes})
	if err != nil {
		return 0, err
	}
	return res.MessageID, nil
}

// GetHistory 获取群消息历史（等价于 Client.Message().GetGroupHistory）。
func (s *GroupAPI) GetHistory(ctx context.Context, groupID types.GroupID, messageSeq int64, count int) (*types.MessageHistory, error) {
	return s.c.message.GetGroupHistory(ctx, groupID, messageSeq, count)
}

// GetFileSystemInfo 获取群文件系统信息。
func (s *GroupAPI) GetFileSystemInfo(ctx context.Context, groupID types.GroupID) (*types.GroupFileSystemInfo, error) {
	return callTyped[types.GetGroupFileSystemInfoRequest, types.GroupFileSystemInfo](ctx, s.c, types.ActionGetGroupFileSystem,
		&types.GetGroupFileSystemInfoRequest{GroupID: groupID})
}

// GetRootFiles 获取群根目录文件列表。
func (s *GroupAPI) GetRootFiles(ctx context.Context, groupID types.GroupID) (*types.GroupFiles, error) {
	return callTyped[types.GetGroupRootFilesRequest, types.GroupFiles](ctx, s.c, types.ActionGetGroupRootFiles,
		&types.GetGroupRootFilesRequest{GroupID: groupID})
}

// GetFilesByFolder 获取群子目录文件列表。
func (s *GroupAPI) GetFilesByFolder(ctx context.Context, groupID types.GroupID, folderID string) (*types.GroupFiles, error) {
	return callTyped[types.GetGroupFilesByFolderRequest, types.GroupFiles](ctx, s.c, types.ActionGetGroupFilesByFolder,
		&types.GetGroupFilesByFolderRequest{GroupID: groupID, FolderID: folderID})
}

// GetFileURL 获取群文件下载链接。
func (s *GroupAPI) GetFileURL(ctx context.Context, groupID types.GroupID, fileID string, busID int) (string, error) {
	res, err := callTyped[types.GetGroupFileURLRequest, types.FileURLResult](ctx, s.c, types.ActionGetGroupFileURL,
		&types.GetGroupFileURLRequest{GroupID: groupID, FileID: fileID, BusID: busID})
	if err != nil {
		return "", err
	}
	return res.URL, nil
}

// UploadFile 上传群文件。
//
// file 可以是协议端可访问的本地路径、http(s) URL 或 base64://。
func (s *GroupAPI) UploadFile(ctx context.Context, groupID types.GroupID, file, name, folder string) error {
	return callVoid(ctx, s.c, types.ActionUploadGroupFile, &types.UploadGroupFileRequest{
		GroupID: groupID, File: file, Name: name, Folder: folder,
	})
}

// DeleteFile 删除群文件。
func (s *GroupAPI) DeleteFile(ctx context.Context, groupID types.GroupID, fileID string, busID int) error {
	return callVoid(ctx, s.c, types.ActionDeleteGroupFile, &types.DeleteGroupFileRequest{
		GroupID: groupID, FileID: fileID, BusID: busID,
	})
}

// CreateFileFolder 创建群文件夹。
func (s *GroupAPI) CreateFileFolder(ctx context.Context, groupID types.GroupID, name, parentID string) error {
	return callVoid(ctx, s.c, types.ActionCreateGroupFileFolder, &types.CreateGroupFileFolderRequest{
		GroupID: groupID, Name: name, ParentID: parentID,
	})
}

// DeleteFileFolder 删除群文件夹。
func (s *GroupAPI) DeleteFileFolder(ctx context.Context, groupID types.GroupID, folderID string) error {
	return callVoid(ctx, s.c, types.ActionDeleteGroupFileFolder, &types.DeleteGroupFileFolderRequest{
		GroupID: groupID, FolderID: folderID,
	})
}

// GetNotice 获取群公告（扩展）。
func (s *GroupAPI) GetNotice(ctx context.Context, groupID types.GroupID) ([]types.GroupNotice, error) {
	var out []types.GroupNotice
	err := s.c.callInto(ctx, types.ActionGetGroupNotice, &types.GetGroupNoticeRequest{GroupID: groupID}, &out)
	return out, err
}

// SendNotice 发送群公告（扩展）。
func (s *GroupAPI) SendNotice(ctx context.Context, groupID types.GroupID, content string) error {
	return callVoid(ctx, s.c, types.ActionSendGroupNotice, &types.SendGroupNoticeRequest{
		GroupID: groupID, Content: content,
	})
}

// GetEssenceMsgList 获取群精华消息列表（扩展）。
func (s *GroupAPI) GetEssenceMsgList(ctx context.Context, groupID types.GroupID) ([]types.EssenceMessage, error) {
	var out []types.EssenceMessage
	err := s.c.callInto(ctx, types.ActionGetEssenceMsgList, &types.GetEssenceMsgListRequest{GroupID: groupID}, &out)
	return out, err
}

// SetEssenceMsg 设置群精华消息（扩展）。
func (s *GroupAPI) SetEssenceMsg(ctx context.Context, messageID types.MessageID) error {
	return callVoid(ctx, s.c, types.ActionSetEssenceMsg, &types.SetEssenceMsgRequest{MessageID: messageID})
}

// DeleteEssenceMsg 移除群精华消息（扩展）。
func (s *GroupAPI) DeleteEssenceMsg(ctx context.Context, messageID types.MessageID) error {
	return callVoid(ctx, s.c, types.ActionDeleteEssenceMsg, &types.DeleteEssenceMsgRequest{MessageID: messageID})
}
