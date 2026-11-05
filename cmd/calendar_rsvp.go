package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
  --rsvp-status        兼容别名，等价于 --action
  --dry-run            只预览请求，不执行（不联网、不读凭据）

权限:
  calendar:calendar.event:reply（必需 User Token，以本人身份答复；未登录时报错）

示例:
  # 接受主日历上某个邀请
  feishu-cli calendar rsvp --event-id EVENT_xxx --action accept

  # 拒绝指定日历上某个邀请
  feishu-cli calendar rsvp --calendar-id CAL_xxx --event-id EVENT_xxx --action decline

  # 待定
  feishu-cli calendar rsvp --event-id EVENT_xxx --action tentative

  # 只预览
  feishu-cli calendar rsvp --event-id EVENT_xxx --action accept --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, _ := cmd.Flags().GetString("calendar-id")
		eventID, _ := cmd.Flags().GetString("event-id")
		action, _ := cmd.Flags().GetString("action")
		rsvpStatus, _ := cmd.Flags().GetString("rsvp-status")

		calendarID = strings.TrimSpace(calendarID)
		eventID = strings.TrimSpace(eventID)
		action = strings.TrimSpace(action)
		rsvpStatus = strings.TrimSpace(rsvpStatus)

		if eventID == "" {
			return clierr.Usagef("--event-id 不能为空")
		}
		if action != "" && rsvpStatus != "" && action != rsvpStatus {
			return clierr.Usagef("--action 与 --rsvp-status 不能同时指定不同值")
		}
		if action == "" {
			action = rsvpStatus
		}
		if action == "" {
			return clierr.Usagef("必须指定 --action 或 --rsvp-status")
		}
		if action != "accept" && action != "decline" && action != "tentative" {
			return clierr.Usagef("无效的 --action: %s，有效值: accept/decline/tentative", action)
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			steps := []dryRunStep{}
			calPath := calendarID
			if calendarID == "" {
				calPath = "<primary_calendar_id>"
				steps = append(steps, dryRunStep{Method: "POST", URL: "/open-apis/calendar/v4/calendars/primary", Desc: "未传 --calendar-id：先取当前用户主日历"})
			}
			steps = append(steps, dryRunStep{
				Method: "POST",
				URL:    "/open-apis/calendar/v4/calendars/" + calPath + "/events/" + eventID + "/reply",
				Desc:   "以本人身份答复日程（需 User Token）",
				Body:   map[string]any{"rsvp_status": action},
			})
			return printDryRunPlan(cmd, "calendar rsvp 预览（未执行）", nil, steps)
		}

		if err := config.Validate(); err != nil {
			return err
		}

		token, errToken := requireUserToken(cmd, "calendar rsvp")
		if errToken != nil {
			return errToken
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
	calendarRsvpCmd.Flags().String("rsvp-status", "", "答复动作，兼容别名：accept/decline/tentative")
	calendarRsvpCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	calendarRsvpCmd.Flags().String("user-access-token", "", "User Access Token（必需，可显式传入或经 auth login 解析）")

	mustMarkFlagRequired(calendarRsvpCmd, "event-id")
}
