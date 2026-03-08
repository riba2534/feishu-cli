package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var refreshMu sync.Mutex

// GetUserAccessToken 按优先级获取 user_access_token:
// 1. CLI flag --user-access-token
// 2. config.yaml user_access_token（如过期且有 refresh_token 则自动刷新）
// 3. 环境变量 FEISHU_USER_ACCESS_TOKEN
func GetUserAccessToken(cmd *cobra.Command) string {
	if token, _ := cmd.Flags().GetString("user-access-token"); token != "" {
		return token
	}

	cfg := config.Get()

	// 检查 token 是否过期，有 refresh_token 则自动刷新
	if cfg.RefreshToken != "" && cfg.TokenExpireAt > 0 {
		now := time.Now().Unix()
		// 提前 5 分钟刷新，避免临界过期
		if now > cfg.TokenExpireAt-300 {
			if newToken, err := refreshAccessToken(cfg); err == nil {
				return newToken
			}
			// 刷新失败，继续用旧 token 试试
		}
	}

	if token := cfg.UserAccessToken; token != "" {
		return token
	}
	return os.Getenv("FEISHU_USER_ACCESS_TOKEN")
}

// RequireUserAccessToken 获取 user_access_token，为空时返回错误
func RequireUserAccessToken(cmd *cobra.Command) (string, error) {
	token := GetUserAccessToken(cmd)
	if token == "" {
		return "", fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n" +
			"  1. 运行 feishu-cli auth login 进行 OAuth 授权（推荐）\n" +
			"  2. 命令行参数: --user-access-token <token>\n" +
			"  3. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
			"  4. 配置文件: user_access_token: <token>")
	}
	return token, nil
}

type refreshResponse struct {
	Code int `json:"code"`
	Data struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		RefreshExpiresIn int64  `json:"refresh_expires_in"`
	} `json:"data"`
	Msg string `json:"msg"`
}

type appTokenResponse struct {
	Code           int    `json:"code"`
	AppAccessToken string `json:"app_access_token"`
	Msg            string `json:"msg"`
}

func refreshAccessToken(cfg *config.Config) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	// 双检锁：可能其他 goroutine 已经刷新了
	cfg = config.Get()
	if cfg.TokenExpireAt > 0 && time.Now().Unix() <= cfg.TokenExpireAt-300 {
		return cfg.UserAccessToken, nil
	}

	// 1. 获取 app_access_token
	appToken, err := getAppAccessToken(cfg)
	if err != nil {
		return "", fmt.Errorf("获取 app_access_token 失败: %w", err)
	}

	// 2. 用 refresh_token 换新 token
	body := fmt.Sprintf(`{"grant_type":"refresh_token","refresh_token":"%s"}`, cfg.RefreshToken)
	req, err := http.NewRequest("POST", cfg.BaseURL+"/open-apis/authen/v1/oidc/refresh_access_token",
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求刷新接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result refreshResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析刷新响应失败: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("刷新 token 失败 (code=%d): %s", result.Code, result.Msg)
	}

	newRefresh := result.Data.RefreshToken
	if newRefresh == "" {
		newRefresh = cfg.RefreshToken
	}
	expireAt := time.Now().Unix() + result.Data.ExpiresIn

	// 3. 写回配置文件
	if err := config.SaveToken(result.Data.AccessToken, newRefresh, expireAt); err != nil {
		fmt.Fprintf(os.Stderr, "警告: token 已刷新但写入配置失败: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "已自动刷新 user_access_token\n")
	return result.Data.AccessToken, nil
}

func getAppAccessToken(cfg *config.Config) (string, error) {
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
	var result appTokenResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("code=%d: %s", result.Code, result.Msg)
	}
	return result.AppAccessToken, nil
}
