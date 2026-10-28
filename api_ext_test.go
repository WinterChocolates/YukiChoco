package YukiChoco_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	YukiChoco "github.com/WinterChocolates/YukiChoco"
	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

func TestConvenienceMethods(t *testing.T) {
	c := newHTTPClient(t, func(rw http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/")
		switch action {
		case types.ActionGetLoginInfo:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"user_id":10000,"nickname":"bot"}`)))
		case types.ActionGetVersionInfo:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"app_name":"mock","app_version":"1","protocol_version":"v11"}`)))
		case types.ActionGetGroupInfo:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"group_id":1,"group_name":"g","member_count":1,"max_member_count":2}`)))
		case types.ActionGetGroupMemberInfo:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"group_id":1,"user_id":2,"nickname":"nick","card":""}`)))
		case types.ActionGetFriendList, types.ActionGetGroupList, types.ActionGetUnidirectionalList:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `[]`)))
		default:
			_, _ = rw.Write([]byte(envelopeJSON("ok", 0, `{"message_id":42,"url":"http://x"}`)))
		}
	})
	ctx := context.Background()

	if id, err := c.SendGroupText(ctx, 1, "a"); err != nil || id != 42 {
		t.Fatalf("SendGroupText: %d %v", id, err)
	}
	if id, err := c.SendPrivateText(ctx, 2, "a"); err != nil || id != 42 {
		t.Fatalf("SendPrivateText: %d %v", id, err)
	}
	if id, err := c.SendGroupMessage(ctx, 1, segment.TextMessage("a")); err != nil || id != 42 {
		t.Fatalf("SendGroupMessage: %d %v", id, err)
	}
	if id, err := c.SendPrivateMessage(ctx, 2, segment.TextMessage("a")); err != nil || id != 42 {
		t.Fatalf("SendPrivateMessage: %d %v", id, err)
	}
	if id, err := c.ReplyGroup(ctx, 1, 9, "a"); err != nil || id != 42 {
		t.Fatalf("ReplyGroup: %d %v", id, err)
	}
	if id, err := c.ReplyPrivate(ctx, 2, 9, "a"); err != nil || id != 42 {
		t.Fatalf("ReplyPrivate: %d %v", id, err)
	}

	groupEv := &types.GroupMessageEvent{MessageEvent: types.MessageEvent{
		GroupID: 1, MessageID: 9, UserID: 2,
	}}
	if id, err := c.ReplyGroupMessage(ctx, groupEv, "a"); err != nil || id != 42 {
		t.Fatalf("ReplyGroupMessage: %d %v", id, err)
	}
	privEv := &types.PrivateMessageEvent{MessageEvent: types.MessageEvent{UserID: 2, MessageID: 9}}
	if id, err := c.ReplyPrivateMessage(ctx, privEv, "a"); err != nil || id != 42 {
		t.Fatalf("ReplyPrivateMessage: %d %v", id, err)
	}

	if id, err := c.SendGroupImage(ctx, 1, []byte("x")); err != nil || id != 42 {
		t.Fatalf("SendGroupImage: %d %v", id, err)
	}
	if id, err := c.SendPrivateImage(ctx, 2, "http://x/a.png"); err != nil || id != 42 {
		t.Fatalf("SendPrivateImage: %d %v", id, err)
	}

	if err := c.UploadGroupFileFrom(ctx, 1, ".", "a.txt"); err != nil {
		t.Fatalf("UploadGroupFileFrom: %v", err)
	}
	if err := c.UploadGroupFileWith(ctx, 1, "a.txt", []byte("x"), ""); err != nil {
		t.Fatalf("UploadGroupFileWith: %v", err)
	}
	if err := c.UploadPrivateFileFrom(ctx, 2, ".", "a.txt"); err != nil {
		t.Fatalf("UploadPrivateFileFrom: %v", err)
	}
	if err := c.UploadPrivateFileWith(ctx, 2, "a.txt", []byte("x")); err != nil {
		t.Fatalf("UploadPrivateFileWith: %v", err)
	}

	if got := c.GroupName(ctx, 1); got != "g" {
		t.Fatalf("GroupName=%q", got)
	}
	if got, err := c.MemberName(ctx, 1, 2); err != nil || got != "nick" {
		t.Fatalf("MemberName=%q %v", got, err)
	}

	if err := c.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if c.SelfID() != 10000 {
		t.Fatalf("self=%d", c.SelfID())
	}
	if _, ok := c.LoginInfo(); !ok {
		t.Fatal("LoginInfo 缺失")
	}
	if v, ok := c.VersionInfo(); !ok || v.AppName != "mock" {
		t.Fatalf("VersionInfo=%+v", v)
	}
	if friends, err := c.Friend().GetList(ctx); err != nil || len(friends) != 0 {
		t.Fatalf("friends=%v err=%v", friends, err)
	}
	if groups, err := c.Group().GetList(ctx); err != nil || len(groups) != 0 {
		t.Fatalf("groups=%v err=%v", groups, err)
	}
	if c.Router() == nil || c.GroupRouter() == nil || c.PrivateRouter() == nil || c.Events() == nil {
		t.Fatal("路由入口缺失")
	}

	var apiErr *YukiChoco.APIError
	if errors.As(&YukiChoco.APIError{Retcode: 103}, &apiErr); !errors.Is(apiErr, YukiChoco.ErrNotFound) {
		t.Fatal("APIError 归类")
	}
	if (&YukiChoco.APIError{}).IsRetryable() {
		t.Fatal("APIError 不应可重试")
	}
	if c.Config().LogValue().String() == "" {
		t.Fatal("LogValue 为空")
	}
}
