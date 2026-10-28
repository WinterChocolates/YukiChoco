package YukiChoco

import (
	"context"

	"github.com/WinterChocolates/YukiChoco/types"
)

// SystemAPI 是系统类动作。
type SystemAPI struct{ c *Client }

// GetLoginInfo 获取登录号信息。
func (s *SystemAPI) GetLoginInfo(ctx context.Context) (*types.LoginInfo, error) {
	return callTyped[types.EmptyRequest, types.LoginInfo](ctx, s.c, types.ActionGetLoginInfo, &types.EmptyRequest{})
}

// GetStatus 获取运行状态。
func (s *SystemAPI) GetStatus(ctx context.Context) (*types.Status, error) {
	return callTyped[types.EmptyRequest, types.Status](ctx, s.c, types.ActionGetStatus, &types.EmptyRequest{})
}

// GetVersionInfo 获取协议端版本信息。
func (s *SystemAPI) GetVersionInfo(ctx context.Context) (*types.VersionInfo, error) {
	return callTyped[types.EmptyRequest, types.VersionInfo](ctx, s.c, types.ActionGetVersionInfo, &types.EmptyRequest{})
}

// GetOnlineClients 获取在线客户端列表（扩展）。
func (s *SystemAPI) GetOnlineClients(ctx context.Context, noCache bool) ([]types.OnlineClient, error) {
	var out []types.OnlineClient
	err := s.c.callInto(ctx, types.ActionGetOnlineClients, &types.GetOnlineClientsRequest{NoCache: noCache}, &out)
	return out, err
}

// GetStrangerInfo 获取陌生人信息。
func (s *SystemAPI) GetStrangerInfo(ctx context.Context, userID types.UserID, noCache bool) (*types.StrangerInfo, error) {
	return callTyped[types.GetStrangerInfoRequest, types.StrangerInfo](ctx, s.c, types.ActionGetStrangerInfo,
		&types.GetStrangerInfoRequest{UserID: userID, NoCache: noCache})
}

// GetCookies 获取 Cookies。
func (s *SystemAPI) GetCookies(ctx context.Context, domain string) (*types.Cookies, error) {
	return callTyped[types.GetCookiesRequest, types.Cookies](ctx, s.c, types.ActionGetCookies,
		&types.GetCookiesRequest{Domain: domain})
}

// GetCSRFToken 获取 CSRF Token。
func (s *SystemAPI) GetCSRFToken(ctx context.Context) (int64, error) {
	res, err := callTyped[types.EmptyRequest, types.GetCSRFTokenResult](ctx, s.c, types.ActionGetCSRFToken, &types.EmptyRequest{})
	if err != nil {
		return 0, err
	}
	return res.Token, nil
}

// GetCredentials 获取协议端凭证（扩展）。
func (s *SystemAPI) GetCredentials(ctx context.Context, domain string) (*types.Credentials, error) {
	return callTyped[types.GetCredentialsRequest, types.Credentials](ctx, s.c, types.ActionGetCredentials,
		&types.GetCredentialsRequest{Domain: domain})
}

// SetQQProfile 设置账号资料（扩展）。
func (s *SystemAPI) SetQQProfile(ctx context.Context, req *types.SetQQProfileRequest) error {
	return callVoid(ctx, s.c, types.ActionSetQQProfile, req)
}

// SetQQAvatar 设置账号头像（扩展）。
func (s *SystemAPI) SetQQAvatar(ctx context.Context, file string) error {
	return callVoid(ctx, s.c, types.ActionSetQQAvatar, &types.SetQQAvatarRequest{File: file})
}

// SetRestart 重启协议端。
func (s *SystemAPI) SetRestart(ctx context.Context, delay int) error {
	return callVoid(ctx, s.c, types.ActionSetRestart, &types.SetRestartRequest{Delay: delay})
}

// CanSendImage 检查协议端是否支持图片。
func (s *SystemAPI) CanSendImage(ctx context.Context) (bool, error) {
	res, err := callTyped[types.EmptyRequest, types.CanSendResult](ctx, s.c, types.ActionCanSendImage, &types.EmptyRequest{})
	if err != nil {
		return false, err
	}
	return res.Yes, nil
}

// CanSendRecord 检查协议端是否支持语音。
func (s *SystemAPI) CanSendRecord(ctx context.Context) (bool, error) {
	res, err := callTyped[types.EmptyRequest, types.CanSendResult](ctx, s.c, types.ActionCanSendRecord, &types.EmptyRequest{})
	if err != nil {
		return false, err
	}
	return res.Yes, nil
}
