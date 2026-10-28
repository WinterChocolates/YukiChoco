package YukiChoco

import (
	"context"

	"github.com/WinterChocolates/YukiChoco/types"
)

// FriendAPI 是好友类动作。
type FriendAPI struct{ c *Client }

// GetList 获取好友列表。
func (s *FriendAPI) GetList(ctx context.Context) ([]types.FriendInfo, error) {
	var out []types.FriendInfo
	err := s.c.callInto(ctx, types.ActionGetFriendList, &types.EmptyRequest{}, &out)
	return out, err
}

// SendLike 给好友点赞。
func (s *FriendAPI) SendLike(ctx context.Context, userID types.UserID, times int) error {
	return callVoid(ctx, s.c, types.ActionSendLike, &types.SendLikeRequest{UserID: userID, Times: times})
}

// SetAddRequest 处理好友请求。
func (s *FriendAPI) SetAddRequest(ctx context.Context, flag types.Flag, approve bool, remark string) error {
	return callVoid(ctx, s.c, types.ActionSetFriendAddRequest, &types.SetFriendAddRequestRequest{
		Flag: flag, Approve: approve, Remark: remark,
	})
}

// Delete 删除好友（扩展）。
func (s *FriendAPI) Delete(ctx context.Context, userID types.UserID) error {
	return callVoid(ctx, s.c, types.ActionDeleteFriend, &types.DeleteFriendRequest{UserID: userID})
}

// GetUnidirectionalList 获取单向好友列表（扩展）。
func (s *FriendAPI) GetUnidirectionalList(ctx context.Context) ([]types.UnidirectionalFriend, error) {
	var out []types.UnidirectionalFriend
	err := s.c.callInto(ctx, types.ActionGetUnidirectionalList, &types.EmptyRequest{}, &out)
	return out, err
}
