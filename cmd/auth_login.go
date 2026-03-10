package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录授权（获取 User Access Token）",
	Long: `通过 OAuth 2.0 Authorization Code Flow 完成用户授权。

本地桌面环境（默认）:
  自动启动本地 HTTP 服务器，打开浏览器完成授权回调。

远程 SSH 环境（自动检测或 --manual）:
  打印授权 URL，用户在本机浏览器打开，授权后复制回调 URL 粘贴到终端。

非交互模式（--print-url，推荐 AI Agent 使用）:
  仅输出授权 URL 和 state 的 JSON，不启动服务器也不等待输入。
  配合 auth callback 命令完成两步式登录。

Token 保存位置: ~/.feishu-cli/token.json

前置条件:
  在飞书开放平台 → 应用详情 → 安全设置 → 重定向 URL 中添加:
  http://127.0.0.1:9768/callback

示例:
  # 自动检测环境
  feishu-cli auth login

  # 强制手动模式（SSH 远程环境）
  feishu-cli auth login --manual

  # 指定端口
  feishu-cli auth login --port 8080

  # 非交互模式（AI Agent 推荐）
  feishu-cli auth login --print-url
  # 然后用户在浏览器完成授权后执行:
  feishu-cli auth callback "<回调URL>" --state "<state>"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		cfg := config.Get()

		port, _ := cmd.Flags().GetInt("port")
		manual, _ := cmd.Flags().GetBool("manual")
		noManual, _ := cmd.Flags().GetBool("no-manual")
		scopes, _ := cmd.Flags().GetString("scopes")
		printURL, _ := cmd.Flags().GetBool("print-url")

		opts := auth.LoginOptions{
			Port:        port,
			Manual:      manual,
			NoManual:    noManual,
			AppID:       cfg.AppID,
			AppSecret:   cfg.AppSecret,
			BaseURL:     config.ResolveAPIBaseURL(cfg),
			AuthBaseURL: config.ResolveAuthBaseURL(cfg),
			Scopes:      scopes,
		}

		// 非交互模式：仅输出授权 URL 和 state
		if printURL {
			result, err := auth.GenerateAuthURL(opts)
			if err != nil {
				return err
			}
			return printJSON(result)
		}

		token, err := auth.Login(opts)
		if err != nil {
			return err
		}

		path, _ := auth.TokenPath()
		fmt.Fprintln(os.Stderr, "\n✓ 授权成功！")
		fmt.Fprintf(os.Stderr, "  Token 已保存到 %s\n", path)
		fmt.Fprintf(os.Stderr, "  Access Token 有效期至: %s\n", token.ExpiresAt.Format("2006-01-02 15:04:05"))
		if !token.RefreshExpiresAt.IsZero() {
			fmt.Fprintf(os.Stderr, "  Refresh Token 有效期至: %s\n", token.RefreshExpiresAt.Format("2006-01-02 15:04:05"))
		}
		if token.Scope != "" {
			fmt.Fprintf(os.Stderr, "  授权范围: %s\n", token.Scope)
		}

		return nil
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)

	authLoginCmd.Flags().Int("port", auth.DefaultPort, "本地回调服务器端口")
	authLoginCmd.Flags().Bool("manual", false, "强制使用手动粘贴模式")
	authLoginCmd.Flags().Bool("no-manual", false, "强制使用本地回调模式")
	authLoginCmd.Flags().String("scopes", "", "请求的 OAuth scope（空格分隔，如 \"search:docs:read search:message offline_access\"）")
	authLoginCmd.Flags().Bool("print-url", false, "仅输出授权 URL 和 state（非交互模式，用于 AI Agent）")
}
