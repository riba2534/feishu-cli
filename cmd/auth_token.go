package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "导出当前 Access Token（让其他工具/脚本复用 feishu-cli 的 token 管理）",
	Long: `打印当前可用的 Access Token，让你能用任何 HTTP 工具（curl/wget/Python requests/...）
调飞书 OpenAPI，而不用自己实现 OAuth Device Flow / Token 刷新等。

身份选择 (--as):
  user  打印 User Access Token（自动触发刷新，永远是有效的）
  bot   打印 Tenant Access Token（App 身份，2 小时有效）
  auto  默认。先尝试 user，无则回退 bot

示例:
  # 直接拿 user token 给 curl 用
  TOKEN=$(feishu-cli auth token --as user)
  curl -H "Authorization: Bearer $TOKEN" \
    https://open.feishu.cn/open-apis/contact/v3/users/me

  # 拿 bot token
  feishu-cli auth token --as bot

  # 默认 auto（user 优先回退 bot）
  feishu-cli auth token`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		as, _ := cmd.Flags().GetString("as")
		as = strings.ToLower(strings.TrimSpace(as))

		switch as {
		case "user":
			token, err := resolveRequiredUserToken(cmd)
			if err != nil {
				return fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`）: %w", err)
			}
			fmt.Println(token)
			return nil

		case "bot", "tenant", "app":
			token, err := fetchTenantAccessToken()
			if err != nil {
				return err
			}
			fmt.Println(token)
			return nil

		case "", "auto":
			if t := resolveOptionalUserTokenWithFallback(cmd); t != "" {
				fmt.Println(t)
				return nil
			}
			token, err := fetchTenantAccessToken()
			if err != nil {
				return fmt.Errorf("auto 模式获取 token 失败（user 无 token + bot 拿不到）: %w", err)
			}
			fmt.Println(token)
			return nil

		default:
			return fmt.Errorf("--as 仅支持 user|bot|auto，得到 %q", as)
		}
	},
}

// fetchTenantAccessToken 用 App ID + App Secret 换 tenant_access_token
// 端点: POST /open-apis/auth/v3/tenant_access_token/internal
func fetchTenantAccessToken() (string, error) {
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return "", fmt.Errorf("缺少 app_id 或 app_secret 配置")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	reqBody, _ := json.Marshal(map[string]string{
		"app_id":     cfg.AppID,
		"app_secret": cfg.AppSecret,
	})
	url := baseURL + "/open-apis/auth/v3/tenant_access_token/internal"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 tenant_access_token 失败: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("解析 tenant_access_token 响应失败: %w", err)
	}
	if body.Code != 0 {
		return "", fmt.Errorf("获取 tenant_access_token 失败: code=%d msg=%s", body.Code, body.Msg)
	}
	if body.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书未返回 tenant_access_token")
	}
	return body.TenantAccessToken, nil
}

func init() {
	authCmd.AddCommand(authTokenCmd)
	authTokenCmd.Flags().String("as", "auto", "身份: user | bot | auto")
	authTokenCmd.Flags().String("user-access-token", "", "显式传入 User Token（覆盖 --as）")
}
