package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/token"
)

const (
	// OAuth 授权相关 URL
	oauthAuthorizeURL = "https://open.feishu.cn/open-apis/authen/v1/authorize"
	tokenURL          = "https://open.feishu.cn/open-apis/authen/v1/oidc/access_token"
	refreshTokenURL   = "https://open.feishu.cn/open-apis/authen/v1/oidc/refresh_access_token"
)

// OAuthClient OAuth 客户端
type OAuthClient struct {
	AppID       string
	AppSecret   string
	RedirectURI string
	HTTPClient  *http.Client
}

// NewOAuthClient 创建 OAuth 客户端
func NewOAuthClient(redirectURI string) (*OAuthClient, error) {
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("缺少 App ID 或 App Secret，请先配置应用凭证")
	}

	return &OAuthClient{
		AppID:       cfg.AppID,
		AppSecret:   cfg.AppSecret,
		RedirectURI: redirectURI,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// GetAuthorizeURL 生成授权 URL
func (c *OAuthClient) GetAuthorizeURL(state string, scope string) string {
	authURL := fmt.Sprintf("%s?app_id=%s&redirect_uri=%s&state=%s",
		oauthAuthorizeURL,
		c.AppID,
		url.QueryEscape(c.RedirectURI),
		state,
	)
	if scope != "" {
		authURL = fmt.Sprintf("%s&scope=%s", authURL, url.QueryEscape(scope))
	}
	return authURL
}

// ExchangeCodeForToken 用授权码换取 access token
func (c *OAuthClient) ExchangeCodeForToken(code string) (*token.UserToken, error) {
	reqBody := map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}

	return c.doTokenRequest(tokenURL, reqBody)
}

// RefreshUserAccessToken 刷新用户访问令牌
func (c *OAuthClient) RefreshUserAccessToken(refreshToken string) (*token.UserToken, error) {
	reqBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}

	return c.doTokenRequest(refreshTokenURL, reqBody)
}

// getAppAccessToken 获取应用级 Access Token
func (c *OAuthClient) getAppAccessToken() (string, error) {
	reqBody := map[string]string{
		"app_id":     c.AppID,
		"app_secret": c.AppSecret,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", "https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
		Expire         int    `json:"expire"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("获取 app_access_token 失败: code=%d, msg=%s", result.Code, result.Msg)
	}

	return result.AppAccessToken, nil
}

// doTokenRequest 执行 token 请求
func (c *OAuthClient) doTokenRequest(apiURL string, reqBody map[string]string) (*token.UserToken, error) {
	// 1. 获取 App Access Token
	appAccessToken, err := c.getAppAccessToken()
	if err != nil {
		return nil, err
	}

	// 2. 构造请求体
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 3. 创建请求
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", appAccessToken))

	// 4. 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp token.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if tokenResp.Code != 0 {
		return nil, fmt.Errorf("获取 token 失败: code=%d, msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	userToken := &token.UserToken{
		AccessToken:  tokenResp.Data.AccessToken,
		RefreshToken: tokenResp.Data.RefreshToken,
		ExpiresIn:    tokenResp.Data.ExpiresIn,
		ExpiresAt:    time.Now().Unix() + tokenResp.Data.ExpiresIn,
	}

	return userToken, nil
}

// GetUserAccessToken 获取用户访问令牌（优先从文件加载，过期则刷新）
func GetUserAccessToken() (string, error) {
	// 1. 检查环境变量
	if envToken := config.Get().UserAccessToken; envToken != "" {
		return envToken, nil
	}

	// 2. 检查 token 文件
	tok, err := token.LoadToken()
	if err != nil {
		return "", fmt.Errorf("未找到 User Access Token，请先运行 'feishu-cli auth login' 进行授权: %w", err)
	}

	// 3. 检查是否过期
	if !tok.IsExpired() {
		return tok.AccessToken, nil
	}

	// 4. 过期则刷新
	oauthClient, err := NewOAuthClient("")
	if err != nil {
		return "", err
	}

	newToken, err := oauthClient.RefreshUserAccessToken(tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("刷新 token 失败: %w", err)
	}

	// 5. 保存新 token
	if err := token.SaveToken(newToken); err != nil {
		return "", fmt.Errorf("保存刷新后的 token 失败: %w", err)
	}

	return newToken.AccessToken, nil
}
