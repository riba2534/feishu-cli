package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarRsvpCmd = &cobra.Command{
	Use:   "rsvp",
	Short: "答复日程邀请（accept/decline/tentative）",
	Long: `答复日程邀请，可接受、拒绝或标记待定。

与已有 calendar event-reply 命令的区别：
  - rsvp 使用 flag 风格参数（--calendar-id / --event-id / --action），方便 AI Agent 调度
  - rsvp 的 --calendar-id 可省略，默认走主日历（自动调用 calendar primary 获取）
  - event-reply 使用位置参数 <calendar_id> <event_id> + --status

参数:
  --calendar-id        日历 ID（可选，默认主日历）
  --event-id           日程 ID（必填）
  --action             答复动作（必填）: accept / decline / tentative

权限:
  calendar:calendar.event:reply（推荐 User Token，以本人身份答复）

示例:
  # 接受主日历上某个邀请
  feishu-cli calendar rsvp --event-id EVENT_xxx --action accept

  # 拒绝指定日历上某个邀请
  feishu-cli calendar rsvp --calendar-id CAL_xxx --event-id EVENT_xxx --action decline

  # 待定
  feishu-cli calendar rsvp --event-id EVENT_xxx --action tentative`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, errToken := requireUserToken(cmd, "calendar rsvp")
		if errToken != nil {
			return errToken
		}

		calendarID, _ := cmd.Flags().GetString("calendar-id")
		eventID, _ := cmd.Flags().GetString("event-id")
		action, _ := cmd.Flags().GetString("action")

		calendarID = strings.TrimSpace(calendarID)
		eventID = strings.TrimSpace(eventID)
		action = strings.TrimSpace(action)

		if eventID == "" {
			return fmt.Errorf("--event-id 不能为空")
		}
		if action != "accept" && action != "decline" && action != "tentative" {
			return fmt.Errorf("无效的 --action: %s，有效值: accept/decline/tentative", action)
		}

		// 未传 calendar-id 默认拿主日历
		if calendarID == "" {
			primary, err := client.GetPrimaryCalendar(token)
			if err != nil {
				return fmt.Errorf("获取主日历失败: %w（请显式传 --calendar-id）", err)
			}
			if primary == nil || primary.CalendarID == "" {
				return fmt.Errorf("未能解析主日历，请显式传 --calendar-id")
			}
			calendarID = primary.CalendarID
		}

		if err := client.ReplyEvent(calendarID, eventID, action, token); err != nil {
			return err
		}

		statusMap := map[string]string{
			"accept":    "已接受",
			"decline":   "已拒绝",
			"tentative": "待定",
		}
		fmt.Printf("日程答复成功: %s（calendar_id=%s, event_id=%s）\n", statusMap[action], calendarID, eventID)
		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarRsvpCmd)
	calendarRsvpCmd.Flags().String("calendar-id", "", "日历 ID（可选，默认主日历）")
	calendarRsvpCmd.Flags().String("event-id", "", "日程 ID（必填）")
	calendarRsvpCmd.Flags().String("action", "", "答复动作: accept/decline/tentative（必填）")
	calendarRsvpCmd.Flags().String("user-access-token", "", "User Access Token（推荐，以本人身份答复）")

	mustMarkFlagRequired(calendarRsvpCmd, "event-id", "action")
}
