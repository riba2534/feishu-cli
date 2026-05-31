package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	"github.com/riba2534/feishu-cli/internal/config"
)

// 最大下载文件大小限制 (100MB)
const maxDownloadSize = 100 * 1024 * 1024

// 下载超时时间
const downloadTimeout = 5 * time.Minute

// UploadMedia uploads a file to Feishu drive
func UploadMedia(filePath string, parentType string, parentNode string, fileName string, userAccessToken ...string) (string, http.Header, error) {
	return UploadMediaWithExtra(filePath, parentType, parentNode, fileName, "", firstString(userAccessToken))
}

// UploadMediaForImport 通过 medias/upload_all 上传临时媒体用于 drive import
// parent_type 固定为 ccm_import_open，extra 携带 obj_type 和 file_extension
// 官方实现：/open-apis/drive/v1/medias/upload_all，不会在用户云盘留下中间文件
func UploadMediaForImport(filePath, fileName, objType, fileExtension, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

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

	extraJSON, err := json.Marshal(map[string]string{
		"obj_type":       objType,
		"file_extension": fileExtension,
	})
	if err != nil {
		return "", fmt.Errorf("构造 extra 字段失败: %w", err)
	}

	body := larkdrive.NewUploadAllMediaReqBodyBuilder().
		FileName(fileName).
		ParentType("ccm_import_open").
		ParentNode("ccm_import_open").
		Size(fileSize).
		Extra(string(extraJSON)).
		File(file).
		Build()

	req := larkdrive.NewUploadAllMediaReqBuilder().Body(body).Build()

	resp, err := client.Drive.Media.UploadAll(ContextWithTimeout(downloadTimeout), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("上传导入媒体失败: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("上传导入媒体失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.FileToken == nil {
		return "", fmt.Errorf("上传导入媒体成功但未返回 file_token")
	}
	return *resp.Data.FileToken, nil
}

// UploadMediaWithExtra uploads a file to Feishu drive with extra parameter.
// extra 为 JSON 字符串，用于指定扩展信息（如 {"drive_route_token":"documentID"}）。
func UploadMediaWithExtra(filePath, parentType, parentNode, fileName, extra string, userAccessToken ...string) (string, http.Header, error) {
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

	resp, err := client.Drive.Media.UploadAll(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
	DocType         string        // 文档类型（默认 docx）
	Extra           string        // 原始 extra JSON，设置后优先于 DocToken/DocType
	Timeout         time.Duration // 自定义超时时间（0 表示使用默认 5 分钟）
}

func buildDownloadMediaExtra(opts DownloadMediaOptions) string {
	if opts.Extra != "" {
		return opts.Extra
	}
	if opts.DocToken == "" {
		return ""
	}
	docType := opts.DocType
	if docType == "" {
		docType = "docx"
	}
	extraJSON, _ := json.Marshal(map[string]string{
		"doc_token": opts.DocToken,
		"doc_type":  docType,
	})
	return string(extraJSON)
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
		if extra := buildDownloadMediaExtra(opts[0]); extra != "" {
			reqBuilder = reqBuilder.Extra(extra)
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
		if extra := buildDownloadMediaExtra(opts[0]); extra != "" {
			reqBuilder = reqBuilder.Extra(extra)
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
func ListFiles(folderToken string, pageSize int, pageToken string, userAccessToken ...string) ([]*DriveFile, string, bool, error) {
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

	resp, err := client.Drive.File.List(Context(), reqBuilder.Build(), UserTokenOption(firstString(userAccessToken))...)
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

// DriveRemoteEntry 是 ListFolderRecursive 返回的单个云盘条目。
// 与 DriveFile 相比，附带递归基础上的 RelPath（用 "/" 分隔）。
type DriveRemoteEntry struct {
	FileToken string
	Type      string // file / folder / docx / sheet / bitable / mindnote / slides / shortcut
	RelPath   string
}

// ListFolderRecursive 递归列出 folderToken 下的所有条目（每个 type 都收，包括 folder/docx/...）。
// 返回 map 的 key 是相对 listing 根的路径，分隔符固定为 "/"。
// 调用方按 Type 过滤需要的子集（pull/status 只看 type=file，push 把 file 上传、folder 用作 cache）。
func ListFolderRecursive(folderToken, userAccessToken string) (map[string]DriveRemoteEntry, error) {
	out := make(map[string]DriveRemoteEntry)
	if err := listFolderRecursiveInner(folderToken, "", userAccessToken, out); err != nil {
		return nil, err
	}
	return out, nil
}

func listFolderRecursiveInner(folderToken, relBase, userAccessToken string, out map[string]DriveRemoteEntry) error {
	pageToken := ""
	for {
		files, nextPageToken, hasMore, err := ListFiles(folderToken, 200, pageToken, userAccessToken)
		if err != nil {
			return err
		}
		for _, f := range files {
			if f.Name == "" || f.Token == "" {
				continue
			}
			rel := f.Name
			if relBase != "" {
				rel = relBase + "/" + f.Name
			}
			out[rel] = DriveRemoteEntry{FileToken: f.Token, Type: f.Type, RelPath: rel}
			if f.Type == "folder" {
				if err := listFolderRecursiveInner(f.Token, rel, userAccessToken, out); err != nil {
					return err
				}
			}
		}
		if !hasMore || nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
	}
	return nil
}

// HashRemoteFile 流式下载远端文件并计算 SHA-256。
// 仅用于 status 比对，不落地到磁盘；流式读取使内存峰值保持在 O(64KB)，避免大文件 OOM。
func HashRemoteFile(fileToken, userAccessToken string) (string, error) {
	c, err := GetClient()
	if err != nil {
		return "", err
	}
	req := larkdrive.NewDownloadFileReqBuilder().FileToken(fileToken).Build()
	resp, err := c.Drive.File.Download(ContextWithTimeout(downloadTimeout), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("下载远端文件以计算哈希失败 (token=%s): %w", fileToken, err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("下载远端文件以计算哈希失败 (token=%s): code=%d, msg=%s", fileToken, resp.Code, resp.Msg)
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(resp.File, maxDownloadSize)); err != nil {
		return "", fmt.Errorf("读取远端文件流以计算哈希失败 (token=%s): %w", fileToken, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashLocalFile 计算本地文件的 SHA-256。
func HashLocalFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DeleteDriveFileByToken 删除 type=file 的云盘文件（type=folder 不在本镜像范围内删除）。
func DeleteDriveFileByToken(fileToken, userAccessToken string) error {
	if _, err := DeleteFile(fileToken, "file", userAccessToken); err != nil {
		return fmt.Errorf("删除云盘文件失败 (token=%s): %w", fileToken, err)
	}
	return nil
}

// CreateFolder 创建文件夹
func CreateFolder(name string, folderToken string, userAccessToken ...string) (string, string, error) {
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

	resp, err := client.Drive.File.CreateFolder(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
	return MoveFileWithToken(fileToken, targetFolderToken, fileType, "")
}

// MoveFileWithToken 移动文件/文件夹，支持 User Access Token
// 对于 folder 类型，返回的 task_id 需要通过 GetDriveTaskCheck 轮询
func MoveFileWithToken(fileToken, targetFolderToken, fileType, userAccessToken string) (string, error) {
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

	resp, err := client.Drive.File.Move(Context(), req, UserTokenOption(userAccessToken)...)
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
func CopyFile(fileToken string, targetFolderToken string, name string, fileType string, userAccessToken ...string) (string, string, error) {
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

	resp, err := client.Drive.File.Copy(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func DeleteFile(fileToken string, fileType string, userAccessToken ...string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdrive.NewDeleteFileReqBuilder().
		FileToken(fileToken).
		Type(fileType).
		Build()

	resp, err := client.Drive.File.Delete(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func CreateShortcut(parentToken string, targetFileToken string, targetType string, userAccessToken ...string) (*ShortcutInfo, error) {
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

	resp, err := client.Drive.File.CreateShortcut(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
	return DownloadFileWithToken(fileToken, outputPath, "", timeout...)
}

// DownloadFileWithToken 下载云盘文件，支持 User Access Token
func DownloadFileWithToken(fileToken, outputPath, userAccessToken string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}

	t := resolveTimeout(downloadTimeout, timeout)
	if userAccessToken != "" {
		return downloadDriveFileWithUserTokenRaw(fileToken, outputPath, userAccessToken, t)
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDownloadFileReqBuilder().
		FileToken(fileToken).
		Build()

	resp, err := client.Drive.File.Download(ContextWithTimeout(t), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("下载文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return writeStreamToFile(resp.File, outputPath)
}

func downloadDriveFileWithUserTokenRaw(fileToken, outputPath, userAccessToken string, timeout time.Duration) error {
	reqURL := buildDriveFileDownloadURL(fileToken)
	httpClient := &http.Client{Timeout: timeout}
	req, err := newBearerDownloadRequest(reqURL, userAccessToken, "")
	if err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}

	httpResp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		apiErr, parseErr := parseDownloadAPIError("下载文件", httpResp)
		if parseErr != nil {
			return parseErr
		}
		if isDownloadFileSizeLimitError(apiErr.Code, apiErr.Msg, nil) {
			return downloadBearerURLByRange("下载文件", reqURL, outputPath, userAccessToken, timeout)
		}
		return fmt.Errorf("下载文件失败: code=%d, msg=%s", apiErr.Code, apiErr.Msg)
	}

	bodyReader, apiErr, inspectErr := inspectDownloadAPIErrorResponse(httpResp)
	if inspectErr != nil {
		return fmt.Errorf("下载文件失败: 读取响应失败: %w", inspectErr)
	}
	if apiErr != nil {
		if isDownloadFileSizeLimitError(apiErr.Code, apiErr.Msg, nil) {
			return downloadBearerURLByRange("下载文件", reqURL, outputPath, userAccessToken, timeout)
		}
		return fmt.Errorf("下载文件失败: code=%d, msg=%s", apiErr.Code, apiErr.Msg)
	}

	if err := writeStreamToFile(bodyReader, outputPath); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}
	return nil
}

func buildDriveFileDownloadURL(fileToken string) string {
	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	return fmt.Sprintf("%s/open-apis/drive/v1/files/%s/download", baseURL, url.PathEscape(fileToken))
}

func isDownloadFileSizeLimitError(code int, msg string, err error) bool {
	if code == messageResourceFileSizeExceedsLimitCode {
		return true
	}
	text := strings.ToLower(msg)
	if err != nil {
		text += " " + strings.ToLower(err.Error())
	}
	return strings.Contains(text, "downloaded file size exceeds limit") ||
		(strings.Contains(text, "file size") && strings.Contains(text, "exceed"))
}

// maxSingleUploadSize 单次上传的文件大小上限（20MB），超过此大小需使用分片上传
const maxSingleUploadSize = 20 * 1024 * 1024

// UploadFile 上传文件到飞书云空间，超过 20MB 自动使用分片上传（App Token）
func UploadFile(filePath, parentToken, fileName string) (string, error) {
	return UploadFileWithToken(filePath, parentToken, fileName, "")
}

// UploadFileWithToken 上传文件到飞书云空间，支持 User Access Token 覆盖
// userAccessToken 为空时退回 App/Tenant Token
func UploadFileWithToken(filePath, parentToken, fileName, userAccessToken string) (string, error) {
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
		return uploadFileMultipart(filePath, parentToken, fileName, fileSize, userAccessToken)
	}
	return uploadFileSingle(file, parentToken, fileName, fileSize, userAccessToken)
}

// uploadFileSingle 单次上传文件（适用于 ≤ 20MB 的文件）
func uploadFileSingle(file *os.File, parentToken, fileName string, fileSize int, userAccessToken string) (string, error) {
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

	resp, err := client.Drive.File.UploadAll(ContextWithTimeout(downloadTimeout), req, UserTokenOption(userAccessToken)...)
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
func uploadFileMultipart(filePath, parentToken, fileName string, fileSize int, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenOpts := UserTokenOption(userAccessToken)

	// 第一步：准备分片上传
	prepareReq := larkdrive.NewUploadPrepareFileReqBuilder().
		FileUploadInfo(larkdrive.NewFileUploadInfoBuilder().
			FileName(fileName).
			ParentType("explorer").
			ParentNode(parentToken).
			Size(fileSize).
			Build()).
		Build()

	prepareResp, err := client.Drive.File.UploadPrepare(Context(), prepareReq, tokenOpts...)
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

	fmt.Fprintf(os.Stderr, "分片上传: 文件大小 %s, 分片大小 %s, 共 %d 个分片\n",
		formatSize(fileSize), formatSize(blockSize), blockNum)

	// 打开一次文件，通过 io.SectionReader 为每个分片提供无状态视图，避免每次重试都重新 open/seek
	srcFile, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer srcFile.Close()

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
			partReq := larkdrive.NewUploadPartFileReqBuilder().
				Body(larkdrive.NewUploadPartFileReqBodyBuilder().
					UploadId(uploadID).
					Seq(seq).
					Size(int(partSize)).
					File(io.NewSectionReader(srcFile, offset, partSize)).
					Build()).
				Build()

			partResp, err := client.Drive.File.UploadPart(ContextWithTimeout(downloadTimeout), partReq, tokenOpts...)

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
				fmt.Fprintf(os.Stderr, "  第 %d/%d 片上传失败，重试 (%d/%d)...\n", seq+1, blockNum, attempt, maxPartRetries)
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}

		if !uploaded {
			return "", lastErr
		}

		fmt.Fprintf(os.Stderr, "  分片 %d/%d 上传完成 (%s)\n", seq+1, blockNum, formatSize(int(partSize)))
	}

	// 第三步：完成上传
	finishReq := larkdrive.NewUploadFinishFileReqBuilder().
		Body(larkdrive.NewUploadFinishFileReqBodyBuilder().
			UploadId(uploadID).
			BlockNum(blockNum).
			Build()).
		Build()

	finishResp, err := client.Drive.File.UploadFinish(Context(), finishReq, tokenOpts...)
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
func CreateFileVersion(fileToken, objType, name string, userAccessToken ...string) (*FileVersionInfo, error) {
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

	resp, err := client.Drive.FileVersion.Create(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func GetFileVersion(fileToken, versionID, objType string, userAccessToken ...string) (*FileVersionInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetFileVersionReqBuilder().
		FileToken(fileToken).
		VersionId(versionID).
		ObjType(objType).
		Build()

	resp, err := client.Drive.FileVersion.Get(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func ListFileVersions(fileToken, objType string, pageSize int, pageToken string, userAccessToken ...string) ([]*FileVersionInfo, string, bool, error) {
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

	resp, err := client.Drive.FileVersion.List(Context(), reqBuilder.Build(), UserTokenOption(firstString(userAccessToken))...)
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
func DeleteFileVersion(fileToken, versionID, objType string, userAccessToken ...string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDeleteFileVersionReqBuilder().
		FileToken(fileToken).
		VersionId(versionID).
		ObjType(objType).
		Build()

	resp, err := client.Drive.FileVersion.Delete(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func BatchGetMeta(docTokens []string, docType string, userAccessToken ...string) ([]*FileMeta, error) {
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

	resp, err := client.Drive.Meta.BatchQuery(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
func GetFileStatistics(fileToken, fileType string, userAccessToken ...string) (*FileStats, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetFileStatisticsReqBuilder().
		FileToken(fileToken).
		FileType(fileType).
		Build()

	resp, err := client.Drive.FileStatistics.Get(Context(), req, UserTokenOption(firstString(userAccessToken))...)
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
