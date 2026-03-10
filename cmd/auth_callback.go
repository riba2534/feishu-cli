package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authCallbackCmd = &cobra.Command{
	Use:   "callback <callback_url>",
	Short: "处理 OAuth 授权回调（配合 --print-url 使用）",
	Long: `处理 OAuth 授权回调 URL，完成 token 交换。

配合 auth login --print-url 使用的第二步操作。

使用流程:
  1. feishu-cli auth login --print-url    # 获取授权 URL 和 state
  2. 在浏览器中完成授权
  3. feishu-cli auth callback "<回调URL>" --state "<state>"

示例:
  feishu-cli auth callback "http://127.0.0.1:9768/callback?code=xxx&state=yyy" --state "yyy"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		cfg := config.Get()

		state, _ := cmd.Flags().GetString("state")
		port, _ := cmd.Flags().GetInt("port")

		// 从回调 URL 中解析 code 并校验 state
		code, err := auth.ParseCallbackURL(args[0], state)
		if err != nil {
			return err
		}

		// 构造 redirectURI（需与 --print-url 时一致）
		redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, auth.CallbackPath)

		baseURL := config.ResolveAPIBaseURL(cfg)

		// 用 code 换 token
		token, err := auth.ExchangeToken(code, cfg.AppID, cfg.AppSecret, redirectURI, baseURL)
		if err != nil {
			return err
		}

		// 保存 token
		if err := auth.SaveToken(token); err != nil {
			return err
		}

		// JSON 输出到 stdout（供 AI Agent 解析）
		result := map[string]any{
			"status":     "success",
			"expires_at": token.ExpiresAt.Format("2006-01-02T15:04:05+08:00"),
			"scope":      token.Scope,
		}
		if err := printJSON(result); err != nil {
			return err
		}

		// 人类可读输出到 stderr
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
	authCmd.AddCommand(authCallbackCmd)

	authCallbackCmd.Flags().String("state", "", "授权 state 参数（从 --print-url 输出获取）")
	authCallbackCmd.Flags().Int("port", auth.DefaultPort, "回调端口（需与 --print-url 时一致）")
	mustMarkFlagRequired(authCallbackCmd, "state")
}
