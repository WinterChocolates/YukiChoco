package YukiChoco

import (
	"context"
	"fmt"

	"github.com/WinterChocolates/YukiChoco/types"
)

// FileAPI 是文件类动作。
type FileAPI struct{ c *Client }

// UploadPrivateFile 上传私聊文件。
//
// file 可以是协议端可访问的本地路径、http(s) URL 或 base64://。
func (s *FileAPI) UploadPrivateFile(ctx context.Context, userID types.UserID, file, name string) error {
	return callVoid(ctx, s.c, types.ActionUploadPrivateFile, &types.UploadPrivateFileRequest{
		UserID: userID, File: file, Name: name,
	})
}

// GetPrivateFileURL 获取私聊文件下载链接。
func (s *FileAPI) GetPrivateFileURL(ctx context.Context, fileID string) (string, error) {
	res, err := callTyped[types.GetPrivateFileURLRequest, types.FileURLResult](ctx, s.c, types.ActionGetPrivateFileURL,
		&types.GetPrivateFileURLRequest{FileID: fileID})
	if err != nil {
		return "", err
	}
	return res.URL, nil
}

// DownloadFile 让协议端下载文件并返回其本地路径（go-cqhttp 扩展）。
func (s *FileAPI) DownloadFile(ctx context.Context, url string, threadCount int, headers string) (string, error) {
	var out string
	err := s.c.callInto(ctx, types.ActionDownloadFile, &types.DownloadFileRequest{
		URL: url, ThreadCount: threadCount, Headers: headers,
	}, &out)
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", fmt.Errorf("yukichoco: download_file 未返回文件路径")
	}
	return out, nil
}
