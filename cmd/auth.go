package cmd

import (
	"github.com/spf13/cobra"
)

// authCmd 认证命令组
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "管理飞书用户认证",
	Long: `管理飞书用户认证，包括登录、刷新 token、查看状态等操作。

User Access Token 用于需要用户身份的 API 操作，如搜索消息、搜索应用等。`,
}

func init() {
	rootCmd.AddCommand(authCmd)
}
