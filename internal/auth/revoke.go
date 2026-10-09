package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
)

// accountsBaseFor 按 baseURL 选择 OAuth accounts 域（吊销 / 设备流端点所在域）。
func accountsBaseFor(baseURL string) string {
	return config.ResolveAccountsBase(baseURL)
}

// revokeEndpointFunc 解析吊销端点 URL，可在测试中替换指向 httptest server。
var revokeEndpointFunc = func(baseURL string) string {
	return accountsBaseFor(baseURL) + "/oauth/v1/revoke"
}

// RevokeToken 吊销一个 OAuth token（access_token 或 refresh_token）。
//
// 端点: {accounts}/oauth/v1/revoke，application/x-www-form-urlencoded，
// 参数 client_id / client_secret / token / token_type_hint。
// tokenTypeHint 可传 "access_token" / "refresh_token"，留空由服务端推断。
//
// 吊销 refresh_token 会使整个授权失效；HTTP 2xx 且业务 code=0（或空响应体）视为成功。
func RevokeToken(appID, appSecret, baseURL, token, tokenTypeHint string) error {
	if token == "" {
		return fmt.Errorf("缺少待吊销的 token")
	}
	endpoint := revokeEndpointFunc(baseURL)

	form := url.Values{}
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)
	form.Set("token", token)
	if tokenTypeHint != "" {
		form.Set("token_type_hint", tokenTypeHint)
	}

	req, err := http.NewRequestWithContext(runctx.Root(), "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("构造吊销请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := config.NewHTTPClient(10 * time.Second)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求吊销端点失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedAuthBody(resp.Body)
	if err != nil {
		return fmt.Errorf("读取吊销响应失败: %w", err)
	}

	// 先解析业务码 / OAuth 错误，再看 HTTP 状态：飞书错误常随 HTTP 4xx 下发
	var data struct {
		Code             int    `json:"code"`
		Msg              string `json:"msg"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	parsed := len(body) > 0 && json.Unmarshal(body, &data) == nil
	if parsed && data.Code != 0 {
		msg := data.Msg
		if msg == "" {
			msg = data.ErrorDescription
		}
		if msg == "" {
			msg = "未知错误"
		}
		return fmt.Errorf("吊销失败: code=%d, msg=%s", data.Code, redactAuthPreview(msg))
	}
	if parsed && data.Error != "" {
		desc := data.ErrorDescription
		if desc == "" {
			desc = data.Error
		}
		return fmt.Errorf("吊销失败: %s", redactAuthPreview(desc))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("吊销端点返回 HTTP %d: %s", resp.StatusCode, authBodyPreview(body))
	}
	// HTTP 2xx：空响应体或非标准 JSON（部分实现吊销成功不返回内容）保守视为成功
	return nil
}

// RevokeStoredToken 吊销给定 token store 中的凭证：优先吊销 refresh_token（会使整个授权失效），
// 无 refresh_token 时退回吊销 access_token。两者皆空返回 nil（无可吊销的凭证）。
func RevokeStoredToken(t *TokenStore, appID, appSecret, baseURL string) error {
	if t == nil {
		return nil
	}
	revokeToken := t.RefreshToken
	hint := "refresh_token"
	if revokeToken == "" {
		revokeToken = t.AccessToken
		hint = "access_token"
	}
	if revokeToken == "" {
		return nil
	}
	return RevokeToken(appID, appSecret, baseURL, revokeToken, hint)
}
