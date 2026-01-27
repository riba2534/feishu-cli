package cmd

import (
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "用户操作命令",
	Long: `用户操作命令，用于获取用户信息。

子命令:
  info    获取用户详细信息

用户 ID 类型:
  open_id     Open ID（默认）
  union_id    Union ID
  user_id     用户 ID

示例:
  # 获取用户信息（使用 open_id）
  feishu-cli user info ou_xxx

  # 使用 user_id 获取用户信息
  feishu-cli user info xxx --user-id-type user_id

  # JSON 格式输出
  feishu-cli user info ou_xxx -o json`,
}

func init() {
	rootCmd.AddCommand(userCmd)
}
