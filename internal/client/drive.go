package client

import (
	"bytes"
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
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// 最大下载文件大小限制 (100MB)
const maxDownloadSize = 100 * 1024 * 1024

// 下载超时时间
const downloadTimeout = 5 * time.Minute

// UploadMedia uploads a file to Feishu drive
func UploadMedia(filePath string, parentType string, parentNode string, fileName string, userAccessToken ...string) (string, http.Header, error) {
	return UploadMediaWithExtra(filePath, parentType, parentNode, fileName, "", firstString(userAccessToken))
}

const (
	driveImportMediaParentType = "ccm_import_open"
)

// DriveNeedsMultipart 判断是否超过 files/medias 单次上传 20MB 上限。
func DriveNeedsMultipart(size int64) bool {
	return size > int64(maxSingleUploadSize)
}

// UploadMediaForImport 通过 medias 上传临时媒体用于 drive import。
//
// 官方协议：
//   - parent_type 固定 ccm_import_open，extra 携带 obj_type / file_extension
//   - ≤20MB：POST /medias/upload_all，**省略 parent_node**（不要填 ccm_import_open）
//   - >20MB：upload_prepare / upload_part / upload_finish，prepare 显式带 parent_node=""
//
// 不会在用户云盘留下中间文件。
func UploadMediaForImport(filePath, fileName, objType, fileExtension, userAccessToken string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
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

	if DriveNeedsMultipart(stat.Size()) {
		return uploadMediaForImportMultipart(filePath, fileName, stat.Size(), string(extraJSON), userAccessToken)
	}
	return uploadMediaForImportAll(file, fileName, stat.Size(), string(extraJSON), userAccessToken)
}

func uploadMediaForImportAll(file io.Reader, fileName string, fileSize int64, extra, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	// upload_all 省略 parent_node，与官方 lark-cli 一致（staging 在隐式导入区）。
	fd := larkcore.NewFormdata().
		AddField("file_name", fileName).
		AddField("parent_type", driveImportMediaParentType).
		AddField("size", fmt.Sprintf("%d", fileSize)).
		AddField("extra", extra).
		AddFile("file", file)

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/medias/upload_all", fd, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("上传导入媒体失败: %w", err)
	}
	if err := CheckAPIResponse("上传导入媒体", resp); err != nil {
		return "", err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析导入媒体上传响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("上传导入媒体失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data.FileToken == "" {
		return "", fmt.Errorf("上传导入媒体成功但未返回 file_token")
	}
	return apiResp.Data.FileToken, nil
}

func uploadMediaForImportMultipart(filePath, fileName string, fileSize int64, extra, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	// upload_prepare 必须显式发送 parent_node（即使为空字符串）。
	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_prepare", map[string]any{
		"file_name":   fileName,
		"parent_type": driveImportMediaParentType,
		"parent_node": "",
		"size":        fileSize,
		"extra":       extra,
	}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("初始化导入媒体分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("初始化导入媒体分片上传", prepareResp); err != nil {
		return "", err
	}

	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, fileSize)
	if err != nil {
		return "", fmt.Errorf("初始化导入媒体分片上传失败: %w", err)
	}

	fmt.Fprintf(os.Stderr, "导入媒体分片上传: %s，%d 片 × %s\n", fileName, session.BlockNum, formatSize(int(session.BlockSize)))

	src, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	buffer := make([]byte, int(session.BlockSize))
	remaining := fileSize
	for seq := int64(0); seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}
		n, readErr := io.ReadFull(src, buffer[:int(chunkSize)])
		if readErr != nil {
			return "", fmt.Errorf("读取导入媒体分片失败: %w", readErr)
		}
		fd := larkcore.NewFormdata().
			AddField("upload_id", session.UploadID).
			AddField("seq", fmt.Sprintf("%d", seq)).
			AddField("size", fmt.Sprintf("%d", n)).
			AddFile("file", bytes.NewReader(buffer[:n]))
		partResp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/medias/upload_part", fd, tokenType, opts...)
		if err != nil {
			return "", fmt.Errorf("上传导入媒体分片 %d/%d 失败: %w", seq+1, session.BlockNum, err)
		}
		if err := CheckAPIResponse(fmt.Sprintf("上传导入媒体分片 %d/%d", seq+1, session.BlockNum), partResp); err != nil {
			return "", err
		}
		fmt.Fprintf(os.Stderr, "  分片 %d/%d 上传完成 (%s)\n", seq+1, session.BlockNum, formatSize(n))
		remaining -= int64(n)
	}
	if remaining != 0 {
		return "", fmt.Errorf("upload_prepare 分片计划不一致: 结束后仍剩 %d 字节", remaining)
	}

	finishResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_finish", map[string]any{
		"upload_id": session.UploadID,
		"block_num": session.BlockNum,
	}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("完成导入媒体分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成导入媒体分片上传", finishResp); err != nil {
		return "", err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(finishResp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析导入媒体 finish 响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("完成导入媒体分片上传失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data.FileToken == "" {
		return "", fmt.Errorf("分片上传完成但未返回 file_token")
	}
	return apiResp.Data.FileToken, nil
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

// DownloadFromURL 从（预签名/公开）URL 下载文件，限制最大 100MB。
//
// 请求 context 派生自进程根 context（Ctrl-C 可中断）；timeout > 0 时为总时长上限，
// 默认 5 分钟；单次等待另受空闲超时约束，断流时按分片有界重试并从断点续传（服务端支持 Range 时）。
// 写盘为同目录临时文件 + rename，失败不留半截文件、不破坏已存在的同名文件。
func DownloadFromURL(url string, outputPath string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}
	_, _, err := downloadStreamToFile(downloadStreamSpec{
		Action:     "从 URL 下载",
		URL:        url,
		HTTPClient: publicDownloadHTTPClient(nil),
		MaxBytes:   maxDownloadSize,
	}, outputPath, resolveTimeout(downloadTimeout, timeout))
	return err
}

// saveToFile 将 reader 内容原子写入文件，限制最大大小 (maxDownloadSize)。
// 恰好 100MB 允许写入；严格大于 100MB 则报错，且不会留下不完整文件、不会改动已存在的同名文件。
func saveToFile(reader io.Reader, outputPath string) error {
	limited := &maxSizeReader{r: reader, max: maxDownloadSize}
	if _, err := safefile.AtomicWriteFrom(outputPath, limited, 0o644); err != nil {
		if limited.exceeded {
			return fmt.Errorf("文件超过大小限制 (%d MB)", maxDownloadSize/(1024*1024))
		}
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// errDownloadTooLarge 由 maxSizeReader 在超过上限时返回，使原子写中止。
var errDownloadTooLarge = fmt.Errorf("文件超过大小限制 (%d MB)", maxDownloadSize/(1024*1024))

// maxSizeReader 读取超过 max 字节时返回错误（恰好 max 字节允许）。
type maxSizeReader struct {
	r        io.Reader
	max      int64
	n        int64
	exceeded bool
}

func (m *maxSizeReader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	m.n += int64(n)
	if m.n > m.max {
		m.exceeded = true
		return 0, errDownloadTooLarge
	}
	return n, err
}

// validatePath 验证路径安全性，防止路径遍历攻击
// validatePath 校验本地输出路径：按路径段拒绝越出当前目录的 ".."，
// 并拒绝敏感目录（~/.ssh、~/.feishu-cli、/etc 等），规则见 internal/safefile。
func validatePath(path string) error {
	return safefile.ValidateOutputPath(path)
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

// DriveRemoteEntry 是 ListFolderRecursive / ListFolderEntries 返回的单个云盘条目。
// 与 DriveFile 相比，附带递归基础上的 RelPath（用 "/" 分隔）。
type DriveRemoteEntry struct {
	FileToken    string
	Type         string // file / folder / docx / sheet / bitable / mindnote / slides / shortcut
	RelPath      string
	Name         string
	CreatedTime  string // 服务端 epoch 字符串（秒/毫秒）
	ModifiedTime string
}

// ListFolderRecursive 递归列出 folderToken 下的所有条目（每个 type 都收，包括 folder/docx/...）。
// 返回 map 的 key 是相对 listing 根的路径，分隔符固定为 "/"。
// 远端存在重复相对路径时 fail closed（需要按策略处理重名时改用 ListFolderEntries）。
func ListFolderRecursive(folderToken, userAccessToken string) (map[string]DriveRemoteEntry, error) {
	entries, err := ListFolderEntries(folderToken, userAccessToken)
	if err != nil {
		return nil, err
	}
	out := make(map[string]DriveRemoteEntry, len(entries))
	for _, e := range entries {
		if existing, exists := out[e.RelPath]; exists {
			return nil, fmt.Errorf("远端存在重复相对路径 %q: 发现多个条目 (token %s[%s] 与 token %s[%s])，为防止静默覆盖导致数据丢失，已中止操作",
				e.RelPath, existing.FileToken, existing.Type, e.FileToken, e.Type)
		}
		out[e.RelPath] = e
	}
	return out, nil
}

// ListFolderEntries 递归列出 folderToken 下的所有条目（含重名条目，按服务端返回顺序），
// 每层完整翻页，遇到"has_more 但无游标/游标不前进"立即报错。
func ListFolderEntries(folderToken, userAccessToken string) ([]DriveRemoteEntry, error) {
	var out []DriveRemoteEntry
	if err := listFolderEntriesInner(folderToken, "", userAccessToken, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func listFolderEntriesInner(folderToken, relBase, userAccessToken string, out *[]DriveRemoteEntry) error {
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
			*out = append(*out, DriveRemoteEntry{
				FileToken:    f.Token,
				Type:         f.Type,
				RelPath:      rel,
				Name:         f.Name,
				CreatedTime:  f.CreatedTime,
				ModifiedTime: f.ModifiedTime,
			})
			if f.Type == "folder" {
				if err := listFolderEntriesInner(f.Token, rel, userAccessToken, out); err != nil {
					return err
				}
			}
		}
		more, cursor, perr := PaginationCursor(hasMore, "", nextPageToken, pageToken)
		if perr != nil {
			return perr
		}
		if !more {
			break
		}
		pageToken = cursor
	}
	return nil
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

// GetRootFolderToken 解析当前身份的 Drive 根目录 token。
// 官方协议：GET /open-apis/drive/explorer/v2/root_folder/meta，取 data.token。
// drive move 省略 --folder-token 时必须先取真实 root，不能把空字符串交给 move API。
func GetRootFolderToken(userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), "/open-apis/drive/explorer/v2/root_folder/meta", nil, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("获取根目录 token 失败: %w", err)
	}
	if err := CheckAPIResponse("获取根目录 token", resp); err != nil {
		return "", err
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析根目录元数据失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("获取根目录 token 失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if strings.TrimSpace(apiResp.Data.Token) == "" {
		return "", fmt.Errorf("root_folder/meta 未返回 token")
	}
	return apiResp.Data.Token, nil
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

// DeleteDriveFileAsync 以异步模式删除云盘文件/文件夹：DELETE /open-apis/drive/v1/files/{token}?type=&async=true。
// 返回 task_id 非空时需用 task_check 轮询（文件夹删除等）；为空表示已同步删除完成。
// 业务错误随 HTTP 400/403 下发时先解析业务码。
func DeleteDriveFileAsync(fileToken, fileType, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("type", fileType)
	q.Set("async", "true")
	apiPath := "/open-apis/drive/v1/files/" + url.PathEscape(fileToken) + "?" + q.Encode()
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Delete(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("删除文件失败: %w", err)
	}
	if err := CheckAPIResponse("删除文件", resp); err != nil {
		return "", err
	}
	var parsed struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return "", fmt.Errorf("解析删除响应失败: %w", err)
	}
	return parsed.Data.TaskID, nil
}

// UpdateDriveTitle 重命名云盘文件/文件夹/在线文档/wiki 节点：
// PATCH /open-apis/drive/v1/files/{token}?type=<type>，body {"new_title": title}。
func UpdateDriveTitle(token, docType, title, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	apiPath := "/open-apis/drive/v1/files/" + url.PathEscape(token) + "?type=" + url.QueryEscape(docType)
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Patch(Context(), apiPath, map[string]any{"new_title": title}, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("修改标题失败: %w", err)
	}
	return CheckAPIResponse("修改标题", resp)
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

// DownloadFileWithToken 下载云盘文件，支持 User Access Token（为空时使用 Bot/Tenant 身份）。
//
// 两种身份都走流式下载（不再把整个文件读进内存、无 100MB 上限）：遇到"文件超过下载大小限制"
// 自动切换 HTTP Range 分片；每个分片有界重试并断点续传；空闲超时替代总时长（timeout > 0 时额外
// 作为总时长上限）；写盘为同目录临时文件 + rename。
func DownloadFileWithToken(fileToken, outputPath, userAccessToken string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	}
	d, err := OpenDriveFileDownload(fileToken, "", userAccessToken, t)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.SaveTo(outputPath)
	return err
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

// DownloadFileVersion 下载云盘文件的指定历史版本内容。
//
// 远端版本下载 = GET /open-apis/drive/v1/files/{file_token}/download?version=N，
// 即同一个 file_token + version 查询参数，不会产生新 token（lark dry-run 实证）。
// SDK v3.5.3 的 NewDownloadFileReqBuilder.Build() 只拷贝 PathParams、丢弃 QueryParams，
// 因此这里直接拼 URL 走 Bearer 流式下载（不再把整个版本内容读进内存）。
func DownloadFileVersion(fileToken, version, outputPath, userAccessToken string, timeout ...time.Duration) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}
	if fileToken == "" {
		return fmt.Errorf("file_token 不能为空")
	}
	if version == "" {
		return fmt.Errorf("version 不能为空")
	}
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	}
	// 与最新版本下载同一条流式链路：业务错误（含 HTTP 200 + JSON 信封）在打开时返回，不会落盘
	d, err := OpenDriveFileDownload(fileToken, version, userAccessToken, t)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.SaveTo(outputPath)
	return err
}

// parseDownloadJSONError 判断 download 的 HTTP 200 响应是否为飞书业务错误体 {code,msg}。
// 飞书 OpenAPI 业务错误响应带 Content-Type: application/json；成功的文件下载返回文件
// MIME / octet-stream。故仅在 Content-Type 为 JSON 时才尝试解析业务错误，parse 出 code != 0
// 即视为错误，否则当二进制写盘。
//
// 局限：HTTP 200 + application/json + {code:N} 无法与「内容恰为该结构的合法 .json 文件」完美
// 区分。本函数仅服务于 DownloadFileVersion 的文本/markdown 版本下载（.md 的 Content-Type 非
// application/json，不触发）；若需下载可能是顶层 {code:N} 结构的 .json 文件，请用
// DownloadMedia / DownloadFileWithToken（走 SDK Success 判定，不做此启发式）。
func parseDownloadJSONError(header http.Header, body []byte) (int, string, bool) {
	if header == nil || !strings.Contains(header.Get("Content-Type"), "application/json") {
		return 0, "", false
	}
	var e struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &e); err != nil {
		return 0, "", false
	}
	if e.Code != 0 {
		return e.Code, e.Msg, true
	}
	return 0, "", false
}

// maxSingleUploadSize 单次上传的文件大小上限（20MB），超过此大小需使用分片上传。
// 测试可覆盖该阈值以验证 20MB 边界走 multipart，而不必落 20MB 实物文件。
var maxSingleUploadSize = 20 * 1024 * 1024

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

// DriveOverwriteResult 原地覆盖上传的结果。
type DriveOverwriteResult struct {
	FileToken string `json:"file_token"`
	Version   string `json:"version,omitempty"`
}

// OverwriteDriveFileFromPath 用本地文件原地覆盖已有云盘文件（file_token 不变，产生新版本）。
//
// 官方协议（与 lark-cli drive +push --if-exists=overwrite 一致）：
//   - ≤20MB：POST /open-apis/drive/v1/files/upload_all，multipart 表单携带 file_token；
//   - >20MB：upload_prepare 携带 file_token → upload_part（每片有界重试）→ upload_finish。
//
// 覆盖保留原 file_token，因此链接、协作者、评论、历史版本都不会断开；任何一步失败都直接返回错误，
// 绝不回退为"先删后传"（那样删除成功、上传失败就会丢文件）。服务端若未返回 version
// （租户未灰度覆盖字段），视为失败而不是虚报成功。
func OverwriteDriveFileFromPath(filePath, parentToken, fileName, fileToken, userAccessToken string) (*DriveOverwriteResult, error) {
	if strings.TrimSpace(fileToken) == "" {
		return nil, fmt.Errorf("覆盖上传需要已有文件的 file_token")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	size := stat.Size()
	if DriveNeedsMultipart(size) {
		return overwriteDriveFileMultipart(f, parentToken, fileName, fileToken, size, userAccessToken)
	}
	return overwriteDriveFileAll(f, parentToken, fileName, fileToken, size, userAccessToken)
}

func overwriteDriveFileAll(file io.Reader, parentToken, fileName, fileToken string, size int64, userAccessToken string) (*DriveOverwriteResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	fd := larkcore.NewFormdata().
		AddField("file_name", fileName).
		AddField("parent_type", "explorer").
		AddField("parent_node", parentToken).
		AddField("size", fmt.Sprintf("%d", size)).
		AddField("file_token", fileToken).
		AddFile("file", file)
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/files/upload_all", fd, tokenType, append(opts, larkcore.WithFileUpload())...)
	if err != nil {
		return nil, fmt.Errorf("覆盖上传文件失败: %w", err)
	}
	if err := CheckAPIResponse("覆盖上传文件", resp); err != nil {
		return nil, err
	}
	return parseDriveOverwriteResponse(resp.RawBody, fileToken)
}

func overwriteDriveFileMultipart(file io.ReaderAt, parentToken, fileName, fileToken string, size int64, userAccessToken string) (*DriveOverwriteResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/files/upload_prepare", map[string]any{
		"file_name":   fileName,
		"parent_type": "explorer",
		"parent_node": parentToken,
		"size":        size,
		"file_token":  fileToken,
	}, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("覆盖上传（分片准备）失败: %w", err)
	}
	if err := CheckAPIResponse("覆盖上传（分片准备）", prepareResp); err != nil {
		return nil, err
	}
	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, size)
	if err != nil {
		return nil, fmt.Errorf("覆盖上传（分片准备）失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "覆盖分片上传: 文件大小 %s, 分片大小 %s, 共 %d 个分片\n",
		formatSize(int(size)), formatSize(int(session.BlockSize)), session.BlockNum)

	const maxPartRetries = 3
	for seq := int64(0); seq < session.BlockNum; seq++ {
		offset := seq * session.BlockSize
		partSize := session.BlockSize
		if remaining := size - offset; partSize > remaining {
			partSize = remaining
		}
		var lastErr error
		for attempt := 1; attempt <= maxPartRetries; attempt++ {
			fd := larkcore.NewFormdata().
				AddField("upload_id", session.UploadID).
				AddField("seq", fmt.Sprintf("%d", seq)).
				AddField("size", fmt.Sprintf("%d", partSize)).
				AddFile("file", io.NewSectionReader(file, offset, partSize))
			partResp, perr := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/files/upload_part", fd, tokenType, append(opts, larkcore.WithFileUpload())...)
			if perr == nil {
				perr = CheckAPIResponse(fmt.Sprintf("上传分片 %d/%d", seq+1, session.BlockNum), partResp)
			} else {
				perr = fmt.Errorf("上传分片 %d/%d 失败: %w", seq+1, session.BlockNum, perr)
			}
			if perr == nil {
				lastErr = nil
				break
			}
			lastErr = perr
			// 参数类错误（如 1062009 大小不一致）重试无意义，直接失败
			if apiErr, ok := AsAPIError(perr); ok && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 && apiErr.HTTPStatus != http.StatusTooManyRequests && apiErr.Code != 99991400 {
				break
			}
			if attempt < maxPartRetries {
				fmt.Fprintf(os.Stderr, "  第 %d/%d 片上传失败，重试 (%d/%d)...\n", seq+1, session.BlockNum, attempt, maxPartRetries)
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
		if lastErr != nil {
			return nil, lastErr
		}
		fmt.Fprintf(os.Stderr, "  分片 %d/%d 上传完成 (%s)\n", seq+1, session.BlockNum, formatSize(int(partSize)))
	}

	finishResp, err := cli.Post(Context(), "/open-apis/drive/v1/files/upload_finish", map[string]any{
		"upload_id": session.UploadID,
		"block_num": session.BlockNum,
	}, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("完成覆盖分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成覆盖分片上传", finishResp); err != nil {
		return nil, err
	}
	return parseDriveOverwriteResponse(finishResp.RawBody, fileToken)
}

func parseDriveOverwriteResponse(raw []byte, wantToken string) (*DriveOverwriteResult, error) {
	var apiResp struct {
		Data struct {
			FileToken   string `json:"file_token"`
			Version     string `json:"version"`
			DataVersion string `json:"data_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("解析覆盖上传响应失败: %w", err)
	}
	res := &DriveOverwriteResult{FileToken: apiResp.Data.FileToken, Version: apiResp.Data.Version}
	if res.Version == "" {
		res.Version = apiResp.Data.DataVersion
	}
	if res.FileToken == "" {
		return nil, fmt.Errorf("覆盖上传失败: 未返回 file_token")
	}
	if res.Version == "" {
		// 协议保证覆盖会返回新版本号；缺失说明租户尚未灰度该字段，不能当作成功
		return res, fmt.Errorf("覆盖上传后服务端未返回 version（租户可能尚未支持原地覆盖），请暂时改用 --if-exists skip")
	}
	if wantToken != "" && res.FileToken != wantToken {
		return res, fmt.Errorf("覆盖上传返回的 file_token %s 与原文件 %s 不一致，请在云盘中核对", res.FileToken, wantToken)
	}
	return res, nil
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
