package event

import (
	"context"
	"strings"

	"github.com/WinterChocolates/YukiChoco/types"
)

// Router 按谓词过滤事件。
//
// 谓词方法返回新的 Router，因此可以安全地从一个基础路由器派生出多条互不影响的
// 规则（都是浅拷贝 + 追加谓词）：
//
//	base := event.NewRouter(c.Events())
//	base.Where(isGroup).Handle(handlerA)
//	base.Where(isFriend).Handle(handlerB)
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
//	event.RouteGroup(c.Events()).AtMe(self).Prefix("/cmd").
//		Handle(func(ctx context.Context, msg *types.GroupMessage) error { ... })
//
// 只有 message_receive 且 message_scene=group 的事件会进入谓词判断。
type GroupRouter struct {
	d     *Dispatcher
	preds []func(*types.GroupMessage) bool
}

// RouteGroup 创建群消息路由器。
func RouteGroup(d *Dispatcher) *GroupRouter { return &GroupRouter{d: d} }

// Where 追加自定义谓词。
func (r *GroupRouter) Where(pred func(*types.GroupMessage) bool) *GroupRouter {
	return &GroupRouter{d: r.d, preds: append(append([]func(*types.GroupMessage) bool(nil), r.preds...), pred)}
}

// Group 只匹配指定群。
func (r *GroupRouter) Group(id types.GroupID) *GroupRouter {
	return r.Where(func(m *types.GroupMessage) bool { return m.Group.GroupID == id })
}

// AtMe 只匹配提及了 self 的消息。
func (r *GroupRouter) AtMe(self types.UserID) *GroupRouter {
	return r.Where(func(m *types.GroupMessage) bool { return m.Segments.Mentioned(self) })
}

// MentionsAll 只匹配 @全体成员 的消息。
func (r *GroupRouter) MentionsAll() *GroupRouter {
	return r.Where(func(m *types.GroupMessage) bool { return m.Segments.MentionsAll() })
}

// Prefix 只匹配纯文本以 prefix 开头的消息（大小写敏感）。
func (r *GroupRouter) Prefix(prefix string) *GroupRouter {
	return r.Where(func(m *types.GroupMessage) bool {
		return strings.HasPrefix(strings.TrimSpace(m.PlainText()), prefix)
	})
}

// Admin 只匹配发送者为群主或管理员的消息。
func (r *GroupRouter) Admin() *GroupRouter {
	return r.Where(func(m *types.GroupMessage) bool {
		switch m.GroupMember.Role {
		case types.GroupMemberRoleOwner, types.GroupMemberRoleAdmin:
			return true
		default:
			return false
		}
	})
}

// Handle 注册命中谓词时执行的处理函数，返回注销函数。
func (r *GroupRouter) Handle(h func(context.Context, *types.GroupMessage) error) func() {
	preds := append([]func(*types.GroupMessage) bool(nil), r.preds...)
	return r.d.SubscribeType(types.EventMessageReceive, func(ctx context.Context, ev types.Event) error {
		recv, ok := ev.(*types.MessageReceiveEvent)
		if !ok {
			return nil
		}
		msg, ok := recv.GroupMessage()
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

// HandleEvent 命中谓词后以原始事件形式回调（需要 MessageReceiveEvent 其它字段时用）。
func (r *GroupRouter) HandleEvent(h func(context.Context, *types.MessageReceiveEvent, *types.GroupMessage) error) func() {
	preds := append([]func(*types.GroupMessage) bool(nil), r.preds...)
	return r.d.SubscribeType(types.EventMessageReceive, func(ctx context.Context, ev types.Event) error {
		recv, ok := ev.(*types.MessageReceiveEvent)
		if !ok {
			return nil
		}
		msg, ok := recv.GroupMessage()
		if !ok {
			return nil
		}
		for _, p := range preds {
			if !p(msg) {
				return nil
			}
		}
		return h(ctx, recv, msg)
	})
}
