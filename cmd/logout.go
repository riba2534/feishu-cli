package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "退出登录，清除缓存的用户令牌",
	Long: `退出飞书 OAuth 登录，删除本地缓存的 user_access_token。

退出后，feishu-cli 将回退到使用应用身份（tenant_access_token）调用 API。

示例:
  feishu-cli logout`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.DeleteToken(); err != nil {
			return fmt.Errorf("退出登录失败: %w", err)
		}
		fmt.Println("已退出登录，用户令牌已清除。")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
