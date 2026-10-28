package YukiChoco

import (
	"context"

	"github.com/WinterChocolates/YukiChoco/types"
)

// 本文件是手写的便捷方法。生成的 service 方法已经够用，这里只补最常见的组合：
// 纯文本发送与引用回复。需要更复杂的消息时直接用 OutgoingSegments 组合。

// SendGroupText 以纯文本发送群消息。
func (c *Client) SendGroupText(ctx context.Context, groupID types.GroupID, text string) (*types.SendGroupMessageResponse, error) {
	return c.group.SendMessage(ctx, groupID, &types.OutgoingTextSegment{Text: text})
}

// SendPrivateText 以纯文本发送私聊消息。
func (c *Client) SendPrivateText(ctx context.Context, userID types.UserID, text string) (*types.SendPrivateMessageResponse, error) {
	return c.message.SendPrivateMessage(ctx, userID, &types.OutgoingTextSegment{Text: text})
}

// ReplyGroup 引用某条群消息并以纯文本回复。
func (c *Client) ReplyGroup(ctx context.Context, groupID types.GroupID, seq types.MessageSeq, text string) (*types.SendGroupMessageResponse, error) {
	return c.group.SendMessage(ctx, groupID,
		&types.OutgoingReplySegment{MessageSeq: seq},
		&types.OutgoingTextSegment{Text: text},
	)
}

// ReplyPrivate 引用某条私聊消息并以纯文本回复。
func (c *Client) ReplyPrivate(ctx context.Context, userID types.UserID, seq types.MessageSeq, text string) (*types.SendPrivateMessageResponse, error) {
	return c.message.SendPrivateMessage(ctx, userID,
		&types.OutgoingReplySegment{MessageSeq: seq},
		&types.OutgoingTextSegment{Text: text},
	)
}

// SendGroupSegments 发送任意消息段组合，便于把解码到的段直接转发出去。
func (c *Client) SendGroupSegments(ctx context.Context, groupID types.GroupID, segments ...types.OutgoingSegment) (*types.SendGroupMessageResponse, error) {
	return c.group.SendMessage(ctx, groupID, segments...)
}

// SendPrivateSegments 发送任意消息段组合到私聊。
func (c *Client) SendPrivateSegments(ctx context.Context, userID types.UserID, segments ...types.OutgoingSegment) (*types.SendPrivateMessageResponse, error) {
	return c.message.SendPrivateMessage(ctx, userID, segments...)
}
