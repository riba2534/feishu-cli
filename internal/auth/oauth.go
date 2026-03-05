package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OAuthConfig OAuth 登录配置
type OAuthConfig struct {
	AppID     string
	AppSecret string
	BaseURL   string // 飞书 API 基础地址，默认 https://open.feishu.cn
	Port      int    // 本地回调服务器端口，默认 3000
	Scope     string // 权限范围（空格分隔）
}

// oauthTokenResponse 飞书 OAuth token 响应
type oauthTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int64  `json:"expires_in"`
		RefreshExpiresIn int64  `json:"refresh_expires_in"`
		Scope            string `json:"scope"`
	} `json:"data"`
}

// Login 执行 OAuth 登录流程
// 1. 启动本地 HTTP 服务器接收回调
// 2. 打开浏览器跳转到飞书授权页
// 3. 等待用户授权后获取 authorization_code
// 4. 用 code 换取 user_access_token
func Login(cfg OAuthConfig) (*UserToken, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.feishu.cn"
	}
	if cfg.Port == 0 {
		cfg.Port = 3000
	}

	// 生成随机 state 参数防止 CSRF
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("生成 state 参数失败: %w", err)
	}

	// 用于接收 authorization_code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", cfg.Port)

	// 启动本地 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// 验证 state 参数
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("state 参数不匹配，可能存在 CSRF 攻击")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("授权失败", "state 参数不匹配，请重试。"))
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("未收到 authorization_code")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("授权失败", "未收到授权码，请重试。"))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, htmlPage("授权成功", "您已成功登录飞书，可以关闭此页面。"))
		codeChan <- code
	})

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("启动本地服务器失败（端口 %d 可能被占用）: %w", cfg.Port, err)
	}

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("本地服务器异常: %w", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	// 构建授权 URL
	authURL := buildAuthURL(cfg.BaseURL, cfg.AppID, redirectURI, state, cfg.Scope)

	fmt.Println("正在打开浏览器进行飞书授权...")
	fmt.Printf("如果浏览器未自动打开，请手动访问以下地址:\n\n%s\n\n", authURL)

	// 尝试打开浏览器
	openBrowser(authURL)

	fmt.Println("等待授权回调...")

	// 等待授权回调（超时 5 分钟）
	var code string
	select {
	case code = <-codeChan:
		// 收到 code
	case err := <-errChan:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("授权超时（5 分钟），请重新执行 login 命令")
	}

	fmt.Println("收到授权码，正在获取令牌...")

	// 用 code 换取 token
	token, err := exchangeToken(cfg, code, redirectURI)
	if err != nil {
		return nil, err
	}

	return token, nil
}

// RefreshAccessToken 使用 refresh_token 刷新 access_token
func RefreshAccessToken(cfg OAuthConfig, refreshToken string) (*UserToken, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.feishu.cn"
	}

	apiURL := cfg.BaseURL + "/open-apis/authen/v1/oidc/refresh_access_token"

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}
	bodyBytes, _ := json.Marshal(body)

	// 先获取 app_access_token
	appToken, err := getAppAccessToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("获取 app_access_token 失败: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+appToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("刷新 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if tokenResp.Code != 0 {
		return nil, fmt.Errorf("刷新 token 失败: code=%d, msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	now := time.Now().Unix()
	token := &UserToken{
		AccessToken:      tokenResp.Data.AccessToken,
		RefreshToken:     tokenResp.Data.RefreshToken,
		ExpiresAt:        now + tokenResp.Data.ExpiresIn,
		RefreshExpiresAt: now + tokenResp.Data.RefreshExpiresIn,
		TokenType:        tokenResp.Data.TokenType,
		Scope:            tokenResp.Data.Scope,
	}

	return token, nil
}

// exchangeToken 用 authorization_code 换取 token
func exchangeToken(cfg OAuthConfig, code, redirectURI string) (*UserToken, error) {
	apiURL := cfg.BaseURL + "/open-apis/authen/v1/oidc/access_token"

	body := map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}
	bodyBytes, _ := json.Marshal(body)

	// 先获取 app_access_token
	appToken, err := getAppAccessToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("获取 app_access_token 失败: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+appToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("换取 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if tokenResp.Code != 0 {
		return nil, fmt.Errorf("换取 token 失败: code=%d, msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	now := time.Now().Unix()
	token := &UserToken{
		AccessToken:      tokenResp.Data.AccessToken,
		RefreshToken:     tokenResp.Data.RefreshToken,
		ExpiresAt:        now + tokenResp.Data.ExpiresIn,
		RefreshExpiresAt: now + tokenResp.Data.RefreshExpiresIn,
		TokenType:        tokenResp.Data.TokenType,
		Scope:            tokenResp.Data.Scope,
	}

	return token, nil
}

// appAccessTokenResponse app_access_token 响应
type appAccessTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	AppAccessToken    string `json:"app_access_token"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// getAppAccessToken 获取 app_access_token（用于 OAuth token 交换）
func getAppAccessToken(cfg OAuthConfig) (string, error) {
	apiURL := cfg.BaseURL + "/open-apis/auth/v3/app_access_token/internal"

	body := map[string]string{
		"app_id":     cfg.AppID,
		"app_secret": cfg.AppSecret,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取 app_access_token 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var tokenResp appAccessTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if tokenResp.Code != 0 {
		return "", fmt.Errorf("获取 app_access_token 失败: code=%d, msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	return tokenResp.AppAccessToken, nil
}

// buildAuthURL 构建飞书 OAuth 授权 URL
func buildAuthURL(baseURL, appID, redirectURI, state, scope string) string {
	params := url.Values{}
	params.Set("app_id", appID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	if scope != "" {
		params.Set("scope", scope)
	}
	return baseURL + "/open-apis/authen/v1/authorize?" + params.Encode()
}

// generateState 生成随机 state 参数
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser 尝试打开系统默认浏览器
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}

// htmlPage 生成简单的 HTML 页面
func htmlPage(title, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s - 飞书 CLI</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f5f6f7}
.card{background:#fff;padding:40px;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,.1);text-align:center;max-width:400px}
h1{color:#1f2329;margin-bottom:16px}p{color:#646a73;font-size:16px}</style>
</head><body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, title, message)
}
