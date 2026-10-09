package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarEventSearchCmd = &cobra.Command{
	Use:   "event-search",
	Short: "搜索日程",
	Long: `在指定日历中搜索日程（POST /calendars/{id}/events/search_event）。

参数:
  --calendar-id, -c   日历 ID（可选，默认 primary）
  --query, -q         搜索关键词（可选；可只靠 filter 搜索）
  --start             搜索起始时间，RFC3339 或 YYYY-MM-DD（可选，写入 filter.time_range）
  --end               搜索结束时间，RFC3339 或 YYYY-MM-DD（可选；只给一边时补同一天边界）
  --attendee-ids      参与人 ID，逗号分隔（ou_/oc_/omm_，可选）
  --page-size         每页数量（1-30，默认 20；越界报错）
  --page-token        分页标记（可选）
  --output, -o        json 时输出 events / next_page_token / has_more（has_more 以服务端为准）
  --as                身份：bot | user | auto（默认 auto；已配置 User 刷新失败 fail-closed）

示例:
  feishu-cli calendar event-search --query "会议"
  feishu-cli calendar event-search --start 2026-04-20 --attendee-ids ou_xxx
  feishu-cli calendar event-search -c CAL_xxx -q "评审" \
    --start 2024-01-01T00:00:00+08:00 --end 2024-12-31T23:59:59+08:00`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID, _ := cmd.Flags().GetString("calendar-id")
		query, _ := cmd.Flags().GetString("query")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		attendeeIDs, _ := cmd.Flags().GetString("attendee-ids")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		startRFC, endRFC, err := client.ParseSearchEventTimeRange(startTime, endTime)
		if err != nil {
			return err
		}
		if _, err := client.ResolvePageSize(pageSize, 20, 1, 30); err != nil {
			return err
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		res, err := client.SearchEventsWithParams(client.SearchEventsParams{
			CalendarID:  calendarID,
			Query:       query,
			StartTime:   startRFC,
			EndTime:     endRFC,
			AttendeeIDs: splitAndTrim(attendeeIDs),
			PageToken:   pageToken,
			PageSize:    pageSize,
		}, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"events":          res.Events,
				"next_page_token": res.PageToken,
				"has_more":        res.HasMore,
			})
		}

		if len(res.Events) == 0 {
			fmt.Println("未找到匹配的日程")
			return nil
		}

		fmt.Printf("搜索到 %d 个日程:\n\n", len(res.Events))
		for i, event := range res.Events {
			fmt.Printf("[%d] %s\n", i+1, event.Summary)
			fmt.Printf("    日程 ID:   %s\n", event.EventID)
			fmt.Printf("    开始时间:  %s\n", event.StartTime)
			fmt.Printf("    结束时间:  %s\n", event.EndTime)
			if event.Location != "" {
				fmt.Printf("    地点:      %s\n", event.Location)
			}
			fmt.Println()
		}

		if res.HasMore {
			fmt.Printf("还有更多结果")
			if res.PageToken != "" {
				fmt.Printf("，使用 --page-token %s 获取下一页", res.PageToken)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarEventSearchCmd)
	calendarEventSearchCmd.Flags().StringP("calendar-id", "c", "", "日历 ID（默认 primary）")
	calendarEventSearchCmd.Flags().StringP("query", "q", "", "搜索关键词（可空，支持纯 filter 搜索）")
	calendarEventSearchCmd.Flags().String("start", "", "搜索起始时间，RFC3339 或 YYYY-MM-DD（filter.time_range.start_time）")
	calendarEventSearchCmd.Flags().String("end", "", "搜索结束时间，RFC3339 或 YYYY-MM-DD（filter.time_range.end_time）")
	calendarEventSearchCmd.Flags().String("attendee-ids", "", "参与人 ID，逗号分隔（ou_ 用户 / oc_ 群 / omm_ 会议室）")
	calendarEventSearchCmd.Flags().Int("page-size", 0, "每页数量（1-30，默认 20）")
	calendarEventSearchCmd.Flags().String("page-token", "", "分页标记")
	calendarEventSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarEventSearchCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	calendarEventSearchCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto；已配置 User 刷新失败 fail-closed）")
}
