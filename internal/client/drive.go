package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

// 最大下载文件大小限制 (100MB)
const maxDownloadSize = 100 * 1024 * 1024

// 下载超时时间
const downloadTimeout = 5 * time.Minute

// UploadMedia uploads a file to Feishu drive
func UploadMedia(filePath string, parentType string, parentNode string, fileName string) (string, http.Header, error) {
	return UploadMediaWithExtra(filePath, parentType, parentNode, fileName, "")
}

// UploadMediaWithExtra uploads a file to Feishu drive with extra parameter.
// extra 为 JSON 字符串，用于指定扩展信息（如 {"drive_route_token":"documentID"}）。
func UploadMediaWithExtra(filePath, parentType, parentNode, fileName, extra string) (string, http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return "", nil, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("获取文件信息失败: %w", err)
	}
	fileSize := int(stat.Size())

	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	bodyBuilder := larkdrive.NewUploadAllMediaReqBodyBuilder().
		FileName(fileName).
		ParentType(parentType).
		ParentNode(parentNode).
		Size(fileSize).
		File(file)

	if extra != "" {
		bodyBuilder = bodyBuilder.Extra(extra)
	}

	req := larkdrive.NewUploadAllMediaReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Drive.Media.UploadAll(Context(), req)
	if err != nil {
		return "", nil, fmt.Errorf("上传素材失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return "", headers, fmt.Errorf("上传素材失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.FileToken == nil {
		return "", headers, fmt.Errorf("上传成功但未返回文件 Token")
	}

	return *resp.Data.FileToken, headers, nil
}

// DownloadMediaOptions holds optional parameters for DownloadMedia
type DownloadMediaOptions struct {
	UserAccessToken string        // User Access Token（可选）
	DocToken        string        // 文档 Token（文档内嵌图片下载时需要）
	Timeout         time.Duration // 自定义超时时间（0 表示使用默认 5 分钟）
}

// DownloadMedia downloads a file from Feishu drive
func DownloadMedia(fileToken string, outputPath string, opts ...DownloadMediaOptions) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	reqBuilder := larkdrive.NewDownloadMediaReqBuilder().
		FileToken(fileToken)

	var reqOpts []larkcore.RequestOptionFunc
	if len(opts) > 0 {
		reqOpts = UserTokenOption(opts[0].UserAccessToken)
		if opts[0].DocToken != "" {
			extraJSON, _ := json.Marshal(map[string]string{"doc_token": opts[0].DocToken, "obj_type": "docx"})
			reqBuilder = reqBuilder.Extra(string(extraJSON))
		}
	}

	t := downloadTimeout
	if len(opts) > 0 && opts[0].Timeout > 0 {
		t = opts[0].Timeout
	}

	resp, err := client.Drive.Media.Download(ContextWithTimeout(t), reqBuilder.Build(), reqOpts...)
	if err != nil {
		return fmt.Errorf("下载素材失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("下载素材失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return saveToFile(resp.File, outputPath)
}

// GetMediaTempURL gets a temporary download URL for a media file
func GetMediaTempURL(fileToken string, opts ...DownloadMediaOptions) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	reqBuilder := larkdrive.NewBatchGetTmpDownloadUrlMediaReqBuilder().
		FileTokens([]string{fileToken})

	var reqOpts []larkcore.RequestOptionFunc
	if len(opts) > 0 {
		reqOpts = UserTokenOption(opts[0].UserAccessToken)
		if opts[0].DocToken != "" {
			extraJSON, _ := json.Marshal(map[string]string{"doc_token": opts[0].DocToken, "obj_type": "docx"})
			reqBuilder = reqBuilder.Extra(string(extraJSON))
		}
	}

	resp, err := client.Drive.Media.BatchGetTmpDownloadUrl(Context(), reqBuilder.Build(), reqOpts...)
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
func DownloadFromURL(url string, outputPath string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}

	httpClient := &http.Client{
		Timeout: resolveTimeout(downloadTimeout, timeout),
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("从 URL 下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP 状态码 %d", resp.StatusCode)
	}

	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("文件超过大小限制: %d MB (限制 %d MB)",
			resp.ContentLength/(1024*1024), maxDownloadSize/(1024*1024))
	}

	return saveToFile(resp.Body, outputPath)
}

// saveToFile 将 reader 内容写入文件，限制最大大小
func saveToFile(reader io.Reader, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	limitedReader := io.LimitReader(reader, maxDownloadSize)
	written, err := io.Copy(outFile, limitedReader)
	if err != nil {
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
			files = append(files, &DriveFile{
				Token:        StringVal(f.Token),
				Name:         StringVal(f.Name),
				Type:         StringVal(f.Type),
				ParentToken:  StringVal(f.ParentToken),
				URL:          StringVal(f.Url),
				CreatedTime:  StringVal(f.CreatedTime),
				ModifiedTime: StringVal(f.ModifiedTime),
				OwnerID:      StringVal(f.OwnerId),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.NextPageToken)
		hasMore = BoolVal(resp.Data.HasMore)
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
		token = StringVal(resp.Data.Token)
		url = StringVal(resp.Data.Url)
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
		token = StringVal(resp.Data.File.Token)
		url = StringVal(resp.Data.File.Url)
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

	info := &ShortcutInfo{
		TargetToken: targetFileToken,
		TargetType:  targetType,
	}
	if resp.Data != nil && resp.Data.SuccShortcutNode != nil {
		info.Token = StringVal(resp.Data.SuccShortcutNode.Token)
		info.ParentToken = StringVal(resp.Data.SuccShortcutNode.ParentToken)
	}

	return info, nil
}

// DownloadFile 下载云空间文件
func DownloadFile(fileToken string, outputPath string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDownloadFileReqBuilder().
		FileToken(fileToken).
		Build()

	resp, err := client.Drive.File.Download(ContextWithTimeout(resolveTimeout(downloadTimeout, timeout)), req)
	if err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("下载文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return saveToFile(resp.File, outputPath)
}

// maxSingleUploadSize 单次上传的文件大小上限（20MB），超过此大小需使用分片上传
const maxSingleUploadSize = 20 * 1024 * 1024

// UploadFile 上传文件到飞书云空间，超过 20MB 自动使用分片上传
func UploadFile(filePath, parentToken, fileName string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	fileSize := int(stat.Size())

	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	if fileSize > maxSingleUploadSize {
		return uploadFileMultipart(filePath, parentToken, fileName, fileSize)
	}
	return uploadFileSingle(file, parentToken, fileName, fileSize)
}

// uploadFileSingle 单次上传文件（适用于 ≤ 20MB 的文件）
func uploadFileSingle(file *os.File, parentToken, fileName string, fileSize int) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdrive.NewUploadAllFileReqBuilder().
		Body(larkdrive.NewUploadAllFileReqBodyBuilder().
			FileName(fileName).
			ParentType("explorer").
			ParentNode(parentToken).
			Size(fileSize).
			File(file).
			Build()).
		Build()

	resp, err := client.Drive.File.UploadAll(ContextWithTimeout(downloadTimeout), req)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("上传文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.FileToken == nil {
		return "", fmt.Errorf("上传成功但未返回文件 Token")
	}

	return *resp.Data.FileToken, nil
}

// uploadFileMultipart 使用三阶段分片 API 上传大文件：
// 1. upload_prepare — 获取 upload_id、block_size、block_num
// 2. upload_part   — 逐片上传（含重试机制）
// 3. upload_finish — 完成上传并获取 file_token
func uploadFileMultipart(filePath, parentToken, fileName string, fileSize int) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	// 第一步：准备分片上传
	prepareReq := larkdrive.NewUploadPrepareFileReqBuilder().
		FileUploadInfo(larkdrive.NewFileUploadInfoBuilder().
			FileName(fileName).
			ParentType("explorer").
			ParentNode(parentToken).
			Size(fileSize).
			Build()).
		Build()

	prepareResp, err := client.Drive.File.UploadPrepare(Context(), prepareReq)
	if err != nil {
		return "", fmt.Errorf("分片上传准备失败: %w", err)
	}
	if !prepareResp.Success() {
		return "", fmt.Errorf("分片上传准备失败: code=%d, msg=%s", prepareResp.Code, prepareResp.Msg)
	}

	uploadID := StringVal(prepareResp.Data.UploadId)
	blockSize := IntVal(prepareResp.Data.BlockSize)
	blockNum := IntVal(prepareResp.Data.BlockNum)

	if uploadID == "" || blockSize <= 0 || blockNum <= 0 {
		return "", fmt.Errorf("分片上传准备返回数据异常: upload_id=%s, block_size=%d, block_num=%d", uploadID, blockSize, blockNum)
	}

	fmt.Printf("分片上传: 文件大小 %s, 分片大小 %s, 共 %d 个分片\n",
		formatSize(fileSize), formatSize(blockSize), blockNum)

	// 第二步：逐片上传（每片最多重试 3 次）
	const maxPartRetries = 3
	for seq := 0; seq < blockNum; seq++ {
		offset := int64(seq) * int64(blockSize)
		partSize := int64(blockSize)
		remaining := int64(fileSize) - offset
		if partSize > remaining {
			partSize = remaining
		}

		var lastErr error
		uploaded := false
		for attempt := 1; attempt <= maxPartRetries; attempt++ {
			partFile, err := os.Open(filePath)
			if err != nil {
				return "", fmt.Errorf("打开文件失败: %w", err)
			}
			if _, err := partFile.Seek(offset, io.SeekStart); err != nil {
				partFile.Close()
				return "", fmt.Errorf("定位文件分片 %d 失败: %w", seq, err)
			}

			partReq := larkdrive.NewUploadPartFileReqBuilder().
				Body(larkdrive.NewUploadPartFileReqBodyBuilder().
					UploadId(uploadID).
					Seq(seq).
					Size(int(partSize)).
					File(io.LimitReader(partFile, partSize)).
					Build()).
				Build()

			partResp, err := client.Drive.File.UploadPart(ContextWithTimeout(downloadTimeout), partReq)
			partFile.Close()

			if err == nil && partResp.Success() {
				uploaded = true
				break
			}

			// 记录本次错误
			if err != nil {
				lastErr = fmt.Errorf("上传分片 %d/%d 失败: %w", seq+1, blockNum, err)
			} else {
				lastErr = fmt.Errorf("上传分片 %d/%d 失败: code=%d, msg=%s", seq+1, blockNum, partResp.Code, partResp.Msg)
			}

			if attempt < maxPartRetries {
				fmt.Printf("  第 %d/%d 片上传失败，重试 (%d/%d)...\n", seq+1, blockNum, attempt, maxPartRetries)
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}

		if !uploaded {
			return "", lastErr
		}

		fmt.Printf("  分片 %d/%d 上传完成 (%s)\n", seq+1, blockNum, formatSize(int(partSize)))
	}

	// 第三步：完成上传
	finishReq := larkdrive.NewUploadFinishFileReqBuilder().
		Body(larkdrive.NewUploadFinishFileReqBodyBuilder().
			UploadId(uploadID).
			BlockNum(blockNum).
			Build()).
		Build()

	finishResp, err := client.Drive.File.UploadFinish(Context(), finishReq)
	if err != nil {
		return "", fmt.Errorf("完成分片上传失败: %w", err)
	}
	if !finishResp.Success() {
		return "", fmt.Errorf("完成分片上传失败: code=%d, msg=%s", finishResp.Code, finishResp.Msg)
	}

	if finishResp.Data == nil || finishResp.Data.FileToken == nil {
		return "", fmt.Errorf("分片上传完成但未返回文件 Token")
	}

	return *finishResp.Data.FileToken, nil
}

// formatSize 将字节数格式化为可读字符串
func formatSize(bytes int) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FileVersionInfo 文件版本信息
type FileVersionInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	ParentToken string `json:"parent_token,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	CreatorID   string `json:"creator_id,omitempty"`
	CreateTime  string `json:"create_time,omitempty"`
	UpdateTime  string `json:"update_time,omitempty"`
	Status      string `json:"status,omitempty"`
	ObjType     string `json:"obj_type,omitempty"`
	ParentType  string `json:"parent_type,omitempty"`
}

func versionToInfo(v *larkdrive.Version) *FileVersionInfo {
	if v == nil {
		return nil
	}
	return &FileVersionInfo{
		Name:        StringVal(v.Name),
		Version:     StringVal(v.Version),
		ParentToken: StringVal(v.ParentToken),
		OwnerID:     StringVal(v.OwnerId),
		CreatorID:   StringVal(v.CreatorId),
		CreateTime:  StringVal(v.CreateTime),
		UpdateTime:  StringVal(v.UpdateTime),
		Status:      StringVal(v.Status),
		ObjType:     StringVal(v.ObjType),
		ParentType:  StringVal(v.ParentType),
	}
}

// CreateFileVersion 创建文件版本
func CreateFileVersion(fileToken, objType, name string) (*FileVersionInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	version := larkdrive.NewVersionBuilder().
		Name(name).
		ObjType(objType).
		Build()

	req := larkdrive.NewCreateFileVersionReqBuilder().
		FileToken(fileToken).
		Version(version).
		Build()

	resp, err := client.Drive.FileVersion.Create(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建文件版本失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建文件版本失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("创建文件版本成功但未返回数据")
	}

	return &FileVersionInfo{
		Name:        StringVal(resp.Data.Name),
		Version:     StringVal(resp.Data.Version),
		ParentToken: StringVal(resp.Data.ParentToken),
		OwnerID:     StringVal(resp.Data.OwnerId),
		CreatorID:   StringVal(resp.Data.CreatorId),
		CreateTime:  StringVal(resp.Data.CreateTime),
		UpdateTime:  StringVal(resp.Data.UpdateTime),
		Status:      StringVal(resp.Data.Status),
	}, nil
}

// GetFileVersion 获取文件版本详情
func GetFileVersion(fileToken, versionID, objType string) (*FileVersionInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetFileVersionReqBuilder().
		FileToken(fileToken).
		VersionId(versionID).
		ObjType(objType).
		Build()

	resp, err := client.Drive.FileVersion.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取文件版本失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取文件版本失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("文件版本不存在")
	}

	return &FileVersionInfo{
		Name:        StringVal(resp.Data.Name),
		Version:     StringVal(resp.Data.Version),
		ParentToken: StringVal(resp.Data.ParentToken),
		OwnerID:     StringVal(resp.Data.OwnerId),
		CreatorID:   StringVal(resp.Data.CreatorId),
		CreateTime:  StringVal(resp.Data.CreateTime),
		UpdateTime:  StringVal(resp.Data.UpdateTime),
		Status:      StringVal(resp.Data.Status),
	}, nil
}

// ListFileVersions 列出文件版本
func ListFileVersions(fileToken, objType string, pageSize int, pageToken string) ([]*FileVersionInfo, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkdrive.NewListFileVersionReqBuilder().
		FileToken(fileToken).
		ObjType(objType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Drive.FileVersion.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取文件版本列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取文件版本列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var versions []*FileVersionInfo
	if resp.Data != nil && resp.Data.Items != nil {
		for _, v := range resp.Data.Items {
			versions = append(versions, versionToInfo(v))
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return versions, nextPageToken, hasMore, nil
}

// DeleteFileVersion 删除文件版本
func DeleteFileVersion(fileToken, versionID, objType string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDeleteFileVersionReqBuilder().
		FileToken(fileToken).
		VersionId(versionID).
		ObjType(objType).
		Build()

	resp, err := client.Drive.FileVersion.Delete(Context(), req)
	if err != nil {
		return fmt.Errorf("删除文件版本失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除文件版本失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// FileMeta 文件元数据
type FileMeta struct {
	DocToken         string `json:"doc_token"`
	DocType          string `json:"doc_type"`
	Title            string `json:"title"`
	OwnerID          string `json:"owner_id,omitempty"`
	CreateTime       string `json:"create_time,omitempty"`
	LatestModifyUser string `json:"latest_modify_user,omitempty"`
	LatestModifyTime string `json:"latest_modify_time,omitempty"`
	URL              string `json:"url,omitempty"`
}

// BatchGetMeta 批量获取文件元数据
func BatchGetMeta(docTokens []string, docType string) ([]*FileMeta, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	var requestDocs []*larkdrive.RequestDoc
	for _, token := range docTokens {
		requestDocs = append(requestDocs, larkdrive.NewRequestDocBuilder().
			DocToken(token).
			DocType(docType).
			Build())
	}

	withURL := true
	metaRequest := larkdrive.NewMetaRequestBuilder().
		RequestDocs(requestDocs).
		WithUrl(withURL).
		Build()

	req := larkdrive.NewBatchQueryMetaReqBuilder().
		MetaRequest(metaRequest).
		Build()

	resp, err := client.Drive.Meta.BatchQuery(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("批量获取文件元数据失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("批量获取文件元数据失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var metas []*FileMeta
	if resp.Data != nil && resp.Data.Metas != nil {
		for _, m := range resp.Data.Metas {
			metas = append(metas, &FileMeta{
				DocToken:         StringVal(m.DocToken),
				DocType:          StringVal(m.DocType),
				Title:            StringVal(m.Title),
				OwnerID:          StringVal(m.OwnerId),
				CreateTime:       StringVal(m.CreateTime),
				LatestModifyUser: StringVal(m.LatestModifyUser),
				LatestModifyTime: StringVal(m.LatestModifyTime),
				URL:              StringVal(m.Url),
			})
		}
	}

	return metas, nil
}

// FileStats 文件统计信息
type FileStats struct {
	FileToken      string `json:"file_token"`
	FileType       string `json:"file_type"`
	UV             int    `json:"uv"`
	PV             int    `json:"pv"`
	LikeCount      int    `json:"like_count"`
	UVToday        int    `json:"uv_today"`
	PVToday        int    `json:"pv_today"`
	LikeCountToday int    `json:"like_count_today"`
}

// GetFileStatistics 获取文件统计信息
func GetFileStatistics(fileToken, fileType string) (*FileStats, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetFileStatisticsReqBuilder().
		FileToken(fileToken).
		FileType(fileType).
		Build()

	resp, err := client.Drive.FileStatistics.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取文件统计信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取文件统计信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("获取文件统计信息返回数据为空")
	}

	stats := &FileStats{
		FileToken: StringVal(resp.Data.FileToken),
		FileType:  StringVal(resp.Data.FileType),
	}

	if resp.Data.Statistics != nil {
		stats.UV = IntVal(resp.Data.Statistics.Uv)
		stats.PV = IntVal(resp.Data.Statistics.Pv)
		stats.LikeCount = IntVal(resp.Data.Statistics.LikeCount)
		stats.UVToday = IntVal(resp.Data.Statistics.UvToday)
		stats.PVToday = IntVal(resp.Data.Statistics.PvToday)
		stats.LikeCountToday = IntVal(resp.Data.Statistics.LikeCountToday)
	}

	return stats, nil
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
