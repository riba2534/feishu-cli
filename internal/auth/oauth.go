package auth

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
)

const (
	DefaultPort  = 9768
	CallbackPath = "/callback"
)

// LoginOptions 登录选项
type LoginOptions struct {
	Port        int
	Manual      bool // 强制手动粘贴模式
	NoManual    bool // 强制本地回调模式
	AppID       string
	AppSecret   string
	BaseURL     string // API 地址
	AuthBaseURL string
	Scopes      string // OAuth scope，空格分隔
}

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

// AuthURLResult 授权 URL 生成结果
type AuthURLResult struct {
	AuthURL     string `json:"auth_url"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

// GenerateAuthURL 生成 OAuth 授权 URL（非阻塞，无副作用）
// 配合 auth callback 命令实现两步式非交互登录
func GenerateAuthURL(opts LoginOptions) (*AuthURLResult, error) {
	if opts.AppID == "" || opts.AppSecret == "" {
		return nil, fmt.Errorf("缺少 app_id 或 app_secret，请先配置:\n" +
			"  环境变量: export FEISHU_APP_ID=xxx && export FEISHU_APP_SECRET=xxx\n" +
			"  配置文件: feishu-cli config init")
	}

	state, redirectURI, authURL, err := buildAuthURL(opts)
	if err != nil {
		return nil, err
	}

	return &AuthURLResult{
		AuthURL:     authURL,
		State:       state,
		RedirectURI: redirectURI,
	}, nil
}

// buildAuthURL 构造授权 URL（内部复用）
func buildAuthURL(opts LoginOptions) (state, redirectURI, authURL string, err error) {
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", "", fmt.Errorf("生成 state 失败: %w", err)
	}
	state = hex.EncodeToString(stateBytes)

	redirectURI = fmt.Sprintf("http://127.0.0.1:%d%s", opts.Port, CallbackPath)
	authBaseURL := opts.AuthBaseURL
	if authBaseURL == "" {
		authBaseURL = config.ResolveAuthBaseURL(nil)
	}
	authAuthorizeURL := config.ResolveOAuthAuthorizeURL(&config.Config{AuthBaseURL: authBaseURL})

	authURL = fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		authAuthorizeURL,
		url.QueryEscape(opts.AppID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
	if opts.Scopes != "" {
		authURL += "&scope=" + url.QueryEscape(opts.Scopes)
	}

	return state, redirectURI, authURL, nil
}

// Login 执行 OAuth 登录流程
func Login(opts LoginOptions) (*TokenStore, error) {
	if opts.AppID == "" || opts.AppSecret == "" {
		return nil, fmt.Errorf("缺少 app_id 或 app_secret，请先配置:\n" +
			"  环境变量: export FEISHU_APP_ID=xxx && export FEISHU_APP_SECRET=xxx\n" +
			"  配置文件: feishu-cli config init")
	}

	if opts.BaseURL == "" {
		opts.BaseURL = config.ResolveAPIBaseURL(nil)
	}
	if opts.AuthBaseURL == "" {
		opts.AuthBaseURL = config.ResolveAuthBaseURL(nil)
	}

	state, redirectURI, authURL, err := buildAuthURL(opts)
	if err != nil {
		return nil, err
	}

	// 选择模式
	useManual := opts.Manual || (!opts.NoManual && !isLocalEnvironment())

	var code string
	var loginErr error

	if useManual {
		code, loginErr = loginManual(authURL, state)
	} else {
		code, loginErr = loginLocal(authURL, state, opts.Port)
	}
	if loginErr != nil {
		return nil, loginErr
	}

	// 用 code 换 token
	token, err := ExchangeToken(code, opts.AppID, opts.AppSecret, redirectURI, opts.BaseURL)
	if err != nil {
		return nil, err
	}

	// 保存 token
	if err := SaveToken(token); err != nil {
		return nil, err
	}

	return token, nil
}

// loginLocal 本地回调模式
func loginLocal(authURL, state string, port int) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("error") != "" {
			sendErr(fmt.Errorf("授权失败: %s - %s", q.Get("error"), q.Get("error_description")))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<html><body><h2>授权失败</h2><p>%s</p></body></html>", html.EscapeString(q.Get("error_description")))
			return
		}

		if q.Get("state") != state {
			sendErr(fmt.Errorf("state 不匹配，可能存在 CSRF 攻击"))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<html><body><h2>授权失败</h2><p>安全校验未通过</p></body></html>")
			return
		}

		code := q.Get("code")
		if code == "" {
			sendErr(fmt.Errorf("回调中缺少 code 参数"))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<html><body><h2>授权失败</h2><p>缺少 code 参数</p></body></html>")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><body style="text-align:center;padding-top:100px;font-family:sans-serif">
<h2>✓ 授权成功</h2><p>可以关闭此页面，返回终端查看结果。</p></body></html>`)

		select {
		case codeCh <- code:
		default:
		}
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", fmt.Errorf("启动本地服务器失败（端口 %d 可能被占用）: %w\n提示: 使用 --port 指定其他端口", port, err)
	}

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("本地服务器错误: %w", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	logf("正在启动本地授权服务器 (http://127.0.0.1:%d%s)...", port, CallbackPath)

	// 尝试打开浏览器
	if err := openBrowser(authURL); err != nil {
		logf("无法自动打开浏览器，请手动访问以下链接:")
	} else {
		logf("正在打开浏览器，请在飞书页面完成授权...")
		logf("\n如果浏览器未自动打开，请手动访问以下链接:")
	}
	logf("  %s\n", authURL)
	logf("等待授权回调...")

	// 等待回调或超时
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-timer.C:
		return "", fmt.Errorf("授权超时（2 分钟），请重试")
	}
}

// loginManual 手动粘贴模式
func loginManual(authURL, state string) (string, error) {
	logf("检测到远程环境，使用手动授权模式。")
	logf("\n请在浏览器中打开以下链接完成授权:")
	logf("  %s", authURL)
	logf("\n授权完成后，浏览器会跳转到一个无法访问的页面（这是正常的）。")
	logf("请从浏览器地址栏复制完整 URL，粘贴到此处:")

	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprint(os.Stderr, "> ")
		rawURL, err := readLine()
		if err != nil {
			return "", fmt.Errorf("读取输入失败: %w", err)
		}

		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			logf("输入为空，请粘贴完整的回调 URL")
			continue
		}

		code, err := ParseCallbackURL(rawURL, state)
		if err != nil {
			logf("URL 解析失败: %v\n请重试:", err)
			continue
		}

		return code, nil
	}

	return "", fmt.Errorf("多次输入无效，请重新执行 feishu-cli auth login")
}

// readLine 从 stdin 读取一行
func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ParseCallbackURL 从回调 URL 中解析 code 并校验 state
func ParseCallbackURL(rawURL, expectedState string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL 格式错误: %w", err)
	}

	q := u.Query()

	if errParam := q.Get("error"); errParam != "" {
		return "", fmt.Errorf("授权失败: %s - %s", errParam, q.Get("error_description"))
	}

	gotState := q.Get("state")
	if gotState != expectedState {
		return "", fmt.Errorf("state 不匹配，请确保粘贴的是本次授权的回调 URL")
	}

	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("URL 中缺少 code 参数，请确保粘贴完整的回调 URL")
	}

	return code, nil
}

// ExchangeToken 用授权码换取 token
func ExchangeToken(code, appID, appSecret, redirectURI, baseURL string) (*TokenStore, error) {
	tokenURL := baseURL + "/open-apis/authen/v2/oauth/token"

	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     appID,
		"client_secret": appSecret,
		"redirect_uri":  redirectURI,
	}

	return doTokenRequest(tokenURL, body)
}

// RefreshAccessToken 用 refresh_token 刷新 access_token
func RefreshAccessToken(refreshToken, appID, appSecret, baseURL string) (*TokenStore, error) {
	tokenURL := baseURL + "/open-apis/authen/v2/oauth/token"

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     appID,
		"client_secret": appSecret,
	}

	return doTokenRequest(tokenURL, body)
}

// doTokenRequest 执行 token 请求
func doTokenRequest(tokenURL string, body map[string]string) (*TokenStore, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(tokenURL, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("请求 token 端点失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token 端点返回 HTTP %d: %s", resp.StatusCode, string(respBody))
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

	now := time.Now()
	store := &TokenStore{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	if tokenResp.RefreshExpiresIn > 0 {
		store.RefreshExpiresAt = now.Add(time.Duration(tokenResp.RefreshExpiresIn) * time.Second)
	}

	return store, nil
}
