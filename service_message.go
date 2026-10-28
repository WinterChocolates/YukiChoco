package YukiChoco

import (
	"context"

	"github.com/WinterChocolates/YukiChoco/types"
)

// MessageAPI 是消息类动作。
type MessageAPI struct{ c *Client }

// SendPrivate 发送私聊消息。
func (s *MessageAPI) SendPrivate(ctx context.Context, userID types.UserID, msg types.Message) (types.MessageID, error) {
	res, err := callTyped[types.SendPrivateMsgRequest, types.SendMsgResult](ctx, s.c, types.ActionSendPrivateMsg,
		&types.SendPrivateMsgRequest{UserID: userID, Message: msg})
	if err != nil {
		return 0, err
	}
	return res.MessageID, nil
}

// SendGroup 发送群消息。
func (s *MessageAPI) SendGroup(ctx context.Context, groupID types.GroupID, msg types.Message) (types.MessageID, error) {
	res, err := callTyped[types.SendGroupMsgRequest, types.SendMsgResult](ctx, s.c, types.ActionSendGroupMsg,
		&types.SendGroupMsgRequest{GroupID: groupID, Message: msg})
	if err != nil {
		return 0, err
	}
	return res.MessageID, nil
}

// Send 按 messageType 发送消息（private 或 group）。
func (s *MessageAPI) Send(ctx context.Context, messageType types.MessageType, userID types.UserID, groupID types.GroupID, msg types.Message) (types.MessageID, error) {
	res, err := callTyped[types.SendMsgRequest, types.SendMsgResult](ctx, s.c, types.ActionSendMsg,
		&types.SendMsgRequest{MessageType: messageType, UserID: userID, GroupID: groupID, Message: msg})
	if err != nil {
		return 0, err
	}
	return res.MessageID, nil
}

// SendPrivateForward 发送私聊合并转发。
func (s *MessageAPI) SendPrivateForward(ctx context.Context, userID types.UserID, nodes ...types.Segment) (types.MessageID, error) {
	res, err := callTyped[types.SendPrivateForwardMsgRequest, types.SendMsgResult](ctx, s.c, types.ActionSendPrivateForwardMsg,
		&types.SendPrivateForwardMsgRequest{UserID: userID, Messages: nodes})
	if err != nil {
		return 0, err
	}
	return res.MessageID, nil
}

// Get 获取消息。
func (s *MessageAPI) Get(ctx context.Context, messageID types.MessageID) (*types.MessageDetail, error) {
	return callTyped[types.GetMsgRequest, types.MessageDetail](ctx, s.c, types.ActionGetMsg,
		&types.GetMsgRequest{MessageID: messageID})
}

// GetForward 获取合并转发内容。
func (s *MessageAPI) GetForward(ctx context.Context, id string) (*types.ForwardMessage, error) {
	return callTyped[types.GetForwardMsgRequest, types.ForwardMessage](ctx, s.c, types.ActionGetForwardMsg,
		&types.GetForwardMsgRequest{ID: id})
}

// Delete 撤回消息。
func (s *MessageAPI) Delete(ctx context.Context, messageID types.MessageID) error {
	return callVoid(ctx, s.c, types.ActionDeleteMsg, &types.DeleteMsgRequest{MessageID: messageID})
}

// MarkAsRead 标记消息已读。
func (s *MessageAPI) MarkAsRead(ctx context.Context, messageID types.MessageID) error {
	return callVoid(ctx, s.c, types.ActionMarkMsgAsRead, &types.MarkMsgAsReadRequest{MessageID: messageID})
}

// GetImage 获取图片文件信息。
func (s *MessageAPI) GetImage(ctx context.Context, file string) (*types.ImageInfo, error) {
	return callTyped[types.GetImageRequest, types.ImageInfo](ctx, s.c, types.ActionGetImage,
		&types.GetImageRequest{File: file})
}

// GetRecord 获取语音文件信息（outFormat 通常为 mp3 / amr / wav）。
func (s *MessageAPI) GetRecord(ctx context.Context, file, outFormat string) (*types.Record, error) {
	return callTyped[types.GetRecordRequest, types.Record](ctx, s.c, types.ActionGetRecord,
		&types.GetRecordRequest{File: file, OutFormat: outFormat})
}

// GetGroupHistory 获取群消息历史。
func (s *MessageAPI) GetGroupHistory(ctx context.Context, groupID types.GroupID, messageSeq int64, count int) (*types.MessageHistory, error) {
	return callTyped[types.GetGroupMsgHistoryRequest, types.MessageHistory](ctx, s.c, types.ActionGetGroupMsgHistory,
		&types.GetGroupMsgHistoryRequest{GroupID: groupID, MessageSeq: messageSeq, Count: count})
}

// GetFriendHistory 获取私聊消息历史。
func (s *MessageAPI) GetFriendHistory(ctx context.Context, userID types.UserID, messageSeq int64, count int) (*types.MessageHistory, error) {
	return callTyped[types.GetFriendMsgHistoryRequest, types.MessageHistory](ctx, s.c, types.ActionGetFriendMsgHistory,
		&types.GetFriendMsgHistoryRequest{UserID: userID, MessageSeq: messageSeq, Count: count})
}
