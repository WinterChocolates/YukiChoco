package event

import (
	"context"
	"regexp"
	"strings"

	"github.com/WinterChocolates/YukiChoco/types"
)

// Router 按谓词过滤任意事件。
type Router struct {
	d     *Dispatcher
	preds []func(types.Event) bool
}

// NewRouter 创建事件路由器。
func NewRouter(d *Dispatcher) *Router { return &Router{d: d} }

// Where 追加自定义谓词。
func (r *Router) Where(pred func(types.Event) bool) *Router {
	next := &Router{d: r.d, preds: append(append([]func(types.Event) bool(nil), r.preds...), pred)}
	return next
}

// Type 只匹配指定事件类型。
func (r *Router) Type(t types.EventType) *Router {
	return r.Where(func(ev types.Event) bool { return ev.EventType() == t })
}

// Handle 注册命中谓词时执行的处理函数，返回注销函数。
func (r *Router) Handle(h func(context.Context, types.Event) error) func() {
	preds := append([]func(types.Event) bool(nil), r.preds...)
	return r.d.SubscribeAll(func(ctx context.Context, ev types.Event) error {
		for _, p := range preds {
			if !p(ev) {
				return nil
			}
		}
		return h(ctx, ev)
	})
}

// GroupRouter 是针对群消息的谓词路由器。
//
//	event.RouteGroup(d).AtMe(self).Prefix("/cmd").
//		Handle(func(ctx context.Context, msg *types.GroupMessageEvent) error { ... })
type GroupRouter struct {
	d     *Dispatcher
	preds []func(*types.GroupMessageEvent) bool
}

// RouteGroup 创建群消息路由器。
func RouteGroup(d *Dispatcher) *GroupRouter { return &GroupRouter{d: d} }

// Where 追加自定义谓词。
func (r *GroupRouter) Where(pred func(*types.GroupMessageEvent) bool) *GroupRouter {
	return &GroupRouter{d: r.d, preds: append(append([]func(*types.GroupMessageEvent) bool(nil), r.preds...), pred)}
}

// Group 只匹配指定群。
func (r *GroupRouter) Group(id types.GroupID) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.GroupID == id })
}

// From 只匹配指定发送者。
func (r *GroupRouter) From(userID types.UserID) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.UserID == userID })
}

// AtMe 只匹配 @ 了 self 的消息。
func (r *GroupRouter) AtMe(self types.UserID) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.AtMe(self) })
}

// AtAll 只匹配 @全体成员 的消息。
func (r *GroupRouter) AtAll() *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.AtAll() })
}

// Prefix 只匹配文本以 prefix 开头的消息（比较前会 TrimSpace）。
func (r *GroupRouter) Prefix(prefix string) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool {
		return strings.HasPrefix(strings.TrimSpace(m.PlainText()), prefix)
	})
}

// Suffix 只匹配文本以 suffix 结尾的消息。
func (r *GroupRouter) Suffix(suffix string) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool {
		return strings.HasSuffix(strings.TrimSpace(m.PlainText()), suffix)
	})
}

// Contains 只匹配文本包含 substr 的消息。
func (r *GroupRouter) Contains(substr string) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool {
		return strings.Contains(m.PlainText(), substr)
	})
}

// Regex 只匹配文本满足正则的消息。
func (r *GroupRouter) Regex(re *regexp.Regexp) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return re.MatchString(m.PlainText()) })
}

// Role 只匹配指定群成员角色（owner/admin/member）。
func (r *GroupRouter) Role(role types.SenderRole) *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.Sender.Role == role })
}

// Admin 只匹配管理员或群主。
func (r *GroupRouter) Admin() *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool {
		return m.Sender.Role == types.RoleAdmin || m.Sender.Role == types.RoleOwner
	})
}

// Owner 只匹配群主。
func (r *GroupRouter) Owner() *GroupRouter {
	return r.Where(func(m *types.GroupMessageEvent) bool { return m.Sender.Role == types.RoleOwner })
}

// Handle 注册群消息处理函数，返回注销函数。
func (r *GroupRouter) Handle(h func(context.Context, *types.GroupMessageEvent) error) func() {
	preds := append([]func(*types.GroupMessageEvent) bool(nil), r.preds...)
	return r.d.SubscribeType(types.EventTypeGroupMessage, func(ctx context.Context, ev types.Event) error {
		msg, ok := ev.(*types.GroupMessageEvent)
		if !ok {
			return nil
		}
		for _, p := range preds {
			if !p(msg) {
				return nil
			}
		}
		return h(ctx, msg)
	})
}

// PrivateRouter 是针对私聊消息的谓词路由器。
type PrivateRouter struct {
	d     *Dispatcher
	preds []func(*types.PrivateMessageEvent) bool
}

// RoutePrivate 创建私聊消息路由器。
func RoutePrivate(d *Dispatcher) *PrivateRouter { return &PrivateRouter{d: d} }

// Where 追加自定义谓词。
func (r *PrivateRouter) Where(pred func(*types.PrivateMessageEvent) bool) *PrivateRouter {
	return &PrivateRouter{d: r.d, preds: append(append([]func(*types.PrivateMessageEvent) bool(nil), r.preds...), pred)}
}

// From 只匹配指定发送者。
func (r *PrivateRouter) From(userID types.UserID) *PrivateRouter {
	return r.Where(func(m *types.PrivateMessageEvent) bool { return m.UserID == userID })
}

// Prefix 只匹配文本以 prefix 开头的消息。
func (r *PrivateRouter) Prefix(prefix string) *PrivateRouter {
	return r.Where(func(m *types.PrivateMessageEvent) bool {
		return strings.HasPrefix(strings.TrimSpace(m.PlainText()), prefix)
	})
}

// Contains 只匹配文本包含 substr 的消息。
func (r *PrivateRouter) Contains(substr string) *PrivateRouter {
	return r.Where(func(m *types.PrivateMessageEvent) bool { return strings.Contains(m.PlainText(), substr) })
}

// Regex 只匹配文本满足正则的消息。
func (r *PrivateRouter) Regex(re *regexp.Regexp) *PrivateRouter {
	return r.Where(func(m *types.PrivateMessageEvent) bool { return re.MatchString(m.PlainText()) })
}

// Handle 注册私聊消息处理函数，返回注销函数。
func (r *PrivateRouter) Handle(h func(context.Context, *types.PrivateMessageEvent) error) func() {
	preds := append([]func(*types.PrivateMessageEvent) bool(nil), r.preds...)
	return r.d.SubscribeType(types.EventTypePrivateMessage, func(ctx context.Context, ev types.Event) error {
		msg, ok := ev.(*types.PrivateMessageEvent)
		if !ok {
			return nil
		}
		for _, p := range preds {
			if !p(msg) {
				return nil
			}
		}
		return h(ctx, msg)
	})
}
