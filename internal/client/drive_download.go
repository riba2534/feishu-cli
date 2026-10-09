package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// DriveDownload 是一次已建立的云盘文件下载（首个响应已校验，业务错误已在打开时返回）。
//
// User 与 Bot 身份统一走 Bearer 流式下载：不再经 SDK 把整个文件读进内存（旧 Bot 路径有 100MB 上限），
// 单个分片失败按 downloadPartRetries 有界重试并从断点续传，空闲 downloadIdleTimeout 无数据视为超时，
// 写盘为同目录临时文件 + rename。
type DriveDownload struct {
	stream *downloadStream
	cancel context.CancelFunc
}

// OpenDriveFileDownload 打开云盘文件下载流。version 为空表示最新版本。
// timeout > 0 时为整个下载的总时长上限；0 表示不限总时长（仍受空闲超时约束）。
func OpenDriveFileDownload(fileToken, version, userAccessToken string, timeout time.Duration) (*DriveDownload, error) {
	if strings.TrimSpace(fileToken) == "" {
		return nil, fmt.Errorf("file_token 不能为空")
	}
	bearer, err := driveDownloadBearer(userAccessToken)
	if err != nil {
		return nil, err
	}
	ctx, cancel := downloadContext(timeout)
	stream, err := openDownloadStream(ctx, downloadStreamSpec{
		Action: "下载文件",
		URL:    buildDriveFileDownloadURLWithVersion(fileToken, version),
		Bearer: bearer,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	return &DriveDownload{stream: stream, cancel: cancel}, nil
}

// Read 实现 io.Reader（读到 EOF 表示完整且长度已校验）。
func (d *DriveDownload) Read(p []byte) (int, error) { return d.stream.Read(p) }

// Close 释放连接与 context。
func (d *DriveDownload) Close() error {
	_ = d.stream.Close()
	d.cancel()
	return nil
}

// Size 返回服务端声明的文件大小，未知时为 -1。
func (d *DriveDownload) Size() int64 { return d.stream.Size() }

// Header 返回首个成功响应的 Header。
func (d *DriveDownload) Header() http.Header { return d.stream.Header() }

// FileName 返回响应头 Content-Disposition 中的文件名（已去除路径成分），没有时返回 ""。
func (d *DriveDownload) FileName() string {
	return contentDispositionFileName(d.stream.Header())
}

// SaveTo 把下载内容原子写入 outputPath，返回写入字节数。失败时目标文件保持原样。
func (d *DriveDownload) SaveTo(outputPath string) (int64, error) {
	if err := validatePath(outputPath); err != nil {
		return 0, err
	}
	n, err := safefile.AtomicWriteFrom(outputPath, d.stream, 0o644)
	if err != nil {
		if streamErr := d.stream.Err(); streamErr != nil {
			return n, streamErr
		}
		return n, fmt.Errorf("保存文件失败: %w", err)
	}
	return n, nil
}

// contentDispositionFileName 解析 Content-Disposition 的 filename*/filename，去除路径成分。
func contentDispositionFileName(header http.Header) string {
	if header == nil {
		return ""
	}
	cd := strings.TrimSpace(header.Get("Content-Disposition"))
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	name := ""
	if v := params["filename*"]; v != "" {
		if decoded, ok := decodeRFC5987(v); ok {
			name = decoded
		}
	}
	if name == "" {
		name = params["filename"]
	}
	return sanitizeFilename(name)
}

// HashRemoteFile 流式下载远端文件并计算 SHA-256（内存占用恒定，不受文件大小影响）。
// 下载或读取失败时 fail closed，绝不只 hash 前缀。
func HashRemoteFile(fileToken, userAccessToken string) (string, error) {
	d, err := OpenDriveFileDownload(fileToken, "", userAccessToken, 0)
	if err != nil {
		return "", fmt.Errorf("下载远端文件以计算哈希失败 (token=%s): %w", fileToken, err)
	}
	defer d.Close()
	h := sha256.New()
	if _, err := io.Copy(h, d); err != nil {
		return "", fmt.Errorf("读取远端文件流以计算哈希失败 (token=%s): %w", fileToken, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func buildDriveFileDownloadURLWithVersion(fileToken, version string) string {
	u := buildDriveFileDownloadURL(fileToken)
	if strings.TrimSpace(version) == "" {
		return u
	}
	return u + "?version=" + url.QueryEscape(version)
}

// driveDownloadBearer 返回下载请求使用的 Bearer：显式 User Token 优先，否则使用（进程内缓存的）tenant token。
func driveDownloadBearer(userAccessToken string) (string, error) {
	if t := strings.TrimSpace(userAccessToken); t != "" {
		return t, nil
	}
	return cachedTenantAccessToken()
}

// tenantTokenCache 进程级缓存 tenant_access_token，避免 pull/status 批量下载时每个文件都换一次 token。
var tenantTokenCache struct {
	mu      sync.Mutex
	key     string
	token   string
	expires time.Time
}

// tenantTokenRefreshMargin 提前刷新的余量：剩余有效期不足该值时重新换取。
const tenantTokenRefreshMargin = 5 * time.Minute

func cachedTenantAccessToken() (string, error) {
	cfg := config.Get()
	if cfg == nil {
		return "", fmt.Errorf("配置未初始化")
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return "", fmt.Errorf("缺少 app_id 或 app_secret 配置")
	}
	if err := config.CheckBaseURL(cfg.BaseURL); err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = config.OfficialFeishuOpen
	}
	key := cfg.AppID + "\x00" + secretFingerprint(cfg.AppSecret) + "\x00" + baseURL
	tenantTokenCache.mu.Lock()
	defer tenantTokenCache.mu.Unlock()
	if tenantTokenCache.token != "" && tenantTokenCache.key == key && time.Until(tenantTokenCache.expires) > tenantTokenRefreshMargin {
		return tenantTokenCache.token, nil
	}
	token, expireSec, err := fetchTenantTokenViaSDKPath(baseURL, cfg.AppID, cfg.AppSecret)
	if err != nil {
		return "", fmt.Errorf("获取 tenant token 失败: %w", err)
	}
	tenantTokenCache.key = key
	tenantTokenCache.token = token
	tenantTokenCache.expires = time.Now().Add(time.Duration(expireSec) * time.Second)
	return token, nil
}

// fetchTenantTokenViaSDKPath 与 SDK 取 token 走同一条链路：POST {base}/open-apis/auth/v3/tenant_access_token/internal，
// 经 wrapSDKHTTPClient（官方 host 由 legacyInternalTokenBridge 转到 Accounts OAuth v3；host 白名单、重定向剥离凭证）。
func fetchTenantTokenViaSDKPath(baseURL, appID, appSecret string) (string, int, error) {
	payload, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	req, err := http.NewRequestWithContext(Context(), http.MethodPost, baseURL+legacyTenantTokenInternalPath, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := wrapSDKHTTPClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}
	var out struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, ParseAPIResponse("获取 tenant token", resp.StatusCode, resp.Header, body)
	}
	if out.Code != 0 || strings.TrimSpace(out.Token) == "" {
		return "", 0, fmt.Errorf("code=%d, msg=%s", out.Code, out.Msg)
	}
	if out.Expire <= 0 {
		out.Expire = 1800
	}
	return out.Token, out.Expire, nil
}

// resetTenantTokenCacheForTest 测试用：清空 tenant token 缓存。
func resetTenantTokenCacheForTest() {
	tenantTokenCache.mu.Lock()
	tenantTokenCache.key, tenantTokenCache.token, tenantTokenCache.expires = "", "", time.Time{}
	tenantTokenCache.mu.Unlock()
}

// buildOpenAPIURL 拼接 Open API 完整 URL（base_url 为空时使用官方飞书域名）。
func buildOpenAPIURL(apiPath string) string {
	base := ""
	if cfg := config.Get(); cfg != nil {
		base = strings.TrimRight(cfg.BaseURL, "/")
	}
	if base == "" {
		base = config.OfficialFeishuOpen
	}
	return base + apiPath
}
