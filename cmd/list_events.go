package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listEventsCmd = &cobra.Command{
	Use:   "list-events <calendar_id>",
	Short: "列出日程",
	Long: `列出指定日历中的日程列表。

参数:
  calendar_id       日历 ID

可选参数:
  --start-time      开始时间过滤，RFC3339 格式
  --end-time        结束时间过滤，RFC3339 格式
  --page-size       每页数量（默认 50）
  --page-token      分页标记
  --output, -o      输出格式（json）

时间格式:
  使用 RFC3339 格式，例如：
  - 2024-01-21T00:00:00+08:00
  - 2024-01-21T00:00:00Z

示例:
  # 列出所有日程
  feishu-cli calendar list-events CAL_ID

  # 列出指定时间范围的日程
  feishu-cli calendar list-events CAL_ID \
    --start-time 2024-01-01T00:00:00+08:00 \
    --end-time 2024-01-31T23:59:59+08:00

  # JSON 格式输出
  feishu-cli calendar list-events CAL_ID --output json

  # 指定每页数量
  feishu-cli calendar list-events CAL_ID --page-size 20`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID := args[0]
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		params := &client.ListEventsParams{
			CalendarID: calendarID,
			StartTime:  startTime,
			EndTime:    endTime,
			PageSize:   pageSize,
			PageToken:  pageToken,
		}

		events, nextToken, hasMore, err := client.ListEvents(params)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]interface{}{
				"events":   events,
				"has_more": hasMore,
			}
			if nextToken != "" {
				result["page_token"] = nextToken
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(events) == 0 {
				fmt.Println("未找到日程")
				return nil
			}

			fmt.Printf("共找到 %d 个日程:\n\n", len(events))
			for i, event := range events {
				fmt.Printf("[%d] %s\n", i+1, event.Summary)
				fmt.Printf("    日程 ID:   %s\n", event.EventID)
				fmt.Printf("    开始时间:  %s\n", event.StartTime)
				fmt.Printf("    结束时间:  %s\n", event.EndTime)
				if event.Location != "" {
					fmt.Printf("    地点:      %s\n", event.Location)
				}
				if event.Status != "" {
					fmt.Printf("    状态:      %s\n", event.Status)
				}
				fmt.Println()
			}

			if hasMore {
				fmt.Printf("还有更多日程，使用 --page-token %s 获取下一页\n", nextToken)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(listEventsCmd)
	listEventsCmd.Flags().String("start-time", "", "开始时间过滤，RFC3339 格式")
	listEventsCmd.Flags().String("end-time", "", "结束时间过滤，RFC3339 格式")
	listEventsCmd.Flags().Int("page-size", 50, "每页数量")
	listEventsCmd.Flags().String("page-token", "", "分页标记")
	listEventsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
