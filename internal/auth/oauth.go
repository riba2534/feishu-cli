package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

const maxAuthResponseBytes = 1 << 20
const authHTTPTimeout = 10 * time.Second

// tokenResponse 飞书 token 端点响应
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_token_expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// RefreshAccessToken 用 refresh_token 刷新 access_token。
//
// 实现细节：
//   - 使用 application/x-www-form-urlencoded（飞书 v2 token 端点的标准 OAuth 2.0 编码）
//   - 响应缺 refresh_token / refresh_token_expires_in / scope 时，复用 oldStore 的原值
//     （OAuth 规范允许 refresh 响应不返回新 refresh_token，表示复用原值；
//     直接覆盖空字符串会导致下次过期后彻底失效——正是 issue #94 的根因）
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

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", oldStore.RefreshToken)
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(runctx.Root(), "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构造 token 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := config.NewHTTPClient(authHTTPTimeout)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 token 端点失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedAuthBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 端点返回 HTTP %d: %s", resp.StatusCode, authBodyPreview(respBody))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析 token 响应失败: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("获取 token 失败: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token 响应中缺少 access_token")
	}

	// 兜底：响应缺字段时复用原值，符合 OAuth 2.0 刷新语义（见 issue #94）
	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" {
		refreshToken = oldStore.RefreshToken
	}
	scope := tokenResp.Scope
	if scope == "" {
		scope = oldStore.Scope
	}

	now := time.Now()
	newStore := &TokenStore{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     refreshToken,
		TokenType:        tokenResp.TokenType,
		ExpiresAt:        now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
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

	return newStore, nil
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
