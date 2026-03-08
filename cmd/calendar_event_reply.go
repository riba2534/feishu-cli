package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarEventReplyCmd = &cobra.Command{
	Use:   "event-reply <calendar_id> <event_id>",
	Short: "回复日程邀请",
	Long: `回复日程邀请，可以接受、拒绝或标记为待定。

参数:
  calendar_id       日历 ID（位置参数）
  event_id          日程 ID（位置参数）
  --status          回复状态（必填）: accept/decline/tentative

示例:
  feishu-cli calendar event-reply CAL_xxx EVENT_xxx --status accept
  feishu-cli calendar event-reply CAL_xxx EVENT_xxx --status decline
  feishu-cli calendar event-reply CAL_xxx EVENT_xxx --status tentative`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID := args[0]
		eventID := args[1]
		status, _ := cmd.Flags().GetString("status")

		if status != "accept" && status != "decline" && status != "tentative" {
			return fmt.Errorf("无效的回复状态: %s，有效值: accept/decline/tentative", status)
		}

		if err := client.ReplyEvent(calendarID, eventID, status, token); err != nil {
			return err
		}

		statusMap := map[string]string{
			"accept":    "已接受",
			"decline":   "已拒绝",
			"tentative": "待定",
		}
		fmt.Printf("日程回复成功: %s\n", statusMap[status])

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarEventReplyCmd)
	calendarEventReplyCmd.Flags().String("status", "", "回复状态: accept/decline/tentative（必填）")
	calendarEventReplyCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	mustMarkFlagRequired(calendarEventReplyCmd, "status")
}
