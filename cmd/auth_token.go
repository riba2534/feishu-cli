package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
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
  feishu-cli auth token

  # 把旧版未绑定 app_id 的 token.json 显式绑定到当前应用（不更换 token）
  feishu-cli auth token --bind-legacy-app --as user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		as, _ := cmd.Flags().GetString("as")
		as = strings.ToLower(strings.TrimSpace(as))
		flagUserToken, _ := cmd.Flags().GetString("user-access-token")
		if flagUserToken != "" && (as == "bot" || as == "tenant" || as == "app") {
			return clierr.Usagef("不能同时使用 --as bot 与 --user-access-token：前者要求 App/Tenant 身份，后者是显式 User Token。请去掉其中一个")
		}

		bindLegacy, _ := cmd.Flags().GetBool("bind-legacy-app")
		if bindLegacy {
			if as == "bot" || as == "tenant" || as == "app" {
				return clierr.Usagef("不能同时使用 --bind-legacy-app 与 --as bot：绑定的是 User Token 文件，与 App 身份无关")
			}
			if flagUserToken != "" {
				return clierr.Usagef("不能同时使用 --bind-legacy-app 与 --user-access-token：绑定只针对本地 token.json")
			}
			cfg := config.Get()
			if err := auth.BindLegacyToken(cfg.AppID); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "已将 token.json 绑定到当前应用 %s（未更换 token）\n", cfg.AppID)
		}

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
			userToken, err := resolveAutoUserToken(cmd)
			if err != nil {
				return err
			}
			if userToken != "" {
				fmt.Println(userToken)
				return nil
			}
			token, err := fetchTenantAccessToken()
			if err != nil {
				return fmt.Errorf("auto 模式获取 token 失败（user 无 token + bot 拿不到）: %w", err)
			}
			fmt.Println(token)
			return nil

		default:
			return clierr.Usagef("--as 仅支持 user|bot|auto，得到 %q", as)
		}
	},
}

// fetchTenantAccessToken 用 App ID + App Secret 换 tenant access token。
// 端点: POST {accounts}/oauth/v3/token ，grant_type=client_credentials。
func fetchTenantAccessToken() (string, error) {
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return "", fmt.Errorf("缺少 app_id 或 app_secret 配置")
	}
	return auth.FetchTenantAccessToken(cfg.AppID, cfg.AppSecret, cfg.BaseURL)
}

func init() {
	authCmd.AddCommand(authTokenCmd)
	authTokenCmd.Flags().String("as", "auto", "身份: user | bot | auto")
	authTokenCmd.Flags().String("user-access-token", "", "显式传入 User Token（不可与 --as bot 同时使用）")
	authTokenCmd.Flags().Bool("bind-legacy-app", false, "将未绑定 app_id 的 token.json 绑定到当前应用（不更换 token）")
}
