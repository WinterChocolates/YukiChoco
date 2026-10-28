package YukiChoco

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/WinterChocolates/YukiChoco/event"
	"github.com/WinterChocolates/YukiChoco/segment"
	"github.com/WinterChocolates/YukiChoco/types"
)

// Router 返回事件路由器（高级用法）。
func (c *Client) Router() *event.Router { return event.NewRouter(c.events) }

// GroupRouter 返回群消息路由器。
//
//	YukiChoco.RouteGroup? 见包级函数；这里提供从 Client 直达的入口：
//	c.GroupRouter().AtMe(c.SelfID()).Prefix("/cmd").Handle(...)
func (c *Client) GroupRouter() *event.GroupRouter { return event.RouteGroup(c.events) }

// ───────────────────────────── 文件上传 ─────────────────────────────

// UploadGroupFileFrom 上传本地文件到群文件。
func (c *Client) UploadGroupFileFrom(ctx context.Context, groupID types.GroupID, path, name string) (*types.UploadGroupFileResponse, error) {
	return c.uploadGroup(ctx, groupID, "", path, name)
}

// UploadGroupFileWith 上传任意来源（本地路径 / []byte / io.Reader / URL）到群文件。
//
// parentFolderID 为空时使用协议端默认值（根目录）。
func (c *Client) UploadGroupFileWith(ctx context.Context, groupID types.GroupID, parentFolderID, name string, src any) (*types.UploadGroupFileResponse, error) {
	return c.uploadGroup(ctx, groupID, parentFolderID, src, name)
}

func (c *Client) uploadGroup(ctx context.Context, groupID types.GroupID, parentFolderID string, src any, name string) (*types.UploadGroupFileResponse, error) {
	uri, err := segment.URI(src)
	if err != nil {
		return nil, err
	}
	return c.file.UploadGroupFile(ctx, &types.UploadGroupFileRequest{
		GroupID:        groupID,
		ParentFolderID: parentFolderID,
		FileURI:        uri,
		FileName:       name,
	})
}

// UploadPrivateFileFrom 上传本地文件到私聊。
func (c *Client) UploadPrivateFileFrom(ctx context.Context, userID types.UserID, path, name string) (*types.UploadPrivateFileResponse, error) {
	return c.uploadPrivate(ctx, userID, path, name)
}

// UploadPrivateFileWith 上传任意来源到私聊。
func (c *Client) UploadPrivateFileWith(ctx context.Context, userID types.UserID, name string, src any) (*types.UploadPrivateFileResponse, error) {
	return c.uploadPrivate(ctx, userID, src, name)
}

func (c *Client) uploadPrivate(ctx context.Context, userID types.UserID, src any, name string) (*types.UploadPrivateFileResponse, error) {
	uri, err := segment.URI(src)
	if err != nil {
		return nil, err
	}
	return c.file.UploadPrivateFile(ctx, &types.UploadPrivateFileRequest{
		UserID:   userID,
		FileURI:  uri,
		FileName: name,
	})
}

// ───────────────────────────── 资源下载 ─────────────────────────────

// DownloadResource 通过 resource_id 获取临时链接并流式下载。
//
// 调用方负责关闭返回的 ReadCloser。
func (c *Client) DownloadResource(ctx context.Context, resourceID string) (io.ReadCloser, error) {
	res, err := c.message.GetResourceTempURL(ctx, &types.GetResourceTempURLRequest{ResourceID: resourceID})
	if err != nil {
		return nil, err
	}
	if res.URL == "" {
		return nil, fmt.Errorf("milky: resource %q 没有可用链接", resourceID)
	}
	return c.openStream(ctx, res.URL)
}

// DownloadFile 通过 file_id 获取临时链接并流式下载。
//
// groupID 为零表示私聊文件（需要 fileHash）。
func (c *Client) DownloadFile(ctx context.Context, groupID types.GroupID, fileID, fileHash string) (io.ReadCloser, error) {
	var link string
	if groupID == 0 {
		res, derr := c.file.GetPrivateFileDownloadURL(ctx, &types.GetPrivateFileDownloadURLRequest{
			UserID:   c.SelfID(),
			FileID:   fileID,
			FileHash: fileHash,
		})
		if derr != nil {
			return nil, derr
		}
		link = res.DownloadURL
	} else {
		res, derr := c.file.GetGroupFileDownloadURL(ctx, &types.GetGroupFileDownloadURLRequest{
			GroupID: groupID,
			FileID:  fileID,
		})
		if derr != nil {
			return nil, derr
		}
		link = res.DownloadURL
	}
	if link == "" {
		return nil, fmt.Errorf("milky: file %q 没有可用链接", fileID)
	}
	return c.openStream(ctx, link)
}

// openStream 发起 GET 并返回响应体。
func (c *Client) openStream(ctx context.Context, link string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, fmt.Errorf("milky: 构造下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("milky: 下载失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: 下载返回 HTTP %d", ErrUnexpectedStatus, resp.StatusCode)
	}
	return resp.Body, nil
}
