package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var readUsersCmd = &cobra.Command{
	Use:   "read-users <message_id>",
	Short: "查询消息已读用户",
	Long: `查询指定消息的已读用户列表。

参数:
  message_id       消息 ID（必填）
  --user-id-type   用户 ID 类型（默认: open_id）
  --page-size      每页数量（默认: 20，最大: 100）
  --page-token     分页标记
  --output, -o     输出格式（json）

用户 ID 类型:
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID

示例:
  # 查询消息已读用户
  feishu-cli msg read-users om_xxx

  # 使用 user_id 类型
  feishu-cli msg read-users om_xxx --user-id-type user_id

  # 分页查询
  feishu-cli msg read-users om_xxx --page-size 50

  # JSON 格式输出
  feishu-cli msg read-users om_xxx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.GetReadUsers(messageID, userIDType, pageSize, pageToken)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"items":      result.Items,
				"page_token": result.PageToken,
				"has_more":   result.HasMore,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("已读用户列表（共 %d 人）:\n", len(result.Items))
			for i, user := range result.Items {
				fmt.Printf("\n[%d] 用户 ID: %s\n", i+1, user.UserID)
				fmt.Printf("    ID 类型: %s\n", user.UserIDType)
				fmt.Printf("    阅读时间: %s\n", user.Timestamp)
				if user.TenantKey != "" {
					fmt.Printf("    租户 Key: %s\n", user.TenantKey)
				}
			}
			if result.HasMore {
				fmt.Printf("\n还有更多用户，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(readUsersCmd)
	readUsersCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/user_id/union_id）")
	readUsersCmd.Flags().Int("page-size", 20, "每页数量（最大 100）")
	readUsersCmd.Flags().String("page-token", "", "分页标记")
	readUsersCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
