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
	Long: `在指定日历中搜索日程。

参数:
  --calendar-id, -c   日历 ID（必填）
  --query, -q         搜索关键词（必填）
  --start             搜索起始时间，RFC3339 格式（可选）
  --end               搜索结束时间，RFC3339 格式（可选）
  --page-size         每页数量（可选）
  --page-token        分页标记（可选）

示例:
  feishu-cli calendar event-search --calendar-id CAL_xxx --query "会议"
  feishu-cli calendar event-search -c CAL_xxx -q "评审" \
    --start 2024-01-01T00:00:00+08:00 --end 2024-12-31T23:59:59+08:00`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID, _ := cmd.Flags().GetString("calendar-id")
		query, _ := cmd.Flags().GetString("query")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		events, nextPageToken, err := client.SearchEvents(calendarID, query, startTime, endTime, pageToken, pageSize, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"events":          events,
				"next_page_token": nextPageToken,
			})
		}

		if len(events) == 0 {
			fmt.Println("未找到匹配的日程")
			return nil
		}

		fmt.Printf("搜索到 %d 个日程:\n\n", len(events))
		for i, event := range events {
			fmt.Printf("[%d] %s\n", i+1, event.Summary)
			fmt.Printf("    日程 ID:   %s\n", event.EventID)
			fmt.Printf("    开始时间:  %s\n", event.StartTime)
			fmt.Printf("    结束时间:  %s\n", event.EndTime)
			if event.Location != "" {
				fmt.Printf("    地点:      %s\n", event.Location)
			}
			fmt.Println()
		}

		if nextPageToken != "" {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarEventSearchCmd)
	calendarEventSearchCmd.Flags().StringP("calendar-id", "c", "", "日历 ID（必填）")
	calendarEventSearchCmd.Flags().StringP("query", "q", "", "搜索关键词（必填）")
	calendarEventSearchCmd.Flags().String("start", "", "搜索起始时间，RFC3339 格式")
	calendarEventSearchCmd.Flags().String("end", "", "搜索结束时间，RFC3339 格式")
	calendarEventSearchCmd.Flags().Int("page-size", 0, "每页数量")
	calendarEventSearchCmd.Flags().String("page-token", "", "分页标记")
	calendarEventSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarEventSearchCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	mustMarkFlagRequired(calendarEventSearchCmd, "calendar-id", "query")
}
