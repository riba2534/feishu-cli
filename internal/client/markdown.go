package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

const MarkdownDiffMaxContentBytes int64 = 10 * 1024 * 1024

const (
	// markdownSourceFilePreviewType 是官方 native Markdown 源文件预览码（preview_type=16）。
	markdownSourceFilePreviewType    = "16"
	markdownUploadParentTypeExplorer = "explorer"
	markdownUploadParentTypeWiki     = "wiki"
	markdownEmptyContentError        = "Markdown 内容为空，不支持创建或覆盖空 .md 文件"
)

// MarkdownUploadSpec 描述 native Markdown 创建/覆盖的上传目标。
type MarkdownUploadSpec struct {
	FileToken   string // 覆盖时必填；创建时为空
	FileName    string
	FolderToken string // Drive 文件夹；与 WikiToken 互斥
	WikiToken   string // wiki 节点；parent_type=wiki
}

// MarkdownUploadResult 是 upload_all / upload_finish 的归一化结果。
type MarkdownUploadResult struct {
	FileToken string
	Version   string
}

func (spec MarkdownUploadSpec) target() (parentType, parentNode string) {
	if spec.WikiToken != "" {
		return markdownUploadParentTypeWiki, spec.WikiToken
	}
	return markdownUploadParentTypeExplorer, spec.FolderToken
}

func (spec MarkdownUploadSpec) needsMultipart(size int64) bool {
	return size > int64(maxSingleUploadSize)
}

// FetchMarkdownSource 下载 Drive 原生 Markdown 源文件（或指定历史版本）。
//
// 官方协议：GET /open-apis/drive/v1/medias/{token}/preview_download?preview_type=16[&version=N]
// 不再走 /drive/v1/files/{token}/download。
func FetchMarkdownSource(fileToken, version, userAccessToken string) ([]byte, string, error) {
	if strings.TrimSpace(fileToken) == "" {
		return nil, "", fmt.Errorf("file_token 不能为空")
	}
	if version != "" && strings.TrimSpace(version) == "" {
		return nil, "", fmt.Errorf("version 不能为空")
	}

	cli, err := GetClient()
	if err != nil {
		return nil, "", err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	req := &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/drive/v1/medias/:file_token/preview_download",
		PathParams:                larkcore.PathParams{},
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
	}
	req.PathParams.Set("file_token", fileToken)
	req.QueryParams.Set("preview_type", markdownSourceFilePreviewType)
	if version != "" {
		req.QueryParams.Set("version", version)
	}

	resp, err := cli.Do(ContextWithTimeout(downloadTimeout), req, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// 业务错误常随 HTTP 400/403 下发：先解析飞书信封里的 code（含 log_id），再回退 HTTP 状态
		if err := ParseAPIResponse("下载 Markdown 源文件", resp.StatusCode, resp.Header, resp.RawBody); err != nil {
			return nil, "", err
		}
	}
	if code, msg, isErr := parseDownloadJSONError(resp.Header, resp.RawBody); isErr {
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: code=%d, msg=%s", code, msg)
	}

	fallback := fileToken + ".md"
	fileName := fileNameFromDownloadHeader(resp.Header, fallback)
	return resp.RawBody, fileName, nil
}

// FetchMarkdownSourceLimited 流式下载 Markdown 源文件，超过 maxBytes 立即失败，避免 OOM。
func FetchMarkdownSourceLimited(fileToken, version, userAccessToken string, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		return nil, "", fmt.Errorf("maxBytes 必须为正")
	}
	if strings.TrimSpace(fileToken) == "" {
		return nil, "", fmt.Errorf("file_token 不能为空")
	}
	if version != "" && strings.TrimSpace(version) == "" {
		return nil, "", fmt.Errorf("version 不能为空")
	}

	httpResp, err := openMarkdownPreviewDownload(fileToken, version, userAccessToken)
	if err != nil {
		return nil, "", err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		apiErr, parseErr := parseDownloadAPIError("下载 Markdown 源文件", httpResp)
		if parseErr != nil {
			return nil, "", parseErr
		}
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: code=%d, msg=%s", apiErr.Code, apiErr.Msg)
	}

	bodyReader, apiErr, inspectErr := inspectDownloadAPIErrorResponse(httpResp)
	if inspectErr != nil {
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: 读取响应失败: %w", inspectErr)
	}
	if apiErr != nil {
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: code=%d, msg=%s", apiErr.Code, apiErr.Msg)
	}

	payload, err := io.ReadAll(io.LimitReader(bodyReader, maxBytes+1)) // +1 才能区分恰好上限与越界
	if err != nil {
		return nil, "", fmt.Errorf("下载 Markdown 源文件失败: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, "", fmt.Errorf("remote Markdown content exceeds %s markdown +diff content limit", formatSize(int(maxBytes)))
	}

	fallback := fileToken + ".md"
	fileName := fileNameFromDownloadHeader(httpResp.Header, fallback)
	return payload, fileName, nil
}

func ReadLocalMarkdownLimited(path string, maxBytes int64) ([]byte, error) {
	// 敏感目录、不存在、是目录、无权限读取均为用法错误（退出码 2）
	info, err := safefile.StatInputFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取本地文件失败: %w", err)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("local Markdown file exceeds %s markdown +diff content limit", formatSize(int(maxBytes)))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取本地文件失败: %w", err)
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取本地文件失败: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("local Markdown file exceeds %s markdown +diff content limit", formatSize(int(maxBytes)))
	}
	return payload, nil
}

func openMarkdownPreviewDownload(fileToken, version, userAccessToken string) (*http.Response, error) {
	bearer := strings.TrimSpace(userAccessToken)
	if bearer == "" {
		token, err := fetchTenantAccessTokenForDownload()
		if err != nil {
			return nil, err
		}
		bearer = token
	}
	reqURL := buildMarkdownPreviewDownloadURL(fileToken, version)
	req, err := newBearerDownloadRequest(reqURL, bearer, "")
	if err != nil {
		return nil, fmt.Errorf("下载 Markdown 源文件失败: %w", err)
	}
	// 必须走 config.NewHTTPClient：它带 host 校验（CheckRequestURL）与
	// 重定向剥离 Authorization 头的策略。裸 &http.Client{} 会在 3xx 时
	// 把 Bearer token 一路重放到重定向目标 host。
	httpClient := config.NewHTTPClient(downloadTimeout)
	httpResp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载 Markdown 源文件失败: %w", err)
	}
	return httpResp, nil
}

func buildMarkdownPreviewDownloadURL(fileToken, version string) string {
	cfg := config.Get()
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	u, _ := url.Parse(fmt.Sprintf("%s/open-apis/drive/v1/medias/%s/preview_download", baseURL, url.PathEscape(fileToken)))
	q := u.Query()
	q.Set("preview_type", markdownSourceFilePreviewType)
	if version != "" {
		q.Set("version", version)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// fetchTenantAccessTokenForDownload 取 tenant token 用于 media 预览下载。
// 必须走 auth.FetchTenantAccessTokenResult：它带超时、CheckRequestURL host 校验、
// 重定向剥离凭证头与 expires_in 校验。禁止在此手写 http.Post/http.DefaultClient，
// 否则 app_id+app_secret 会随重定向被重放到任意 host，且没有超时。
func fetchTenantAccessTokenForDownload() (string, error) {
	cfg := config.Get()
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	tok, err := auth.FetchTenantAccessTokenResult(Context(), cfg.AppID, cfg.AppSecret, baseURL)
	if err != nil {
		return "", fmt.Errorf("获取 tenant token 失败: %w", err)
	}
	return tok.AccessToken, nil
}

// FetchFileContent 把一个 Drive 原生 Markdown 文件的最新内容下载到内存。
// 走 preview_download?preview_type=16。
func FetchFileContent(fileToken string, userAccessToken string) ([]byte, error) {
	data, _, err := FetchMarkdownSource(fileToken, "", userAccessToken)
	return data, err
}

// FetchFileVersionContent 把一个 Drive 原生 Markdown 文件指定历史版本下载到内存。
// 走 preview_download?preview_type=16&version=N。
func FetchFileVersionContent(fileToken, version, userAccessToken string) ([]byte, error) {
	data, _, err := FetchMarkdownSource(fileToken, version, userAccessToken)
	return data, err
}

// FetchMarkdownFileName 通过 metas/batch_query 读取现有 .md 文件名（title）。
func FetchMarkdownFileName(fileToken, userAccessToken string) (string, error) {
	title, err := FetchDocMetaTitle(fileToken, "file", userAccessToken)
	if err != nil {
		return "", fmt.Errorf("读取现有 Markdown 文件名失败: %w", err)
	}
	return strings.TrimSpace(title), nil
}

// UploadMarkdownContent 把内存中的 Markdown 字节上传（创建或覆盖）。
// size > 20MB 走 files/upload_prepare → upload_part → upload_finish。
func UploadMarkdownContent(spec MarkdownUploadSpec, content []byte, userAccessToken string) (MarkdownUploadResult, error) {
	if err := validateNonEmptyMarkdownSize(int64(len(content))); err != nil {
		return MarkdownUploadResult{}, err
	}
	if spec.FileName == "" {
		return MarkdownUploadResult{}, fmt.Errorf("file_name 不能为空")
	}
	open := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	}
	size := int64(len(content))
	if spec.needsMultipart(size) {
		return uploadMarkdownMultipart(spec, size, open, userAccessToken)
	}
	return uploadMarkdownAll(spec, size, open, userAccessToken)
}

// UploadMarkdownFile 把本地 .md 文件上传（创建或覆盖）。>20MB 自动分片。
func UploadMarkdownFile(spec MarkdownUploadSpec, filePath string, userAccessToken string) (MarkdownUploadResult, error) {
	if spec.FileName == "" {
		spec.FileName = filepath.Base(filePath)
	}
	// 本地文件内容会上传到云盘：拒绝敏感目录，不存在/是目录归为用法错误
	info, err := safefile.StatInputFile(filePath)
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("读取本地文件失败: %w", err)
	}
	if err := validateNonEmptyMarkdownSize(info.Size()); err != nil {
		return MarkdownUploadResult{}, err
	}
	open := func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}
	if spec.needsMultipart(info.Size()) {
		return uploadMarkdownMultipart(spec, info.Size(), open, userAccessToken)
	}
	return uploadMarkdownAll(spec, info.Size(), open, userAccessToken)
}

// OverwriteFileWithToken 覆盖现有 Drive 文件（如 .md）的内容。
//
// 官方协议仍是 POST /open-apis/drive/v1/files/upload_all，带 file_token 原地覆盖；
// 超过 20MB 改走 upload_prepare / upload_part / upload_finish，并同样携带 file_token。
func OverwriteFileWithToken(fileToken, fileName string, content []byte, userAccessToken string) (string, error) {
	result, err := UploadMarkdownContent(MarkdownUploadSpec{
		FileToken: fileToken,
		FileName:  fileName,
	}, content, userAccessToken)
	if err != nil {
		return "", err
	}
	return result.FileToken, nil
}

// OverwriteFileFromPathWithToken 把本地文件内容覆盖到远端 Drive 文件。
func OverwriteFileFromPathWithToken(filePath, fileToken, fileName, userAccessToken string) (string, error) {
	result, err := UploadMarkdownFile(MarkdownUploadSpec{
		FileToken: fileToken,
		FileName:  fileName,
	}, filePath, userAccessToken)
	if err != nil {
		return "", err
	}
	return result.FileToken, nil
}

func validateNonEmptyMarkdownSize(size int64) error {
	if size == 0 {
		return fmt.Errorf("%s", markdownEmptyContentError)
	}
	return nil
}

func uploadMarkdownAll(spec MarkdownUploadSpec, fileSize int64, openReader func() (io.ReadCloser, error), userAccessToken string) (MarkdownUploadResult, error) {
	if spec.FileToken == "" && spec.FileName == "" {
		return MarkdownUploadResult{}, fmt.Errorf("file_name 不能为空")
	}
	parentType, parentNode := spec.target()

	fileReader, err := openReader()
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("打开 Markdown 内容失败: %w", err)
	}
	defer fileReader.Close()

	cli, err := GetClient()
	if err != nil {
		return MarkdownUploadResult{}, err
	}

	fd := larkcore.NewFormdata().
		AddField("file_name", spec.FileName).
		AddField("parent_type", parentType).
		AddField("parent_node", parentNode).
		AddField("size", fmt.Sprintf("%d", fileSize))
	if spec.FileToken != "" {
		fd.AddField("file_token", spec.FileToken)
	}
	fd.AddFile("file", fileReader)

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/files/upload_all", fd, tokenType, opts...)
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("上传 Markdown 失败: %w", err)
	}
	if err := CheckAPIResponse("上传 Markdown", resp); err != nil {
		return MarkdownUploadResult{}, err
	}
	return parseMarkdownUploadAPIResponse(resp.RawBody, spec.FileToken != "")
}

func uploadMarkdownMultipart(spec MarkdownUploadSpec, fileSize int64, openReader func() (io.ReadCloser, error), userAccessToken string) (MarkdownUploadResult, error) {
	parentType, parentNode := spec.target()
	cli, err := GetClient()
	if err != nil {
		return MarkdownUploadResult{}, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	prepareBody := map[string]any{
		"file_name":   spec.FileName,
		"parent_type": parentType,
		"parent_node": parentNode,
		"size":        fileSize,
	}
	if spec.FileToken != "" {
		prepareBody["file_token"] = spec.FileToken
	}

	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/files/upload_prepare", prepareBody, tokenType, opts...)
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("初始化 Markdown 分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("初始化 Markdown 分片上传", prepareResp); err != nil {
		return MarkdownUploadResult{}, err
	}

	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, fileSize)
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("初始化 Markdown 分片上传失败: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Markdown 分片上传: %d 片 × %s\n", session.BlockNum, formatSize(int(session.BlockSize)))

	fileReader, err := openReader()
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("打开 Markdown 内容失败: %w", err)
	}
	defer fileReader.Close()

	buffer := make([]byte, int(session.BlockSize))
	remaining := fileSize
	for seq := int64(0); seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}
		n, readErr := io.ReadFull(fileReader, buffer[:int(chunkSize)])
		if readErr != nil {
			return MarkdownUploadResult{}, fmt.Errorf("读取 Markdown 分片失败: %w", readErr)
		}

		fd := larkcore.NewFormdata().
			AddField("upload_id", session.UploadID).
			AddField("seq", fmt.Sprintf("%d", seq)).
			AddField("size", fmt.Sprintf("%d", n)).
			AddFile("file", bytes.NewReader(buffer[:n]))
		partResp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/files/upload_part", fd, tokenType, opts...)
		if err != nil {
			return MarkdownUploadResult{}, fmt.Errorf("上传 Markdown 分片 %d/%d 失败: %w", seq+1, session.BlockNum, err)
		}
		if err := CheckAPIResponse(fmt.Sprintf("上传 Markdown 分片 %d/%d", seq+1, session.BlockNum), partResp); err != nil {
			return MarkdownUploadResult{}, err
		}
		fmt.Fprintf(os.Stderr, "  分片 %d/%d 上传完成 (%s)\n", seq+1, session.BlockNum, formatSize(n))
		remaining -= int64(n)
	}
	if remaining != 0 {
		return MarkdownUploadResult{}, fmt.Errorf("upload_prepare 分片计划不一致: 结束后仍剩 %d 字节", remaining)
	}

	finishResp, err := cli.Post(Context(), "/open-apis/drive/v1/files/upload_finish", map[string]any{
		"upload_id": session.UploadID,
		"block_num": session.BlockNum,
	}, tokenType, opts...)
	if err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("完成 Markdown 分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成 Markdown 分片上传", finishResp); err != nil {
		return MarkdownUploadResult{}, err
	}
	return parseMarkdownUploadAPIResponse(finishResp.RawBody, spec.FileToken != "")
}

func parseMarkdownUploadAPIResponse(raw []byte, requireVersion bool) (MarkdownUploadResult, error) {
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileToken   string `json:"file_token"`
			Version     string `json:"version"`
			DataVersion string `json:"data_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return MarkdownUploadResult{}, fmt.Errorf("解析 Markdown 上传响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return MarkdownUploadResult{}, fmt.Errorf("上传 Markdown 失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	result := MarkdownUploadResult{FileToken: apiResp.Data.FileToken, Version: apiResp.Data.Version}
	if result.Version == "" {
		result.Version = apiResp.Data.DataVersion
	}
	if result.FileToken == "" {
		return MarkdownUploadResult{}, fmt.Errorf("上传 Markdown 失败: 未返回 file_token")
	}
	if requireVersion && result.Version == "" {
		return MarkdownUploadResult{}, fmt.Errorf("覆盖 Markdown 失败: 未返回 version")
	}
	return result, nil
}

func fileNameFromDownloadHeader(header http.Header, fallback string) string {
	name := fallback
	if header != nil {
		if headerName := larkcore.FileNameByHeader(header); strings.TrimSpace(headerName) != "" {
			name = headerName
		}
	}
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." {
		return fallback
	}
	return name
}
