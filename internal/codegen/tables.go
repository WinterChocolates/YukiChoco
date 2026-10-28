package main

import (
	"fmt"
	"sort"
)

// idTypes 把 dataType=uin 的字段映射到语义化的 Go ID 类型，避免群号/QQ 号串用。
var idTypes = map[string]string{
	"uin":             "UserID",
	"user_id":         "UserID",
	"sender_id":       "UserID",
	"initiator_id":    "UserID",
	"target_user_id":  "UserID",
	"operator_id":     "UserID",
	"invitor_id":      "UserID",
	"uploader_id":     "UserID",
	"creator_id":      "UserID",
	"receiver_id":     "UserID",
	"self_id":         "UserID",
	"group_id":        "GroupID",
	"source_group_id": "GroupID",
	"peer_id":         "Uin",
}

// seqTypes 把序列号字段映射到 MessageSeq。
var seqTypes = map[string]string{
	"message_seq":       "MessageSeq",
	"start_message_seq": "MessageSeq",
	"next_message_seq":  "MessageSeq",
}

// enumSpec 描述 IR 中以字段值域定义的枚举（IR 本身不给枚举命名）。
type enumSpec struct {
	TypeName string
	Desc     string
	Values   []string
}

// enumByField 以字段名为键。新增字段若不在表中，生成器会直接报错，
// 迫使作者在协议升级时显式确认命名，避免悄悄生成 State/Sex 这类过泛的类型名。
var enumByField = map[string]enumSpec{
	"message_scene": {
		TypeName: "MessageScene",
		Desc:     "消息场景",
		Values:   []string{"friend", "group", "temp"},
	},
	"reaction_type": {
		TypeName: "ReactionType",
		Desc:     "表情回应类型",
		Values:   []string{"face", "emoji"},
	},
	"sex": {
		TypeName: "Sex",
		Desc:     "用户性别",
		Values:   []string{"male", "female", "unknown"},
	},
	"role": {
		TypeName: "GroupMemberRole",
		Desc:     "群成员权限等级",
		Values:   []string{"owner", "admin", "member"},
	},
	"state": {
		TypeName: "RequestState",
		Desc:     "请求状态",
		Values:   []string{"pending", "accepted", "rejected", "ignored"},
	},
	"sub_type": {
		TypeName: "ImageSubType",
		Desc:     "图片类型",
		Values:   []string{"normal", "sticker"},
	},
	"qq_protocol_type": {
		TypeName: "QQProtocolType",
		Desc:     "协议端使用的 QQ 协议平台",
		Values:   []string{"windows", "linux", "macos", "android_pad", "android_phone", "ipad", "iphone", "harmony", "watch"},
	},
	"notification_type": {
		TypeName: "NotificationType",
		Desc:     "群通知类型（用于处理入群请求）",
		Values:   []string{"join_request", "invited_join_request"},
	},
}

// endpointSpec 描述一个端点在 SDK 中的位置：归属哪个 service、方法名叫什么。
//
// 方法名不直接从 endpoint 机械转换：群聊分类下 send_group_message 更适合叫
// Group().SendMessage()，而消息分类下需要保留 SendGroupMessage 以区分私聊。
type endpointSpec struct {
	Service string
	Method  string
}

// serviceOrder 决定 service 在接口聚合中的顺序。
var serviceOrder = []string{"System", "Message", "Friend", "Group", "File"}

// endpoints 覆盖 Milky 1.3 全部 65 个端点。
var endpoints = map[string]endpointSpec{
	// 系统 API
	"get_login_info":           {"System", "GetLoginInfo"},
	"get_impl_info":            {"System", "GetImplInfo"},
	"get_user_profile":         {"System", "GetUserProfile"},
	"get_friend_list":          {"System", "GetFriendList"},
	"get_friend_info":          {"System", "GetFriendInfo"},
	"get_group_list":           {"System", "GetGroupList"},
	"get_group_info":           {"System", "GetGroupInfo"},
	"get_group_member_list":    {"System", "GetGroupMemberList"},
	"get_group_member_info":    {"System", "GetGroupMemberInfo"},
	"get_peer_pins":            {"System", "GetPeerPins"},
	"set_peer_pin":             {"System", "SetPeerPin"},
	"set_avatar":               {"System", "SetAvatar"},
	"set_nickname":             {"System", "SetNickname"},
	"set_bio":                  {"System", "SetBio"},
	"get_custom_face_url_list": {"System", "GetCustomFaceURLList"},
	"get_cookies":              {"System", "GetCookies"},
	"get_csrf_token":           {"System", "GetCSRFToken"},

	// 消息 API
	"send_private_message":   {"Message", "SendPrivateMessage"},
	"send_group_message":     {"Message", "SendGroupMessage"},
	"recall_private_message": {"Message", "RecallPrivateMessage"},
	"recall_group_message":   {"Message", "RecallGroupMessage"},
	"get_message":            {"Message", "GetMessage"},
	"get_history_messages":   {"Message", "GetHistoryMessages"},
	"get_resource_temp_url":  {"Message", "GetResourceTempURL"},
	"get_forwarded_messages": {"Message", "GetForwardedMessages"},
	"mark_message_as_read":   {"Message", "MarkMessageAsRead"},

	// 好友 API
	"send_friend_nudge":     {"Friend", "SendFriendNudge"},
	"send_profile_like":     {"Friend", "SendProfileLike"},
	"delete_friend":         {"Friend", "DeleteFriend"},
	"get_friend_requests":   {"Friend", "GetFriendRequests"},
	"accept_friend_request": {"Friend", "AcceptFriendRequest"},
	"reject_friend_request": {"Friend", "RejectFriendRequest"},

	// 群聊 API（service 内部省略 group 前缀）
	"set_group_name":                 {"Group", "SetName"},
	"set_group_avatar":               {"Group", "SetAvatar"},
	"set_group_member_card":          {"Group", "SetMemberCard"},
	"set_group_member_special_title": {"Group", "SetMemberSpecialTitle"},
	"set_group_member_admin":         {"Group", "SetMemberAdmin"},
	"set_group_member_mute":          {"Group", "SetMemberMute"},
	"set_group_whole_mute":           {"Group", "SetWholeMute"},
	"kick_group_member":              {"Group", "KickMember"},
	"get_group_announcements":        {"Group", "GetAnnouncements"},
	"send_group_announcement":        {"Group", "SendAnnouncement"},
	"delete_group_announcement":      {"Group", "DeleteAnnouncement"},
	"get_group_essence_messages":     {"Group", "GetEssenceMessages"},
	"set_group_essence_message":      {"Group", "SetEssenceMessage"},
	"quit_group":                     {"Group", "Quit"},
	"send_group_message_reaction":    {"Group", "SendMessageReaction"},
	"send_group_nudge":               {"Group", "SendNudge"},
	"get_group_notifications":        {"Group", "GetNotifications"},
	"accept_group_request":           {"Group", "AcceptRequest"},
	"reject_group_request":           {"Group", "RejectRequest"},
	"accept_group_invitation":        {"Group", "AcceptInvitation"},
	"reject_group_invitation":        {"Group", "RejectInvitation"},

	// 文件 API
	"upload_private_file":           {"File", "UploadPrivateFile"},
	"upload_group_file":             {"File", "UploadGroupFile"},
	"get_private_file_download_url": {"File", "GetPrivateFileDownloadURL"},
	"get_group_file_download_url":   {"File", "GetGroupFileDownloadURL"},
	"get_group_files":               {"File", "GetGroupFiles"},
	"move_group_file":               {"File", "MoveGroupFile"},
	"rename_group_file":             {"File", "RenameGroupFile"},
	"delete_group_file":             {"File", "DeleteGroupFile"},
	"persist_group_file":            {"File", "PersistGroupFile"},
	"create_group_folder":           {"File", "CreateGroupFolder"},
	"rename_group_folder":           {"File", "RenameGroupFolder"},
	"delete_group_folder":           {"File", "DeleteGroupFolder"},
}

// validate 校验 IR 与命名表的一致性，任何漂移都必须让生成失败而不是生成可疑代码。
func (g *gen) validate() error {
	// 1. 端点表必须与 IR 完全一致。
	inIR := map[string]bool{}
	for _, api := range g.ir.APIList() {
		inIR[api.Endpoint] = true
		if _, ok := endpoints[api.Endpoint]; !ok {
			return fmt.Errorf("端点 %q 未在 internal/codegen/tables.go 的 endpoints 中登记，请补充命名", api.Endpoint)
		}
	}
	for ep := range endpoints {
		if !inIR[ep] {
			return fmt.Errorf("endpoints 中登记的 %q 在 IR 中不存在，协议可能已删除该端点", ep)
		}
	}

	// 2. 同一 service 内方法名不得重复。
	seenMethod := map[string]bool{}
	for ep, spec := range endpoints {
		key := spec.Service + "." + spec.Method
		if seenMethod[key] {
			return fmt.Errorf("service 方法名重复: %s（端点 %s）", key, ep)
		}
		seenMethod[key] = true
	}

	// 3. 枚举字段必须有命名，且值域与 IR 一致。
	checkField := func(where string, f Field) error {
		if f.FieldType != "enum" {
			return nil
		}
		spec, ok := enumByField[f.Name]
		if !ok {
			return fmt.Errorf("%s.%s: 枚举字段 %q 未在 enumByField 中命名", where, f.Name, f.Name)
		}
		if len(spec.Values) != len(f.Values) {
			return fmt.Errorf("%s.%s: 枚举 %q 值域变化（IR=%v, 表=%v）", where, f.Name, f.Name, f.Values, spec.Values)
		}
		for i := range spec.Values {
			if spec.Values[i] != f.Values[i] {
				return fmt.Errorf("%s.%s: 枚举 %q 值域变化（IR=%v, 表=%v）", where, f.Name, f.Name, f.Values, spec.Values)
			}
		}
		return nil
	}

	var checkFields func(where string, fs []Field) error
	checkFields = func(where string, fs []Field) error {
		for _, f := range fs {
			if err := checkField(where, f); err != nil {
				return err
			}
			if f.FieldType == "ref" {
				if _, ok := g.ir.FindStruct(f.RefStructName); !ok {
					return fmt.Errorf("%s.%s: 引用了未知结构体 %q", where, f.Name, f.RefStructName)
				}
			}
		}
		return nil
	}

	for _, s := range g.ir.CommonStructs {
		if s.StructType == "simple" {
			if err := checkFields(s.Name, s.Fields); err != nil {
				return err
			}
			continue
		}
		if err := checkFields(s.Name, s.BaseFields); err != nil {
			return err
		}
		for _, v := range s.Variants() {
			if err := checkFields(s.Name+":"+v.TagValue, v.Fields); err != nil {
				return err
			}
		}
	}
	for _, api := range g.ir.APIList() {
		if err := checkFields("req:"+api.Endpoint, api.RequestFields); err != nil {
			return err
		}
		if err := checkFields("resp:"+api.Endpoint, api.ResponseFields); err != nil {
			return err
		}
	}

	// 4. 生成名去重（不同联合类型的同名分支会撞车）。
	names := map[string]string{}
	claim := func(name, owner string) error {
		if prev, ok := names[name]; ok {
			return fmt.Errorf("生成类型名冲突: %s 同时来自 %s 和 %s", name, prev, owner)
		}
		names[name] = owner
		return nil
	}
	for i := range g.ir.CommonStructs {
		s := &g.ir.CommonStructs[i]
		if err := claim(s.Name, "公共结构体"); err != nil {
			return err
		}
		for _, v := range s.Variants() {
			if err := claim(g.variantTypeName(s, v), s.Name+":"+v.TagValue); err != nil {
				return err
			}
		}
	}
	enums := map[string]bool{}
	for _, spec := range enumByField {
		enums[spec.TypeName] = true
	}
	for n := range enums {
		if err := claim(n, "枚举"); err != nil {
			return err
		}
	}
	return nil
}

// sortedEnumNames 返回稳定顺序的枚举类型名，保证生成物可复现。
func sortedEnumNames() []string {
	out := make([]string, 0, len(enumByField))
	for _, spec := range enumByField {
		out = append(out, spec.TypeName)
	}
	sort.Strings(out)
	return out
}
