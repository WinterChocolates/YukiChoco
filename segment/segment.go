// Package segment 提供 OneBot v11 消息段的构造助手。
//
//	msg := types.Message{
//		segment.Reply(ev.MessageID),
//		segment.At(ev.UserID),
//		segment.Text(" 你好"),
//		segment.Image("/tmp/a.png"),
//	}
//
// 构造器只负责生成段本身；发送由各 service 完成。
package segment

import (
	"fmt"

	"github.com/WinterChocolates/YukiChoco/types"
)

// Text 构造文本段。
func Text(text string) types.Segment {
	return types.NewSegment(types.SegmentText, map[string]any{"text": text})
}

// At 构造 @某人 段。
func At(userID types.UserID) types.Segment {
	return types.NewSegment(types.SegmentAt, map[string]any{"qq": userID.String()})
}

// AtAll 构造 @全体成员 段。
//
// 只有管理员/群主可用，且受剩余次数限制。
func AtAll() types.Segment {
	return types.NewSegment(types.SegmentAt, map[string]any{"qq": "all"})
}

// Face 构造 QQ 表情段。
func Face(id string) types.Segment {
	return types.NewSegment(types.SegmentFace, map[string]any{"id": id})
}

// MFace 构造商城表情段（扩展）。
func MFace(emojiID, summary string) types.Segment {
	data := map[string]any{"emoji_id": emojiID}
	if summary != "" {
		data["summary"] = summary
	}
	return types.NewSegment(types.SegmentMFace, data)
}

// Image 构造图片段。
//
// file 可以是本地路径、http(s) URL、base64://，或协议端返回的 file 标识。
func Image(file string) types.Segment {
	return types.NewSegment(types.SegmentImage, map[string]any{"file": file})
}

// ImageWithSubType 构造带子类型的图片段（例如 "1" 表示表情包）。
func ImageWithSubType(file, subType string) types.Segment {
	return types.NewSegment(types.SegmentImage, map[string]any{"file": file, "sub_type": subType})
}

// Record 构造语音段。
func Record(file string) types.Segment {
	return types.NewSegment(types.SegmentRecord, map[string]any{"file": file})
}

// Video 构造视频段。
func Video(file string) types.Segment {
	return types.NewSegment(types.SegmentVideo, map[string]any{"file": file})
}

// Reply 构造引用回复段。
func Reply(messageID types.MessageID) types.Segment {
	return types.NewSegment(types.SegmentReply, map[string]any{"id": messageID.String()})
}

// Forward 构造合并转发段（引用已存在的转发消息）。
func Forward(id string) types.Segment {
	return types.NewSegment(types.SegmentForward, map[string]any{"id": id})
}

// Node 构造合并转发节点。
//
// 用于 send_group_forward_msg / send_private_forward_msg 的 messages 参数。
func Node(userID types.UserID, nickname string, content ...types.Segment) types.Segment {
	return types.NewSegment(types.SegmentNode, map[string]any{
		"user_id":  userID.String(),
		"nickname": nickname,
		"content":  content,
	})
}

// NodeWithTime 构造带时间戳的合并转发节点。
func NodeWithTime(userID types.UserID, nickname string, unixTime int64, content ...types.Segment) types.Segment {
	return types.NewSegment(types.SegmentNode, map[string]any{
		"user_id":  userID.String(),
		"nickname": nickname,
		"time":     unixTime,
		"content":  content,
	})
}

// Poke 构造戳一戳段。
func Poke(userID types.UserID) types.Segment {
	return types.NewSegment(types.SegmentPoke, map[string]any{"qq": userID.String()})
}

// Dice 构造掷骰子段。
func Dice() types.Segment {
	return types.NewSegment(types.SegmentDice, nil)
}

// RPS 构造猜拳段。
func RPS() types.Segment {
	return types.NewSegment(types.SegmentRPS, nil)
}

// Shake 构造窗口抖动段。
func Shake() types.Segment {
	return types.NewSegment(types.SegmentShake, nil)
}

// JSON 构造小程序 JSON 卡片段。
func JSON(payload string) types.Segment {
	return types.NewSegment(types.SegmentJSON, map[string]any{"data": payload})
}

// XML 构造 XML 卡片段。
func XML(payload string) types.Segment {
	return types.NewSegment(types.SegmentXML, map[string]any{"data": payload})
}

// Card 构造群名片分享段。
func Card(card string) types.Segment {
	return types.NewSegment(types.SegmentCard, map[string]any{"card": card})
}

// Music 构造音乐分享段（type 通常为 qq / 163 / kugou / xm）。
func Music(musicType, id string) types.Segment {
	data := map[string]any{"type": musicType}
	if id != "" {
		data["id"] = id
	}
	return types.NewSegment(types.SegmentMusic, data)
}

// CustomMusic 构造自定义音乐分享段。
func CustomMusic(url, audio, title, image string) types.Segment {
	return types.NewSegment(types.SegmentMusic, map[string]any{
		"type":  "custom",
		"url":   url,
		"audio": audio,
		"title": title,
		"image": image,
	})
}

// Location 构造位置分享段。
func Location(lat, lon, title, content string) types.Segment {
	return types.NewSegment(types.SegmentLocation, map[string]any{
		"lat":     lat,
		"lon":     lon,
		"title":   title,
		"content": content,
	})
}

// Contact 构造推荐好友/群段（type 为 friend 或 group）。
func Contact(contactType string, id types.UserID) types.Segment {
	return types.NewSegment(types.SegmentContact, map[string]any{
		"type": contactType,
		"id":   id.String(),
	})
}

// Share 构造链接分享段。
func Share(url, title, content, image string) types.Segment {
	return types.NewSegment(types.SegmentShare, map[string]any{
		"url":     url,
		"title":   title,
		"content": content,
		"image":   image,
	})
}

// Anonymous 构造匿名消息段。
func Anonymous(ignore bool) types.Segment {
	v := "0"
	if ignore {
		v = "1"
	}
	return types.NewSegment(types.SegmentAnonymous, map[string]any{"ignore": v})
}

// File 构造文件段。
func File(name, file string) types.Segment {
	return types.NewSegment(types.SegmentFile, map[string]any{"name": name, "file": file})
}

// TTS 构造文本转语音段。
func TTS(text string) types.Segment {
	return types.NewSegment(types.SegmentTTS, map[string]any{"text": text})
}

// Markdown 构造 Markdown 段（扩展，NapCat / Lagrange 支持）。
func Markdown(content string) types.Segment {
	return types.NewSegment(types.SegmentMarkdown, map[string]any{"content": content})
}

// Message 把若干段组合成一条消息。
func Message(segs ...types.Segment) types.Message { return types.Message(segs) }

// TextMessage 构造纯文本消息。
func TextMessage(text string) types.Message { return types.Message{Text(text)} }

// MustImage 与 Image 相同，但接受任意来源（路径/URL/字节/Reader），失败时 panic。
//
// 仅建议在初始化阶段或测试中使用；生产代码请用 ImageFrom。
func MustImage(src any) types.Segment {
	seg, err := ImageFrom(src)
	if err != nil {
		panic(fmt.Sprintf("segment: 构造图片失败: %v", err))
	}
	return seg
}

// ImageFrom 把任意来源转成图片段（本地路径 / URL / base64:// / []byte / io.Reader）。
func ImageFrom(src any) (types.Segment, error) {
	uri, err := URI(src)
	if err != nil {
		return types.Segment{}, err
	}
	return Image(uri), nil
}

// RecordFrom 把任意来源转成语音段。
func RecordFrom(src any) (types.Segment, error) {
	uri, err := URI(src)
	if err != nil {
		return types.Segment{}, err
	}
	return Record(uri), nil
}

// VideoFrom 把任意来源转成视频段。
func VideoFrom(src any) (types.Segment, error) {
	uri, err := URI(src)
	if err != nil {
		return types.Segment{}, err
	}
	return Video(uri), nil
}
