package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var readUsersCmd = &cobra.Command{
	Use:   "read-users <message_id>",
	Short: "查询消息已读用户",
	Long: `查询指定消息的已读用户列表（GET /open-apis/im/v1/messages/:message_id/read_users）。

只能查询"当前身份自己发送的"、且发送时间在 7 天内的消息，按消息发送者选择身份:
  Bot 发送的消息   --as bot
  本人发送的消息   --as user
  --as auto（默认）已登录时用 User，未登录时用 Bot；查 Bot 发的消息请显式 --as bot

参数:
  message_id       消息 ID（必填）
  --as             身份: bot | user | auto（默认 auto）
  --user-id-type   用户 ID 类型（默认: open_id）
  --page-size      每页数量（默认: 20，最大: 100）
  --page-token     分页标记
  --output, -o     输出格式（json）

权限:
  im:message:readonly / im:message / im:message:basic 任一（User 身份为用户授权，Bot 身份为应用权限；
  Bot 还须在该会话中）

用户 ID 类型:
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID

示例:
  # 查询 Bot 发送的消息的已读用户
  feishu-cli msg read-users om_xxx --as bot

  # 查询本人发送的消息的已读用户
  feishu-cli msg read-users om_xxx --as user

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

		// 接口同时接受 User / Tenant，只能查当前身份自己发送的消息：按 --as 显式选择身份
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		messageID := args[0]
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.GetReadUsers(messageID, userIDType, pageSize, pageToken, token)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]any{
				"items":      result.Items,
				"page_token": result.PageToken,
				"has_more":   result.HasMore,
			}); err != nil {
				return err
			}
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
	readUsersCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌，--as user/auto 时使用）")
	readUsersCmd.Flags().String("as", "auto", "身份: bot（查 Bot 发送的消息）| user（查本人发送的消息）| auto（User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed）")
}
