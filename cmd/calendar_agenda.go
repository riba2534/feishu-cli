package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarAgendaCmd = &cobra.Command{
	Use:   "agenda [calendar_id]",
	Short: "查看日程列表（展开重复日程）",
	Long: `查看指定日历的日程列表，自动将重复日程展开为独立实例。

参数:
  calendar_id  日历 ID（可选，默认为 "primary" 主日历）

可选参数:
  --start-date    起始日期，格式 YYYY-MM-DD（默认今天）
  --end-date      结束日期，格式 YYYY-MM-DD（默认起始日期的下一天）
  --page-size     每页数量
  --page-token    分页标记
  --output, -o    输出格式（json）

示例:
  # 查看今日日程（使用主日历）
  feishu-cli calendar agenda

  # 查看指定日期范围
  feishu-cli calendar agenda --start-date 2026-03-28 --end-date 2026-03-29

  # 查看指定日历
  feishu-cli calendar agenda CAL_xxx --start-date 2026-03-28

  # JSON 格式输出
  feishu-cli calendar agenda -o json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		// 解析 calendar_id，默认 "primary"
		calendarID := "primary"
		if len(args) > 0 {
			calendarID = args[0]
		}

		startDateStr, _ := cmd.Flags().GetString("start-date")
		endDateStr, _ := cmd.Flags().GetString("end-date")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		// 解析时间范围
		now := time.Now()
		loc := now.Location()

		var startTime, endTime time.Time

		if startDateStr != "" {
			t, err := time.ParseInLocation("2006-01-02", startDateStr, loc)
			if err != nil {
				return fmt.Errorf("解析起始日期失败（格式应为 YYYY-MM-DD）: %w", err)
			}
			startTime = t
		} else {
			// 默认今天 00:00
			startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		}

		if endDateStr != "" {
			t, err := time.ParseInLocation("2006-01-02", endDateStr, loc)
			if err != nil {
				return fmt.Errorf("解析结束日期失败（格式应为 YYYY-MM-DD）: %w", err)
			}
			// 结束日期设为当天 23:59:59，使查询包含该天
			endTime = t.Add(24*time.Hour - time.Second)
		} else {
			// 默认为起始日期的下一天
			endTime = startTime.Add(24*time.Hour - time.Second)
		}

		events, nextToken, hasMore, err := client.ListCalendarAgenda(
			calendarID,
			startTime.Unix(),
			endTime.Unix(),
			pageSize,
			pageToken,
			token,
		)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]any{
				"events":   events,
				"has_more": hasMore,
			}
			if nextToken != "" {
				result["page_token"] = nextToken
			}
			return printJSON(result)
		}

		if len(events) == 0 {
			fmt.Printf("在 %s ~ %s 期间无日程\n",
				startTime.Format("2006-01-02"),
				endTime.Format("2006-01-02"))
			return nil
		}

		fmt.Printf("日程列表（%s ~ %s，共 %d 个）:\n\n",
			startTime.Format("2006-01-02"),
			endTime.Format("2006-01-02"),
			len(events))

		for i, event := range events {
			summary := event.Summary
			if summary == "" {
				summary = "(无标题)"
			}

			fmt.Printf("[%d] %s\n", i+1, summary)
			fmt.Printf("    日程 ID:   %s\n", event.EventID)

			if event.IsAllDay {
				fmt.Printf("    时间:      全天日程 %s ~ %s\n", event.StartTime, event.EndTime)
			} else {
				fmt.Printf("    开始时间:  %s\n", event.StartTime)
				fmt.Printf("    结束时间:  %s\n", event.EndTime)
			}

			if event.Status != "" {
				fmt.Printf("    状态:      %s\n", event.Status)
			}
			if event.FreeBusyStatus != "" {
				fmt.Printf("    忙闲:      %s\n", event.FreeBusyStatus)
			}
			if event.SelfRSVP != "" {
				fmt.Printf("    回复状态:  %s\n", event.SelfRSVP)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("还有更多日程，使用 --page-token %s 获取下一页\n", nextToken)
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarAgendaCmd)
	calendarAgendaCmd.Flags().String("start-date", "", "起始日期，格式 YYYY-MM-DD（默认今天）")
	calendarAgendaCmd.Flags().String("end-date", "", "结束日期，格式 YYYY-MM-DD（默认起始日期的下一天）")
	calendarAgendaCmd.Flags().Int("page-size", 0, "每页数量")
	calendarAgendaCmd.Flags().String("page-token", "", "分页标记")
	calendarAgendaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarAgendaCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
