// Package segment 提供 Milky 消息段的构造助手。
//
// 生成的 types 包里消息段是普通结构体（便于解码与透传）；本包在此之上提供
// 更顺手的构造方式与资源 URI 处理：
//
//	segs := []types.OutgoingSegment{
//		segment.Reply(msg.MessageSeq),
//		segment.Text("你好，"),
//		segment.Mention(msg.SenderID),
//		segment.ImageFromFile("/tmp/a.png"),
//	}
package segment

import (
	"github.com/WinterChocolates/YukiChoco/types"
)

// Text 构造文本段。
func Text(text string) types.OutgoingSegment {
	return &types.OutgoingTextSegment{Text: text}
}

// Mention 构造提及段。
func Mention(userID types.UserID) types.OutgoingSegment {
	return &types.OutgoingMentionSegment{UserID: userID}
}

// MentionAll 构造 @全体成员 段。
//
// 注意：只有管理员才能 @全体成员，否则协议端会返回错误。
func MentionAll() types.OutgoingSegment {
	return &types.OutgoingMentionAllSegment{}
}

// Face 构造 QQ 系统表情段。
func Face(faceID string) types.OutgoingSegment {
	return &types.OutgoingFaceSegment{FaceID: faceID}
}

// LargeFace 构造超级表情段。
func LargeFace(faceID string) types.OutgoingSegment {
	return &types.OutgoingFaceSegment{FaceID: faceID, IsLarge: true}
}

// Reply 构造引用（回复）段。
func Reply(seq types.MessageSeq) types.OutgoingSegment {
	return &types.OutgoingReplySegment{MessageSeq: seq}
}

// Image 构造图片段，uri 支持 file:// / http(s):// / base64://。
func Image(uri string) types.OutgoingSegment {
	return &types.OutgoingImageSegment{URI: uri}
}

// Sticker 构造贴纸图片段。
func Sticker(uri string) types.OutgoingSegment {
	return &types.OutgoingImageSegment{URI: uri, SubType: types.ImageSubTypeSticker}
}

// Record 构造语音段，uri 支持 file:// / http(s):// / base64://。
func Record(uri string) types.OutgoingSegment {
	return &types.OutgoingRecordSegment{URI: uri}
}

// Video 构造视频段，uri 支持 file:// / http(s):// / base64://。
func Video(uri string) types.OutgoingSegment {
	return &types.OutgoingVideoSegment{URI: uri}
}

// LightApp 构造小程序段。
func LightApp(jsonPayload string) types.OutgoingSegment {
	return &types.OutgoingLightAppSegment{JSONPayload: jsonPayload}
}

// Forward 构造合并转发段。
//
//	segment.Forward(
//		types.OutgoingForwardedMessage{UserID: 1, SenderName: "甲", Segments: types.OutgoingSegments{segment.Text("a")}},
//		types.OutgoingForwardedMessage{UserID: 2, SenderName: "乙", Segments: types.OutgoingSegments{segment.Text("b")}},
//	)
func Forward(messages ...types.OutgoingForwardedMessage) types.OutgoingSegment {
	return &types.OutgoingForwardSegment{Messages: messages}
}

// ForwardWithOptions 构造带预览信息的合并转发段。
func ForwardWithOptions(messages []types.OutgoingForwardedMessage, title, preview, summary, prompt string) types.OutgoingSegment {
	seg := &types.OutgoingForwardSegment{Messages: messages}
	if title != "" {
		seg.Title = &title
	}
	if preview != "" {
		seg.Preview = []string{preview}
	}
	if summary != "" {
		seg.Summary = &summary
	}
	if prompt != "" {
		seg.Prompt = &prompt
	}
	return seg
}
