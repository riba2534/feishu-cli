package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var calendarEventReplyCmd = &cobra.Command{
	Use:   "event-reply <calendar_id> <event_id>",
	Short: "回复日程邀请（与 rsvp 等价，必需 User Token）",
	Long: `回复日程邀请，可以接受、拒绝或标记为待定。

与 calendar rsvp 语义相同（rsvp 为全 flag 风格、可省略 calendar_id），本命令保留位置参数风格。
答复是"本人"的动作，必须使用 User Token（auth login 后自动使用；未登录时报错）。
此前版本默认用 Bot 身份调用，Bot 不是被邀请人，答复必然失败或答复到 Bot 自己（行为变更）。

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

		calendarID := args[0]
		eventID := args[1]
		status, _ := cmd.Flags().GetString("status")

		if status != "accept" && status != "decline" && status != "tentative" {
			return clierr.Usagef("无效的回复状态: %s，有效值: accept/decline/tentative", status)
		}

		token, err := requireUserToken(cmd, "calendar event-reply")
		if err != nil {
			return err
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
	calendarEventReplyCmd.Flags().String("user-access-token", "", "User Access Token（必需，可显式传入或经 auth login 解析）")

	mustMarkFlagRequired(calendarEventReplyCmd, "status")
}
