package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createEventCmd = &cobra.Command{
	Use:   "create-event",
	Short: "创建日程",
	Long: `在指定日历中创建新日程。

参数:
  --calendar-id, -c   日历 ID（默认 primary：当前身份的主日历）
  --summary, -s       日程标题（必填）
  --start             开始时间（必填）
  --end               结束时间（必填，须晚于开始时间）
  --description, -d   日程描述（可选）
  --location, -l      地点（可选）
  --rrule             重复规则（RFC5545 RRULE，可选），创建重复日程
  --attendee-ids      参与人（可选），逗号分隔：ou_ 用户 / oc_ 群 / omm_ 会议室 / 邮箱=外部参与人。
                      日程创建后再添加参与人；添加失败会自动删除刚创建的日程（回滚）。
                      User 身份会自动把本人加入参与人列表
  --vchat             同时创建飞书视频会议（可选）
  --dry-run           只预览请求，不执行
  --as                身份：auto（默认）| user | bot
  --output, -o        输出格式，可选 json

身份（默认 auto；此前版本写命令默认 Bot，属行为变更）:
  已登录（auth login）时以本人身份在本人日历上创建；未配置 User Token 时回退 Bot（应用日历）；
  已配置但刷新失败时报错而不会静默切 Bot。需要在应用日历上创建时显式传 --as bot。

时间格式:
  推荐 RFC3339（带时区），例如：
  - 2024-01-21T14:00:00+08:00
  - 2024-01-21T06:00:00Z
  也接受 "2024-01-21 14:00"（按本地时区）与 Unix 秒/毫秒。

重复规则（--rrule，RFC5545 RRULE 字符串）:
  - 每周一：            FREQ=WEEKLY;BYDAY=MO
  - 每个工作日：        FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
  - 每天，共 10 次：    FREQ=DAILY;COUNT=10
  - 每月 1 号至某日结束：FREQ=MONTHLY;BYMONTHDAY=1;UNTIL=20261231T000000Z
  注意: COUNT 与 UNTIL 不能同时出现。--start/--end 决定首个实例的时间。

示例:
  # 在本人主日历创建基本日程
  feishu-cli calendar create-event \
    --summary "团队会议" \
    --start 2024-01-21T14:00:00+08:00 \
    --end 2024-01-21T15:00:00+08:00

  # 创建带描述和地点的日程
  feishu-cli calendar create-event \
    --calendar-id CAL_ID \
    --summary "项目评审" \
    --start 2024-01-21T14:00:00+08:00 \
    --end 2024-01-21T16:00:00+08:00 \
    --description "Q1 项目进度评审" \
    --location "会议室 A101"

  # 邀请参与人与会议室，并创建视频会议（参与人添加失败会自动回滚）
  feishu-cli calendar create-event \
    --summary "三方对齐" \
    --start 2024-01-21T14:00:00+08:00 \
    --end 2024-01-21T15:00:00+08:00 \
    --attendee-ids ou_xxx,oc_xxx,omm_xxx --vchat --output json

  # 创建每周一重复的日程
  feishu-cli calendar create-event \
    --calendar-id CAL_ID \
    --summary "周会" \
    --start 2024-01-22T10:00:00+08:00 \
    --end 2024-01-22T11:00:00+08:00 \
    --rrule "FREQ=WEEKLY;BYDAY=MO"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, _ := cmd.Flags().GetString("calendar-id")
		summary, _ := cmd.Flags().GetString("summary")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		rrule, _ := cmd.Flags().GetString("rrule")
		attendeeIDs, _ := cmd.Flags().GetString("attendee-ids")
		withVChat, _ := cmd.Flags().GetBool("vchat")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		calendarID = strings.TrimSpace(calendarID)
		if calendarID == "" {
			calendarID = "primary"
		}
		if strings.TrimSpace(summary) == "" {
			return clierr.Usagef("--summary 不能为空")
		}
		startUnix, err := client.ParseEventTimeToUnix(startTime)
		if err != nil {
			return clierr.Usagef("--start: %v", err)
		}
		endUnix, err := client.ParseEventTimeToUnix(endTime)
		if err != nil {
			return clierr.Usagef("--end: %v", err)
		}
		if endUnix <= startUnix {
			return clierr.Usagef("--end 必须晚于 --start")
		}
		attendees, err := parseAttendeeIDsFlag(attendeeIDs, "attendee-ids", false)
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			body := map[string]any{
				"summary":    summary,
				"start_time": map[string]any{"timestamp": fmt.Sprint(startUnix)},
				"end_time":   map[string]any{"timestamp": fmt.Sprint(endUnix)},
			}
			if description != "" {
				body["description"] = description
			}
			if location != "" {
				body["location"] = map[string]any{"name": location}
			}
			if rrule != "" {
				body["recurrence"] = rrule
			}
			if withVChat {
				body["vchat"] = map[string]any{"vc_type": "vc"}
			}
			steps := []dryRunStep{{
				Method: "POST",
				URL:    "/open-apis/calendar/v4/calendars/" + calendarID + "/events",
				Desc:   "创建日程",
				Body:   body,
			}}
			if len(attendees) > 0 {
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    "/open-apis/calendar/v4/calendars/" + calendarID + "/events/<event_id>/attendees",
					Desc:   "添加参与人（User 身份会自动带上本人）；失败则删除刚创建的日程回滚",
					Params: map[string]any{"user_id_type": "open_id"},
					Body:   map[string]any{"attendees": client.AttendeeRefsToEventAttendees(attendees), "need_notification": true},
				})
			}
			return printDryRunPlan(cmd, "create-event 预览（未执行）", map[string]any{"calendar_id": calendarID}, steps)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		params := &client.CreateEventParams{
			CalendarID:  calendarID,
			Summary:     summary,
			StartTime:   startTime,
			EndTime:     endTime,
			Description: description,
			Location:    location,
			Recurrence:  rrule,
			WithVChat:   withVChat,
		}

		event, err := client.CreateEvent(params, token)
		if err != nil {
			return err
		}

		if len(attendees) > 0 {
			list := client.AttendeeRefsToEventAttendees(attendees)
			// User 身份把本人也加为参与人（与客户端行为一致，组织者出现在参与人列表里）
			if token != "" {
				if selfID, idErr := resolveCurrentAuthedUserID(cmd, "open_id"); idErr == nil && selfID != "" {
					dup := false
					for _, a := range list {
						if a.UserID == selfID {
							dup = true
						}
					}
					if !dup {
						list = append(list, &client.EventAttendee{Type: "user", UserID: selfID})
					}
				}
			}
			if addErr := client.AddEventAttendees(calendarID, event.EventID, list, token); addErr != nil {
				notify := false
				if rbErr := client.DeleteEventWithOptions(calendarID, event.EventID, client.DeleteEventOptions{NeedNotification: &notify}, token); rbErr != nil {
					return fmt.Errorf("%w\n添加参与人失败，回滚删除日程也失败（%v），请手动删除残留日程 event_id=%s", addErr, rbErr, event.EventID)
				}
				return fmt.Errorf("%w\n添加参与人失败，已删除刚创建的日程（回滚成功）", addErr)
			}
		}

		if output == "json" {
			return printJSON(event)
		}
		fmt.Println("日程创建成功！")
		fmt.Printf("  日程 ID:   %s\n", event.EventID)
		fmt.Printf("  标题:      %s\n", event.Summary)
		printEventTimeLines(event, "  ")
		if event.Description != "" {
			fmt.Printf("  描述:      %s\n", event.Description)
		}
		if event.Location != "" {
			fmt.Printf("  地点:      %s\n", event.Location)
		}
		if event.Recurrence != "" {
			fmt.Printf("  重复规则:  %s\n", event.Recurrence)
		}
		if event.Vchat != nil && event.Vchat.MeetingURL != "" {
			fmt.Printf("  会议链接:  %s\n", event.Vchat.MeetingURL)
		}
		if len(attendees) > 0 {
			fmt.Printf("  参与人:    已添加 %d 个\n", len(attendees))
		}
		return nil
	},
}

func init() {
	calendarCmd.AddCommand(createEventCmd)
	createEventCmd.Flags().StringP("calendar-id", "c", "primary", "日历 ID（默认 primary：当前身份的主日历）")
	createEventCmd.Flags().StringP("summary", "s", "", "日程标题（必填）")
	createEventCmd.Flags().String("start", "", "开始时间（必填），RFC3339，如 2024-01-21T14:00:00+08:00")
	createEventCmd.Flags().String("end", "", "结束时间（必填），RFC3339")
	createEventCmd.Flags().StringP("description", "d", "", "日程描述")
	createEventCmd.Flags().StringP("location", "l", "", "地点")
	createEventCmd.Flags().String("rrule", "", "重复规则（RFC5545 RRULE，如 FREQ=WEEKLY;BYDAY=MO）")
	createEventCmd.Flags().String("attendee-ids", "", "参与人，逗号分隔：ou_ 用户 / oc_ 群 / omm_ 会议室 / 邮箱=外部参与人；添加失败自动删除日程回滚")
	createEventCmd.Flags().Bool("vchat", false, "同时创建飞书视频会议（日程详情带会议链接）")
	createEventCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	createEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	createEventCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(createEventCmd)

	mustMarkFlagRequired(createEventCmd, "summary", "start", "end")
}
