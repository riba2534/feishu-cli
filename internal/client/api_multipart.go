package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
)

// MultipartFile 是 multipart/form-data 里的文件 part。
type MultipartFile struct {
	Field    string // 表单字段名（如 file / image）
	FileName string // part 的 filename（取本地文件名；stdin 时由调用方给默认名）
	Content  []byte
}

// MultipartResponse 是透传上传请求的原始响应，由调用方按业务信封解析。
type MultipartResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// apiMultipartMaxResponse 上传类接口的响应体上限（只是 JSON 信封，留足余量）。
const apiMultipartMaxResponse = 32 << 20

// BuildMultipartBody 按字段名排序写入文本字段，最后写文件 part；返回 body 与 Content-Type。
// 排序保证同样的输入产生稳定的请求体（SDK 的 Formdata 按 map 遍历，顺序不定且 filename 固定为 unknown-file）。
func BuildMultipartBody(fields map[string]string, file MultipartFile) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := w.WriteField(k, fields[k]); err != nil {
			return nil, "", err
		}
	}
	part, err := w.CreateFormFile(file.Field, file.FileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(file.Content); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// DoAPIMultipart 以 multipart/form-data 调用任意 OpenAPI 路径（api --file 使用）。
//
// Bearer：userAccessToken 非空时用 User Token，否则用进程内缓存的 tenant_access_token。
// 请求走 rawHTTPClient（共享连接池、host 白名单、重定向剥离凭证），超时与取消由 ctx 控制。
// 业务错误不在此解析：调用方用 ParseAPIResponse 先看业务信封再看 HTTP 状态码。
func DoAPIMultipart(ctx context.Context, method, apiPath string, query url.Values, fields map[string]string, file MultipartFile, userAccessToken string) (*MultipartResponse, error) {
	cfg := config.Get()
	if err := config.CheckBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if strings.TrimSpace(userAccessToken) == "" && (cfg.AppID == "" || cfg.AppSecret == "") {
		// 与 GetClient 一致：缺 App 凭证属于鉴权配置错误（退出码 3）
		return nil, clierr.Authf("缺少 app_id 或 app_secret 配置")
	}
	body, contentType, err := BuildMultipartBody(fields, file)
	if err != nil {
		return nil, fmt.Errorf("构造 multipart 请求体失败: %w", err)
	}
	bearer, err := driveDownloadBearer(userAccessToken)
	if err != nil {
		return nil, err
	}
	reqURL := strings.TrimRight(cfg.BaseURL, "/") + apiPath
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", contentType)
	resp, err := rawHTTPClient().Do(req)
	if err != nil {
		// 不在这里一律标网络错误：host 白名单、拒绝降级跳转等本地策略错误同样经 Do 返回，
		// 由 exitCodeFor 按错误链区分（真实网络错误 exit 4，策略拒绝不可重试）
		return nil, fmt.Errorf("API 调用失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, apiMultipartMaxResponse+1))
	if err != nil {
		return nil, clierr.Network(fmt.Errorf("读取响应失败: %w", err))
	}
	if int64(len(raw)) > apiMultipartMaxResponse {
		return nil, fmt.Errorf("响应超过 %d 字节上限，已中止（避免输出被截断）", apiMultipartMaxResponse)
	}
	return &MultipartResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: raw}, nil
}
