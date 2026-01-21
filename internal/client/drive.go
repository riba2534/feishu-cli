package client

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

// 最大下载文件大小限制 (100MB)
const maxDownloadSize = 100 * 1024 * 1024

// 下载超时时间
const downloadTimeout = 5 * time.Minute

// UploadMedia uploads a file to Feishu drive
func UploadMedia(filePath string, parentType string, parentNode string, fileName string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	fileSize := int(stat.Size())

	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	req := larkdrive.NewUploadAllMediaReqBuilder().
		Body(larkdrive.NewUploadAllMediaReqBodyBuilder().
			FileName(fileName).
			ParentType(parentType).
			ParentNode(parentNode).
			Size(fileSize).
			File(file).
			Build()).
		Build()

	resp, err := client.Drive.Media.UploadAll(Context(), req)
	if err != nil {
		return "", fmt.Errorf("上传素材失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("上传素材失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.FileToken == nil {
		return "", fmt.Errorf("上传成功但未返回文件 Token")
	}

	return *resp.Data.FileToken, nil
}

// DownloadMedia downloads a file from Feishu drive
func DownloadMedia(fileToken string, outputPath string) error {
	// 路径安全检查
	if err := validatePath(outputPath); err != nil {
		return err
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDownloadMediaReqBuilder().
		FileToken(fileToken).
		Build()

	resp, err := client.Drive.Media.Download(ContextWithTimeout(downloadTimeout), req)
	if err != nil {
		return fmt.Errorf("下载素材失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("下载素材失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	// 使用 LimitedReader 限制下载大小
	limitedReader := io.LimitReader(resp.File, maxDownloadSize)
	written, err := io.Copy(outFile, limitedReader)
	if err != nil {
		// 下载失败时删除未完成的文件
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("写入文件失败: %w", err)
	}

	if written >= maxDownloadSize {
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("文件超过大小限制 (%d MB)", maxDownloadSize/(1024*1024))
	}

	return nil
}

// GetMediaTempURL gets a temporary download URL for a media file
func GetMediaTempURL(fileToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdrive.NewBatchGetTmpDownloadUrlMediaReqBuilder().
		FileTokens([]string{fileToken}).
		Build()

	resp, err := client.Drive.Media.BatchGetTmpDownloadUrl(Context(), req)
	if err != nil {
		return "", fmt.Errorf("获取临时下载链接失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("获取临时下载链接失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if len(resp.Data.TmpDownloadUrls) == 0 {
		return "", fmt.Errorf("未返回下载链接")
	}

	if resp.Data.TmpDownloadUrls[0].TmpDownloadUrl == nil {
		return "", fmt.Errorf("下载链接为空")
	}

	return *resp.Data.TmpDownloadUrls[0].TmpDownloadUrl, nil
}

// DownloadFromURL downloads a file from a URL with size limit
func DownloadFromURL(url string, outputPath string) error {
	// 路径安全检查
	if err := validatePath(outputPath); err != nil {
		return err
	}

	// 创建带超时的 HTTP 客户端
	httpClient := &http.Client{
		Timeout: downloadTimeout,
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("从 URL 下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP 状态码 %d", resp.StatusCode)
	}

	// 检查 Content-Length
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("文件超过大小限制: %d MB (限制 %d MB)",
			resp.ContentLength/(1024*1024), maxDownloadSize/(1024*1024))
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	// 使用 LimitedReader 限制下载大小
	limitedReader := io.LimitReader(resp.Body, maxDownloadSize)
	written, err := io.Copy(outFile, limitedReader)
	if err != nil {
		// 下载失败时删除未完成的文件
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("写入文件失败: %w", err)
	}

	if written >= maxDownloadSize {
		outFile.Close()
		os.Remove(outputPath)
		return fmt.Errorf("文件超过大小限制 (%d MB)", maxDownloadSize/(1024*1024))
	}

	return nil
}

// validatePath 验证路径安全性，防止路径遍历攻击
func validatePath(path string) error {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("路径不安全: 不允许包含 '..'")
	}
	return nil
}

// DriveFile 云空间文件信息
type DriveFile struct {
	Token        string `json:"token"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ParentToken  string `json:"parent_token,omitempty"`
	URL          string `json:"url,omitempty"`
	CreatedTime  string `json:"created_time,omitempty"`
	ModifiedTime string `json:"modified_time,omitempty"`
	OwnerID      string `json:"owner_id,omitempty"`
}

// ListFiles 列出文件夹中的文件
func ListFiles(folderToken string, pageSize int, pageToken string) ([]*DriveFile, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkdrive.NewListFileReqBuilder()
	if folderToken != "" {
		reqBuilder.FolderToken(folderToken)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Drive.File.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取文件列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取文件列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var files []*DriveFile
	if resp.Data != nil && resp.Data.Files != nil {
		for _, f := range resp.Data.Files {
			file := &DriveFile{}
			if f.Token != nil {
				file.Token = *f.Token
			}
			if f.Name != nil {
				file.Name = *f.Name
			}
			if f.Type != nil {
				file.Type = *f.Type
			}
			if f.ParentToken != nil {
				file.ParentToken = *f.ParentToken
			}
			if f.Url != nil {
				file.URL = *f.Url
			}
			if f.CreatedTime != nil {
				file.CreatedTime = *f.CreatedTime
			}
			if f.ModifiedTime != nil {
				file.ModifiedTime = *f.ModifiedTime
			}
			if f.OwnerId != nil {
				file.OwnerID = *f.OwnerId
			}
			files = append(files, file)
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		if resp.Data.NextPageToken != nil {
			nextPageToken = *resp.Data.NextPageToken
		}
		if resp.Data.HasMore != nil {
			hasMore = *resp.Data.HasMore
		}
	}

	return files, nextPageToken, hasMore, nil
}

// CreateFolder 创建文件夹
func CreateFolder(name string, folderToken string) (string, string, error) {
	client, err := GetClient()
	if err != nil {
		return "", "", err
	}

	req := larkdrive.NewCreateFolderFileReqBuilder().
		Body(larkdrive.NewCreateFolderFileReqBodyBuilder().
			Name(name).
			FolderToken(folderToken).
			Build()).
		Build()

	resp, err := client.Drive.File.CreateFolder(Context(), req)
	if err != nil {
		return "", "", fmt.Errorf("创建文件夹失败: %w", err)
	}

	if !resp.Success() {
		return "", "", fmt.Errorf("创建文件夹失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var token, url string
	if resp.Data != nil {
		if resp.Data.Token != nil {
			token = *resp.Data.Token
		}
		if resp.Data.Url != nil {
			url = *resp.Data.Url
		}
	}

	return token, url, nil
}

// MoveFile 移动文件或文件夹
func MoveFile(fileToken string, targetFolderToken string, fileType string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdrive.NewMoveFileReqBuilder().
		Body(larkdrive.NewMoveFileReqBodyBuilder().
			Type(fileType).
			FolderToken(targetFolderToken).
			Build()).
		FileToken(fileToken).
		Build()

	resp, err := client.Drive.File.Move(Context(), req)
	if err != nil {
		return "", fmt.Errorf("移动文件失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("移动文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	// 返回任务 ID（异步操作）
	if resp.Data != nil && resp.Data.TaskId != nil {
		return *resp.Data.TaskId, nil
	}

	return "", nil
}

// CopyFile 复制文件
func CopyFile(fileToken string, targetFolderToken string, name string, fileType string) (string, string, error) {
	client, err := GetClient()
	if err != nil {
		return "", "", err
	}

	reqBuilder := larkdrive.NewCopyFileReqBodyBuilder().
		Type(fileType).
		FolderToken(targetFolderToken)

	if name != "" {
		reqBuilder.Name(name)
	}

	req := larkdrive.NewCopyFileReqBuilder().
		FileToken(fileToken).
		Body(reqBuilder.Build()).
		Build()

	resp, err := client.Drive.File.Copy(Context(), req)
	if err != nil {
		return "", "", fmt.Errorf("复制文件失败: %w", err)
	}

	if !resp.Success() {
		return "", "", fmt.Errorf("复制文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var token, url string
	if resp.Data != nil && resp.Data.File != nil {
		if resp.Data.File.Token != nil {
			token = *resp.Data.File.Token
		}
		if resp.Data.File.Url != nil {
			url = *resp.Data.File.Url
		}
	}

	return token, url, nil
}

// DeleteFile 删除文件或文件夹
func DeleteFile(fileToken string, fileType string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdrive.NewDeleteFileReqBuilder().
		FileToken(fileToken).
		Type(fileType).
		Build()

	resp, err := client.Drive.File.Delete(Context(), req)
	if err != nil {
		return "", fmt.Errorf("删除文件失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("删除文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	// 返回任务 ID（异步操作）
	if resp.Data != nil && resp.Data.TaskId != nil {
		return *resp.Data.TaskId, nil
	}

	return "", nil
}

// ShortcutInfo 快捷方式信息
type ShortcutInfo struct {
	Token       string `json:"token"`
	TargetToken string `json:"target_token"`
	TargetType  string `json:"target_type"`
	ParentToken string `json:"parent_token,omitempty"`
}

// CreateShortcut 创建文件快捷方式
func CreateShortcut(parentToken string, targetFileToken string, targetType string) (*ShortcutInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewCreateShortcutFileReqBuilder().
		Body(larkdrive.NewCreateShortcutFileReqBodyBuilder().
			ParentToken(parentToken).
			ReferEntity(larkdrive.NewReferEntityBuilder().
				ReferToken(targetFileToken).
				ReferType(targetType).
				Build()).
			Build()).
		Build()

	resp, err := client.Drive.File.CreateShortcut(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建快捷方式失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建快捷方式失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	info := &ShortcutInfo{}
	if resp.Data != nil && resp.Data.SuccShortcutNode != nil {
		if resp.Data.SuccShortcutNode.Token != nil {
			info.Token = *resp.Data.SuccShortcutNode.Token
		}
		// Note: TargetToken and TargetType fields are not available in current SDK version
		// Using Token as fallback for TargetToken
		info.TargetToken = targetFileToken
		info.TargetType = targetType
		if resp.Data.SuccShortcutNode.ParentToken != nil {
			info.ParentToken = *resp.Data.SuccShortcutNode.ParentToken
		}
	}

	return info, nil
}

// DriveQuota 云空间容量信息
type DriveQuota struct {
	Total int64 `json:"total"` // 总容量（字节）
	Used  int64 `json:"used"`  // 已用容量（字节）
}

// GetDriveQuota 获取云空间容量信息
// 注意：当前飞书 SDK 版本不支持此 API
func GetDriveQuota() (*DriveQuota, error) {
	return nil, fmt.Errorf("获取云空间容量功能暂不支持：当前 SDK 版本未提供此 API")
}
