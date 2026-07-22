package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createEventCmd = &cobra.Command{
	Use:   "create-event",
	Short: "创建日程",
	Long: `在指定日历中创建新日程。

参数:
  --calendar-id, -c   日历 ID（必填）
  --summary, -s       日程标题（必填）
  --start             开始时间，RFC3339 格式（必填）
  --end               结束时间，RFC3339 格式（必填）
  --description, -d   日程描述（可选）
  --location, -l      地点（可选）
  --rrule             重复规则（RFC5545 RRULE，可选），创建重复日程
  --output, -o        输出格式，可选 json

时间格式:
  使用 RFC3339 格式，例如：
  - 2024-01-21T14:00:00+08:00
  - 2024-01-21T06:00:00Z

重复规则（--rrule，RFC5545 RRULE 字符串）:
  - 每周一：            FREQ=WEEKLY;BYDAY=MO
  - 每个工作日：        FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
  - 每天，共 10 次：    FREQ=DAILY;COUNT=10
  - 每月 1 号至某日结束：FREQ=MONTHLY;BYMONTHDAY=1;UNTIL=20261231T000000Z
  注意: COUNT 与 UNTIL 不能同时出现。--start/--end 决定首个实例的时间。

示例:
  # 创建基本日程
  feishu-cli calendar create-event \
    --calendar-id CAL_ID \
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

  # JSON 格式输出
  feishu-cli calendar create-event \
    --calendar-id CAL_ID \
    --summary "会议" \
    --start 2024-01-21T14:00:00+08:00 \
    --end 2024-01-21T15:00:00+08:00 \
    --output json

  # 创建每周一重复的日程
  feishu-cli calendar create-event \
    --calendar-id CAL_ID \
    --summary "周会" \
    --start 2024-01-22T10:00:00+08:00 \
    --end 2024-01-22T11:00:00+08:00 \
    --rrule "FREQ=WEEKLY;BYDAY=MO"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID, _ := cmd.Flags().GetString("calendar-id")
		summary, _ := cmd.Flags().GetString("summary")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		rrule, _ := cmd.Flags().GetString("rrule")
		output, _ := cmd.Flags().GetString("output")

		params := &client.CreateEventParams{
			CalendarID:  calendarID,
			Summary:     summary,
			StartTime:   startTime,
			EndTime:     endTime,
			Description: description,
			Location:    location,
			Recurrence:  rrule,
		}

		event, err := client.CreateEvent(params, token)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(event); err != nil {
				return err
			}
		} else {
			fmt.Println("日程创建成功！")
			fmt.Printf("  日程 ID:   %s\n", event.EventID)
			fmt.Printf("  标题:      %s\n", event.Summary)
			fmt.Printf("  开始时间:  %s\n", event.StartTime)
			fmt.Printf("  结束时间:  %s\n", event.EndTime)
			if event.Description != "" {
				fmt.Printf("  描述:      %s\n", event.Description)
			}
			if event.Location != "" {
				fmt.Printf("  地点:      %s\n", event.Location)
			}
			if event.Recurrence != "" {
				fmt.Printf("  重复规则:  %s\n", event.Recurrence)
			}
			if event.AppLink != "" {
				fmt.Printf("  链接:      %s\n", event.AppLink)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(createEventCmd)
	createEventCmd.Flags().StringP("calendar-id", "c", "", "日历 ID（必填）")
	createEventCmd.Flags().StringP("summary", "s", "", "日程标题（必填）")
	createEventCmd.Flags().String("start", "", "开始时间，RFC3339 格式（必填）")
	createEventCmd.Flags().String("end", "", "结束时间，RFC3339 格式（必填）")
	createEventCmd.Flags().StringP("description", "d", "", "日程描述")
	createEventCmd.Flags().StringP("location", "l", "", "地点")
	createEventCmd.Flags().String("rrule", "", "重复规则（RFC5545 RRULE，如 FREQ=WEEKLY;BYDAY=MO）")
	createEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	createEventCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	mustMarkFlagRequired(createEventCmd, "calendar-id", "summary", "start", "end")
}
