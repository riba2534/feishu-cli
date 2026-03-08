package cmd

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
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var loginPort int

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "通过 OAuth 授权登录飞书",
	Long:  "启动本地 HTTP 服务器，通过浏览器完成飞书 OAuth 授权，自动获取并保存 User Access Token。",
	RunE:  runAuthLogin,
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authLoginCmd.Flags().IntVar(&loginPort, "port", 52000, "本地回调服务器端口")
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	if err := config.Init(cfgFile); err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
	}

	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("缺少 app_id 或 app_secret，请先配置:\n  feishu-cli config init\n  或设置环境变量 FEISHU_APP_ID / FEISHU_APP_SECRET")
	}

	// 生成随机 state 防 CSRF
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("生成 state 失败: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", loginPort)

	// channel 接收授权码
	type authResult struct {
		code string
		err  error
	}
	codeCh := make(chan authResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			codeCh <- authResult{err: fmt.Errorf("state 不匹配，可能遭到 CSRF 攻击")}
			fmt.Fprintf(w, "<html><body><h2>授权失败：state 不匹配</h2></body></html>")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			codeCh <- authResult{err: fmt.Errorf("未收到授权码")}
			fmt.Fprintf(w, "<html><body><h2>授权失败：未收到授权码</h2></body></html>")
			return
		}
		codeCh <- authResult{code: code}
		fmt.Fprintf(w, "<html><body><h2>授权成功！</h2><p>可以关闭此页面。</p></body></html>")
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", loginPort))
	if err != nil {
		return fmt.Errorf("启动回调服务器失败（端口 %d）: %w", loginPort, err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// 构造授权 URL 并打开浏览器
	authURL := fmt.Sprintf("%s/open-apis/authen/v1/authorize?app_id=%s&redirect_uri=%s&state=%s",
		cfg.BaseURL, cfg.AppID, url.QueryEscape(redirectURI), state)

	fmt.Printf("正在打开浏览器进行授权...\n")
	fmt.Printf("如果浏览器未自动打开，请手动访问:\n%s\n\n", authURL)

	if err := exec.Command("open", authURL).Start(); err != nil {
		fmt.Printf("无法自动打开浏览器: %v\n", err)
	}

	// 等待授权码，超时 2 分钟
	fmt.Println("等待授权回调...")
	select {
	case result := <-codeCh:
		if result.err != nil {
			return result.err
		}
		return exchangeToken(cfg, result.code, redirectURI)
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("等待授权超时（2 分钟）")
	}
}

func exchangeToken(cfg *config.Config, code, redirectURI string) error {
	// 1. 获取 app_access_token
	appToken, err := fetchAppAccessToken(cfg)
	if err != nil {
		return fmt.Errorf("获取 app_access_token 失败: %w", err)
	}

	// 2. 用授权码换 user tokens
	body := fmt.Sprintf(`{"grant_type":"authorization_code","code":"%s"}`, code)
	req, err := http.NewRequest("POST", cfg.BaseURL+"/open-apis/authen/v1/oidc/access_token",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 token 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Data struct {
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			ExpiresIn        int64  `json:"expires_in"`
			RefreshExpiresIn int64  `json:"refresh_expires_in"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w\n响应: %s", err, string(respBody))
	}
	if result.Code != 0 {
		return fmt.Errorf("换取 token 失败 (code=%d): %s", result.Code, result.Msg)
	}

	expireAt := time.Now().Unix() + result.Data.ExpiresIn
	if err := config.SaveToken(result.Data.AccessToken, result.Data.RefreshToken, expireAt); err != nil {
		return fmt.Errorf("保存 token 失败: %w", err)
	}

	refreshDays := result.Data.RefreshExpiresIn / 86400

	// 获取用户信息
	name, openID := fetchUserInfo(cfg.BaseURL, result.Data.AccessToken)

	fmt.Printf("\n登录成功！\n")
	if name != "" {
		fmt.Printf("  用户: %s\n", name)
	}
	if openID != "" {
		fmt.Printf("  OpenID: %s\n", openID)
	}
	fmt.Printf("  Token 有效期: %d 小时\n", result.Data.ExpiresIn/3600)
	fmt.Printf("  Refresh Token 有效期: %d 天\n", refreshDays)
	fmt.Printf("  Token 已保存到配置文件\n")

	return nil
}

// fetchUserInfo 通过 user_access_token 获取当前用户信息
func fetchUserInfo(baseURL, accessToken string) (name, openID string) {
	req, err := http.NewRequest("GET", baseURL+"/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code int `json:"code"`
		Data struct {
			Name   string `json:"name"`
			EnName string `json:"en_name"`
			OpenID string `json:"open_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.Code != 0 {
		return "", ""
	}
	n := result.Data.Name
	if n == "" {
		n = result.Data.EnName
	}
	return n, result.Data.OpenID
}

// fetchAppAccessToken 获取 app_access_token（独立实现，不依赖 client 包）
func fetchAppAccessToken(cfg *config.Config) (string, error) {
	body := fmt.Sprintf(`{"app_id":"%s","app_secret":"%s"}`, cfg.AppID, cfg.AppSecret)
	req, err := http.NewRequest("POST", cfg.BaseURL+"/open-apis/auth/v3/app_access_token/internal",
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code           int    `json:"code"`
		AppAccessToken string `json:"app_access_token"`
		Msg            string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("code=%d: %s", result.Code, result.Msg)
	}
	return result.AppAccessToken, nil
}
