package auth

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/config"
	"os"
)

// logf 输出日志到 stderr，避免污染 stdout 的 JSON 输出
func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// ResolveUserAccessToken 按优先级链获取 user_access_token，支持自动刷新
//
// 优先级:
//  1. flagValue（--user-access-token 参数）
//  2. FEISHU_USER_ACCESS_TOKEN 环境变量
//  3. token.json（access_token 有效直接返回；过期则用 refresh_token 刷新）
//  4. configValue（config.yaml 静态配置）
//  5. 全部为空 → 返回错误
func ResolveUserAccessToken(flagValue, configValue, appID, appSecret, baseURL string) (string, error) {
	// 1. 命令行参数
	if flagValue != "" {
		return flagValue, nil
	}

	// 2. 环境变量
	if envToken := os.Getenv("FEISHU_USER_ACCESS_TOKEN"); envToken != "" {
		return envToken, nil
	}

	// 3. token.json
	var tokenFileExpired bool
	token, err := LoadToken()
	if err == nil && token != nil {
		if token.IsAccessTokenValid() {
			return token.AccessToken, nil
		}

		// access_token 过期，尝试刷新
		if token.IsRefreshTokenValid() {
			if strings.TrimSpace(baseURL) == "" {
				baseURL = config.ResolveAPIBaseURL(nil)
			}
			logf("[自动刷新] Access Token 已过期，正在刷新...")
			newToken, refreshErr := RefreshAccessToken(token.RefreshToken, appID, appSecret, baseURL)
			if refreshErr != nil {
				logf("[自动刷新] 刷新失败: %v", refreshErr)
			} else {
				if saveErr := SaveToken(newToken); saveErr != nil {
					logf("[自动刷新] Token 已刷新但保存失败: %v", saveErr)
				} else {
					logf("[自动刷新] 刷新成功，新 Token 有效期至 %s", newToken.ExpiresAt.Format("2006-01-02 15:04:05"))
				}
				// 无论保存是否成功，刷新后的 token 都可以使用
				return newToken.AccessToken, nil
			}
		}
		tokenFileExpired = true // token.json 存在但所有 token 都过期了
	}

	// 4. 配置文件
	if configValue != "" {
		return configValue, nil
	}

	// 5. 区分"从未登录"和"登录过期"
	if tokenFileExpired {
		return "", fmt.Errorf("User Access Token 已过期（access_token 和 refresh_token 均已失效）。\n" +
			"请重新登录: feishu-cli auth login")
	}
	return "", fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n" +
		"  1. OAuth 登录: feishu-cli auth login\n" +
		"  2. 命令行参数: --user-access-token <token>\n" +
		"  3. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
		"  4. 配置文件: user_access_token: <token>")
}
