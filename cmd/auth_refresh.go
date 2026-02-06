package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/token"
	"github.com/spf13/cobra"
)

// authRefreshCmd 刷新 token 命令
var authRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "刷新 User Access Token",
	Long: `使用 refresh_token 换取新的 access_token。

当 access_token 过期时，可以使用 refresh_token 获取新的 token，
无需重新登录。refresh_token 的有效期通常为 30 天。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. 加载现有 token
		tok, err := token.LoadToken()
		if err != nil {
			return fmt.Errorf("加载 token 失败: %w", err)
		}

		fmt.Println("=")
		fmt.Println("刷新 User Access Token")
		fmt.Println("=")
		fmt.Println()

		// 2. 显示当前 token 状态
		fmt.Println("当前 Token 状态:")
		fmt.Printf("  Access Token: %s...%s\n",
			tok.AccessToken[:min(10, len(tok.AccessToken))],
			tok.AccessToken[max(0, len(tok.AccessToken)-10):])
		fmt.Printf("  过期状态: %s\n", tok.FormatExpiry())
		fmt.Println()

		// 3. 创建 OAuth 客户端
		oauthClient, err := client.NewOAuthClient("")
		if err != nil {
			return err
		}

		// 4. 刷新 token
		fmt.Println("正在刷新 token...")
		newToken, err := oauthClient.RefreshUserAccessToken(tok.RefreshToken)
		if err != nil {
			return fmt.Errorf("刷新 token 失败: %w", err)
		}

		// 5. 保存新 token
		if err := token.SaveToken(newToken); err != nil {
			return err
		}

		// 6. 显示成功信息
		fmt.Println()
		fmt.Println("=")
		fmt.Println("刷新成功！")
		fmt.Println("=")
		fmt.Println()
		fmt.Printf("新的 Access Token: %s...%s\n",
			newToken.AccessToken[:min(10, len(newToken.AccessToken))],
			newToken.AccessToken[max(0, len(newToken.AccessToken)-10):])
		fmt.Printf("新的过期时间: %s\n", newToken.FormatExpiry())
		fmt.Println()
		fmt.Println("Token 已更新到: ~/.lark_user_token")
		fmt.Println()

		return nil
	},
}

func init() {
	authCmd.AddCommand(authRefreshCmd)
}
