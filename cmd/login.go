package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "通过 OAuth 登录飞书，获取用户令牌",
	Long: `通过 OAuth 2.0 授权码流程登录飞书，获取 user_access_token。

登录后，feishu-cli 将以用户身份调用飞书 API，无需将应用逐个添加为文档协作者。

前置条件:
  1. 已配置 FEISHU_APP_ID 和 FEISHU_APP_SECRET
  2. 在飞书开放平台的应用配置中添加重定向 URL: http://localhost:<port>/callback

示例:
  # 使用默认端口 (3000) 登录
  feishu-cli login

  # 指定端口和权限范围
  feishu-cli login --port 8080 --scope "docx:document wiki:wiki:readonly"

  # 查看当前登录状态
  feishu-cli login --status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		statusOnly, _ := cmd.Flags().GetBool("status")
		if statusOnly {
			return showLoginStatus()
		}

		cfg := config.Get()
		if cfg.AppID == "" || cfg.AppSecret == "" {
			return fmt.Errorf("缺少 app_id 或 app_secret 配置，请先配置后再登录:\n" +
				"  export FEISHU_APP_ID=cli_xxx\n" +
				"  export FEISHU_APP_SECRET=xxx")
		}

		port, _ := cmd.Flags().GetInt("port")
		scope, _ := cmd.Flags().GetString("scope")

		oauthCfg := auth.OAuthConfig{
			AppID:     cfg.AppID,
			AppSecret: cfg.AppSecret,
			BaseURL:   cfg.BaseURL,
			Port:      port,
			Scope:     scope,
		}

		token, err := auth.Login(oauthCfg)
		if err != nil {
			return fmt.Errorf("登录失败: %w", err)
		}

		if err := auth.SaveToken(token); err != nil {
			return fmt.Errorf("保存令牌失败: %w", err)
		}

		fmt.Println("登录成功！用户令牌已保存。")
		fmt.Printf("  Access Token 有效期至: %s\n", time.Unix(token.ExpiresAt, 0).Format("2006-01-02 15:04:05"))
		fmt.Printf("  Refresh Token 有效期至: %s\n", time.Unix(token.RefreshExpiresAt, 0).Format("2006-01-02 15:04:05"))
		if token.Scope != "" {
			fmt.Printf("  授权范围: %s\n", token.Scope)
		}
		fmt.Println("\n后续命令将自动使用用户身份调用 API（可通过 --token-mode 切换）。")

		return nil
	},
}

func showLoginStatus() error {
	token, err := auth.LoadToken()
	if err != nil {
		return fmt.Errorf("读取令牌失败: %w", err)
	}

	if token == nil {
		fmt.Println("当前未登录。请执行 feishu-cli login 进行登录。")
		return nil
	}

	fmt.Println("当前登录状态:")
	if token.IsValid() {
		fmt.Println("  状态: 已登录（令牌有效）")
	} else if token.IsRefreshable() {
		fmt.Println("  状态: 令牌已过期（可自动刷新）")
	} else {
		fmt.Println("  状态: 令牌已过期（需重新登录）")
	}
	fmt.Printf("  Access Token 有效期至: %s\n", time.Unix(token.ExpiresAt, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("  Refresh Token 有效期至: %s\n", time.Unix(token.RefreshExpiresAt, 0).Format("2006-01-02 15:04:05"))
	if token.Scope != "" {
		fmt.Printf("  授权范围: %s\n", token.Scope)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().Int("port", 3000, "本地回调服务器端口")
	loginCmd.Flags().String("scope", "", "权限范围（空格分隔，如 \"docx:document wiki:wiki:readonly\"）")
	loginCmd.Flags().Bool("status", false, "查看当前登录状态")
}
