package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

const maxAuthResponseBytes = 1 << 20
const authHTTPTimeout = 10 * time.Second

// tokenResponse 飞书 token 端点响应
type tokenResponse struct {
	Code             int    `json:"code"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_token_expires_in"`
	Scope            string `json:"scope"`
	StatusMessage    string `json:"status_message"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Msg              string `json:"msg"`
}

// refresh_token 刷新失败的业务码分类（对齐官方 lark-cli errclass/codemeta.go）。
const (
	refreshCodeLegacyFormat = 20026 // refresh_token 无效 / 旧版格式（终态）
	refreshCodeExpired      = 20037 // refresh_token 已过期（终态）
	refreshCodeServerError  = 20050 // 刷新端点临时错误（可重试一次）
	refreshCodeRevoked      = 20064 // refresh_token 已被吊销（终态）
	refreshCodeReused       = 20073 // refresh_token 已被使用（终态）
)

// refreshMaxAttempts 刷新请求最多尝试次数：可重试错误（20050、5xx、传输错误）重试一次。
const refreshMaxAttempts = 2

// refreshRetryDelay 两次刷新尝试之间的等待，测试可调小。
var refreshRetryDelay = 300 * time.Millisecond

// isTerminalRefreshCode 报告业务码是否表示 refresh_token 已不可用（只能重新登录）。
func isTerminalRefreshCode(code int) bool {
	switch code {
	case refreshCodeLegacyFormat, refreshCodeExpired, refreshCodeRevoked, refreshCodeReused:
		return true
	}
	return false
}

func terminalRefreshReason(code int) string {
	switch code {
	case refreshCodeLegacyFormat:
		return "refresh_token 无效（格式不被识别或为旧版格式）"
	case refreshCodeExpired:
		return "refresh_token 已过期"
	case refreshCodeRevoked:
		return "refresh_token 已被吊销"
	case refreshCodeReused:
		return "refresh_token 已被使用（可能已被其他进程或设备轮换）"
	}
	return "refresh_token 已失效"
}

// RefreshError 是 refresh_token 刷新失败的分类结果。
//
//   - Terminal：服务端判定 refresh_token 已不可用（20026/20037/20064/20073 或 invalid_grant），
//     只能重新 auth login；调用方会在 token.json 记录终态标记，避免后续命令重复发起注定失败的刷新。
//   - Retryable：服务端临时错误（20050、5xx）或传输错误，已在内部重试过一次。
//   - Uncertain：请求已发出但没拿到可解析的响应，refresh_token 可能已被服务端消耗。
//
// Error() 携带 code=<N>，client.HasAPICode 与根命令的修复建议照常生效。
type RefreshError struct {
	HTTPStatus  int
	Code        int
	OAuthError  string
	Description string
	Terminal    bool
	Retryable   bool
	Uncertain   bool
	Cause       error
}

func (e *RefreshError) Error() string {
	var b strings.Builder
	b.WriteString("刷新 access_token 失败")
	var parts []string
	if e.Code != 0 {
		parts = append(parts, fmt.Sprintf("code=%d", e.Code))
	}
	if e.OAuthError != "" {
		parts = append(parts, "error="+e.OAuthError)
	}
	if e.Description != "" {
		parts = append(parts, "msg="+e.Description)
	}
	if e.HTTPStatus != 0 && e.Code == 0 && e.OAuthError == "" {
		parts = append(parts, fmt.Sprintf("HTTP %d", e.HTTPStatus))
	}
	if e.Cause != nil && len(parts) == 0 {
		parts = append(parts, e.Cause.Error())
	}
	if len(parts) > 0 {
		b.WriteString(": ")
		b.WriteString(strings.Join(parts, ", "))
	}
	switch {
	case e.Terminal:
		fmt.Fprintf(&b, "；%s，无法自动续期，请重新 `feishu-cli auth login`", terminalRefreshReason(e.Code))
	case e.Uncertain:
		b.WriteString("；刷新请求已发出但未收到有效响应，refresh_token 可能已被服务端消耗：网络恢复后重试；若随后报 20073/20064，请重新 `feishu-cli auth login`")
	}
	return b.String()
}

func (e *RefreshError) Unwrap() error { return e.Cause }

// IsTerminalRefreshError 报告 err 是否为 refresh_token 终态失效。
func IsTerminalRefreshError(err error) bool {
	var re *RefreshError
	return errors.As(err, &re) && re.Terminal
}

// RefreshAccessToken 用 refresh_token 刷新 access_token。
//
// 实现细节：
//   - 使用 application/x-www-form-urlencoded（飞书 v2 token 端点的标准 OAuth 2.0 编码）
//   - 先解析响应体中的业务码/OAuth 错误，再看 HTTP 状态（飞书错误常随 HTTP 400 下发）
//   - 失败分类见 RefreshError：20050 / 5xx / 传输错误重试一次；20026/20037/20064/20073 为终态
//   - 响应缺 refresh_token / refresh_token_expires_in / scope 时，复用 oldStore 的原值
//     （OAuth 规范允许 refresh 响应不返回新 refresh_token，表示复用原值；
//     直接覆盖空字符串会导致下次过期后彻底失效——正是 issue #94 的根因）
//   - 成功响应带 status_message（服务端提示，如 scope 被裁剪）时透传到 stderr
//
// oldStore 必须非空（来自 LoadToken）。函数不会写文件，只返回新 store。
func RefreshAccessToken(oldStore *TokenStore, appID, appSecret, baseURL string) (*TokenStore, error) {
	if oldStore == nil || oldStore.RefreshToken == "" {
		return nil, fmt.Errorf("缺少 refresh_token，无法刷新")
	}
	baseURL = config.ResolveOpenBase(baseURL)
	if err := config.CheckBaseURL(baseURL); err != nil {
		return nil, err
	}
	tokenURL := baseURL + "/open-apis/authen/v2/oauth/token"
	httpClient := config.NewHTTPClient(authHTTPTimeout)

	uncertain := false
	var lastErr *RefreshError
	for attempt := 1; attempt <= refreshMaxAttempts; attempt++ {
		tokenResp, rerr := refreshOnce(httpClient, tokenURL, oldStore.RefreshToken, appID, appSecret)
		if rerr == nil {
			if tokenResp.StatusMessage != "" {
				logf("[自动刷新] 服务端提示: %s", redactAuthPreview(tokenResp.StatusMessage))
			}
			return buildRefreshedStore(oldStore, tokenResp, appID), nil
		}
		if rerr.Uncertain {
			uncertain = true
		}
		lastErr = rerr
		if !rerr.Retryable || attempt == refreshMaxAttempts || runctx.Root().Err() != nil {
			break
		}
		logf("[自动刷新] 第 %d/%d 次刷新失败（%v），重试...", attempt, refreshMaxAttempts, rerr)
		select {
		case <-time.After(refreshRetryDelay):
		case <-runctx.Root().Done():
		}
	}
	// 前一次请求已发出却没拿到响应：即使重试换来的是别的非终态错误，也要提示 refresh_token 可能已被消耗
	if uncertain && !lastErr.Terminal {
		lastErr.Uncertain = true
	}
	return nil, lastErr
}

// refreshOnce 发起一次刷新请求并分类结果。
func refreshOnce(httpClient *http.Client, tokenURL, refreshToken, appID, appSecret string) (*tokenResponse, *RefreshError) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	var wroteRequest atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) { wroteRequest.Store(true) },
	}
	ctx := httptrace.WithClientTrace(runctx.Root(), trace)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, &RefreshError{Description: "构造 token 请求失败", Cause: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		cancelled := runctx.Root().Err() != nil
		// 传输错误打网络标签（退出码 4）：网络恢复后重试即可，不应提示重新登录
		return nil, &RefreshError{
			Description: "请求 token 端点失败",
			Retryable:   !cancelled,
			Uncertain:   wroteRequest.Load(),
			Cause:       clierr.Network(fmt.Errorf("请求 token 端点失败: %w", err)),
		}
	}
	defer resp.Body.Close()

	respBody, err := readLimitedAuthBody(resp.Body)
	if err != nil {
		return nil, &RefreshError{
			HTTPStatus: resp.StatusCode, Description: "读取 token 响应失败",
			Retryable: true, Uncertain: true, Cause: err,
		}
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		// 响应不可解析：服务端可能已处理请求，按"可能已消耗"处理并重试一次
		return nil, &RefreshError{
			HTTPStatus:  resp.StatusCode,
			Description: "token 端点响应无法解析: " + authBodyPreview(respBody),
			Retryable:   true,
			Uncertain:   true,
		}
	}

	if tokenResp.Code != 0 || tokenResp.Error != "" {
		desc := tokenResp.ErrorDescription
		if desc == "" {
			desc = tokenResp.Msg
		}
		re := &RefreshError{
			HTTPStatus:  resp.StatusCode,
			Code:        tokenResp.Code,
			OAuthError:  tokenResp.Error,
			Description: redactAuthPreview(desc),
		}
		switch {
		case isTerminalRefreshCode(tokenResp.Code):
			re.Terminal = true
		case tokenResp.Code == refreshCodeServerError:
			re.Retryable = true
		case tokenResp.Code == 0 && tokenResp.Error == "invalid_grant":
			// 无业务码的标准 OAuth 错误：refresh_token 无效/过期/被吊销
			re.Terminal = true
		case tokenResp.Code == 0 && (tokenResp.Error == "server_error" || tokenResp.Error == "temporarily_unavailable"):
			re.Retryable = true
		case tokenResp.Code == 0 && resp.StatusCode >= 500:
			re.Retryable = true
		}
		return nil, re
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &RefreshError{
			HTTPStatus:  resp.StatusCode,
			Description: "token 端点返回非 200: " + authBodyPreview(respBody),
			Retryable:   resp.StatusCode >= 500,
		}
	}

	if tokenResp.AccessToken == "" {
		return nil, &RefreshError{HTTPStatus: resp.StatusCode, Description: "token 响应中缺少 access_token"}
	}
	return &tokenResp, nil
}

// buildRefreshedStore 用刷新响应构造新的 TokenStore；缺字段时复用旧值（见 issue #94）。
func buildRefreshedStore(oldStore *TokenStore, tokenResp *tokenResponse, appID string) *TokenStore {
	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" {
		refreshToken = oldStore.RefreshToken
	}
	scope := tokenResp.Scope
	if scope == "" {
		scope = oldStore.Scope
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 7200 // 服务端未返回时按 User Access Token 默认有效期 2 小时
	}
	now := time.Now()
	newStore := &TokenStore{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     refreshToken,
		TokenType:        tokenResp.TokenType,
		ExpiresAt:        now.Add(time.Duration(expiresIn) * time.Second),
		RefreshExpiresAt: oldStore.RefreshExpiresAt,
		Scope:            scope,
		AppID:            oldStore.AppID,
	}
	if newStore.AppID == "" {
		newStore.AppID = appID
	}
	// refresh_token_expires_in > 0 才更新过期时间，否则保留原值
	if tokenResp.RefreshExpiresIn > 0 {
		newStore.RefreshExpiresAt = now.Add(time.Duration(tokenResp.RefreshExpiresIn) * time.Second)
	}
	return newStore
}

var authSecretPreview = regexp.MustCompile(`(?i)(access_token|refresh_token|client_secret|app_secret|tenant_access_token|user_access_token)(["'\s:=]+)([^&"' \t\r\n,}]+)`)

func readLimitedAuthBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, int64(maxAuthResponseBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxAuthResponseBytes {
		return nil, fmt.Errorf("响应体超过 1MiB")
	}
	return body, nil
}

func redactAuthPreview(s string) string {
	return authSecretPreview.ReplaceAllString(s, `${1}${2}[REDACTED]`)
}

func truncateAuthBody(body []byte) string {
	const max = 200
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func authBodyPreview(body []byte) string {
	return truncateAuthBody([]byte(redactAuthPreview(string(body))))
}
