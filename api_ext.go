package YukiChoco

import (
	"context"
	"fmt"

	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

// SendGroupMessage 发送群消息。
func (c *Client) SendGroupMessage(ctx context.Context, groupID types.GroupID, msg types.Message) (types.MessageID, error) {
	return c.message.SendGroup(ctx, groupID, msg)
}

// SendPrivateMessage 发送私聊消息。
func (c *Client) SendPrivateMessage(ctx context.Context, userID types.UserID, msg types.Message) (types.MessageID, error) {
	return c.message.SendPrivate(ctx, userID, msg)
}

// SendGroupText 以纯文本发送群消息。
func (c *Client) SendGroupText(ctx context.Context, groupID types.GroupID, text string) (types.MessageID, error) {
	return c.message.SendGroup(ctx, groupID, segment.TextMessage(text))
}

// SendPrivateText 以纯文本发送私聊消息。
func (c *Client) SendPrivateText(ctx context.Context, userID types.UserID, text string) (types.MessageID, error) {
	return c.message.SendPrivate(ctx, userID, segment.TextMessage(text))
}

// ReplyGroup 引用群消息并回复纯文本。
func (c *Client) ReplyGroup(ctx context.Context, groupID types.GroupID, messageID types.MessageID, text string) (types.MessageID, error) {
	return c.message.SendGroup(ctx, groupID, types.Message{
		segment.Reply(messageID),
		segment.Text(text),
	})
}

// ReplyPrivate 引用私聊消息并回复纯文本。
func (c *Client) ReplyPrivate(ctx context.Context, userID types.UserID, messageID types.MessageID, text string) (types.MessageID, error) {
	return c.message.SendPrivate(ctx, userID, types.Message{
		segment.Reply(messageID),
		segment.Text(text),
	})
}

// ReplyGroupMessage 引用一条群消息事件并回复纯文本。
func (c *Client) ReplyGroupMessage(ctx context.Context, ev *types.GroupMessageEvent, text string) (types.MessageID, error) {
	return c.ReplyGroup(ctx, ev.GroupID, ev.MessageID, text)
}

// ReplyPrivateMessage 引用一条私聊消息事件并回复纯文本。
func (c *Client) ReplyPrivateMessage(ctx context.Context, ev *types.PrivateMessageEvent, text string) (types.MessageID, error) {
	return c.ReplyPrivate(ctx, ev.UserID, ev.MessageID, text)
}

// UploadGroupFileFrom 上传本地文件到群文件。
func (c *Client) UploadGroupFileFrom(ctx context.Context, groupID types.GroupID, path, name string) error {
	file, err := segment.URI(path)
	if err != nil {
		return err
	}
	return c.group.UploadFile(ctx, groupID, file, name, "")
}

// UploadGroupFileWith 上传任意来源（路径 / []byte / io.Reader / URL）到群文件。
func (c *Client) UploadGroupFileWith(ctx context.Context, groupID types.GroupID, name string, src any, folder string) error {
	file, err := segment.URI(src)
	if err != nil {
		return err
	}
	return c.group.UploadFile(ctx, groupID, file, name, folder)
}

// UploadPrivateFileFrom 上传本地文件到私聊。
func (c *Client) UploadPrivateFileFrom(ctx context.Context, userID types.UserID, path, name string) error {
	file, err := segment.URI(path)
	if err != nil {
		return err
	}
	return c.file.UploadPrivateFile(ctx, userID, file, name)
}

// UploadPrivateFileWith 上传任意来源到私聊。
func (c *Client) UploadPrivateFileWith(ctx context.Context, userID types.UserID, name string, src any) error {
	file, err := segment.URI(src)
	if err != nil {
		return err
	}
	return c.file.UploadPrivateFile(ctx, userID, file, name)
}

// SendGroupImage 发送本地图片或 URL 到群聊。
func (c *Client) SendGroupImage(ctx context.Context, groupID types.GroupID, src any) (types.MessageID, error) {
	seg, err := segment.ImageFrom(src)
	if err != nil {
		return 0, err
	}
	return c.message.SendGroup(ctx, groupID, types.Message{seg})
}

// SendPrivateImage 发送本地图片或 URL 到私聊。
func (c *Client) SendPrivateImage(ctx context.Context, userID types.UserID, src any) (types.MessageID, error) {
	seg, err := segment.ImageFrom(src)
	if err != nil {
		return 0, err
	}
	return c.message.SendPrivate(ctx, userID, types.Message{seg})
}

// GroupName 是获取群名的便捷方法，失败时返回空字符串。
func (c *Client) GroupName(ctx context.Context, groupID types.GroupID) string {
	info, err := c.group.GetInfo(ctx, groupID, false)
	if err != nil || info == nil {
		return ""
	}
	return info.GroupName
}

// MemberName 返回群成员显示名（优先群名片）。
func (c *Client) MemberName(ctx context.Context, groupID types.GroupID, userID types.UserID) (string, error) {
	info, err := c.group.GetMemberInfo(ctx, groupID, userID, false)
	if err != nil {
		return "", err
	}
	if info.Card != "" {
		return info.Card, nil
	}
	if info.Nickname == "" {
		return "", fmt.Errorf("yukichoco: 成员 %s 没有可用昵称", userID)
	}
	return info.Nickname, nil
}
