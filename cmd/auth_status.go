package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/token"
	"github.com/spf13/cobra"
)

// authStatusCmd 查看 token 状态命令
var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看 User Access Token 状态",
	Long: `显示当前 User Access Token 的信息和过期状态。

检查以下位置的 token（按优先级排序）:
1. FEISHU_USER_ACCESS_TOKEN 环境变量
2. 配置文件中的 user_access_token
3. ~/.lark_user_token 文件`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=")
		fmt.Println("User Access Token 状态")
		fmt.Println("=")
		fmt.Println()

		// 1. 检查环境变量
		envToken := os.Getenv("FEISHU_USER_ACCESS_TOKEN")
		if envToken != "" {
			fmt.Println("Token 来源: 环境变量 FEISHU_USER_ACCESS_TOKEN")
			fmt.Printf("Token: %s...%s\n",
				envToken[:min(10, len(envToken))],
				envToken[max(0, len(envToken)-10):])
			fmt.Println("状态: 可用（环境变量优先级最高）")
			fmt.Println()
			fmt.Println("提示: 环境变量设置的 token 不会自动刷新，过期后需要手动更新")
			return nil
		}

		// 2. 检查配置文件
		cfg := config.Get()
		if cfg.UserAccessToken != "" {
			fmt.Println("Token 来源: 配置文件")
			fmt.Printf("Token: %s...%s\n",
				cfg.UserAccessToken[:min(10, len(cfg.UserAccessToken))],
				cfg.UserAccessToken[max(0, len(cfg.UserAccessToken)-10):])
			fmt.Println("状态: 可用")
			fmt.Println()
			fmt.Println("提示: 配置文件中的 token 不会自动刷新，过期后需要手动更新")
			return nil
		}

		// 3. 检查 token 文件
		tok, err := token.LoadToken()
		if err != nil {
			fmt.Println("状态: 未登录")
			fmt.Println()
			fmt.Println("未找到 User Access Token，请运行以下命令进行授权:")
			fmt.Println("  feishu-cli auth login")
			fmt.Println()
			return nil
		}

		fmt.Println("Token 来源: ~/.lark_user_token")
		fmt.Println()
		fmt.Println("Token 信息:")
		fmt.Printf("  Access Token:  %s...%s\n",
			tok.AccessToken[:min(10, len(tok.AccessToken))],
			tok.AccessToken[max(0, len(tok.AccessToken)-10):])
		fmt.Printf("  Refresh Token: %s...%s\n",
			tok.RefreshToken[:min(10, len(tok.RefreshToken))],
			tok.RefreshToken[max(0, len(tok.RefreshToken)-10):])
		fmt.Println()
		fmt.Println("状态:")
		if tok.IsExpired() {
			fmt.Printf("  ⚠️ 已过期 (%s)\n", tok.FormatExpiry())
			fmt.Println()
			fmt.Println("建议运行以下命令刷新 token:")
			fmt.Println("  feishu-cli auth refresh")
		} else {
			fmt.Printf("  ✅ 有效，剩余 %s\n", tok.FormatExpiry())
		}
		fmt.Println()

		return nil
	},
}

func init() {
	authCmd.AddCommand(authStatusCmd)
}
