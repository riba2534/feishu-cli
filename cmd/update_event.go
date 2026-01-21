package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updateEventCmd = &cobra.Command{
	Use:   "update-event <calendar_id> <event_id>",
	Short: "更新日程",
	Long: `更新指定日程的信息。只更新提供的字段，未提供的字段保持不变。

参数:
  calendar_id       日历 ID
  event_id          日程 ID

可选参数:
  --summary, -s     日程标题
  --start           开始时间，RFC3339 格式
  --end             结束时间，RFC3339 格式
  --description, -d 日程描述
  --location, -l    地点
  --output, -o      输出格式（json）

时间格式:
  使用 RFC3339 格式，例如：
  - 2024-01-21T14:00:00+08:00
  - 2024-01-21T06:00:00Z

示例:
  # 更新日程标题
  feishu-cli calendar update-event CAL_ID EVENT_ID --summary "新标题"

  # 更新日程时间
  feishu-cli calendar update-event CAL_ID EVENT_ID \
    --start 2024-01-21T15:00:00+08:00 \
    --end 2024-01-21T16:00:00+08:00

  # 更新多个字段
  feishu-cli calendar update-event CAL_ID EVENT_ID \
    --summary "更新后的会议" \
    --description "会议内容已更新" \
    --location "会议室 B202"

  # JSON 格式输出
  feishu-cli calendar update-event CAL_ID EVENT_ID --summary "新标题" --output json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID := args[0]
		eventID := args[1]

		summary, _ := cmd.Flags().GetString("summary")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		output, _ := cmd.Flags().GetString("output")

		// 检查是否有任何更新字段
		if summary == "" && startTime == "" && endTime == "" && description == "" && location == "" {
			return fmt.Errorf("请至少提供一个要更新的字段（--summary, --start, --end, --description, --location）")
		}

		params := &client.UpdateEventParams{
			CalendarID:  calendarID,
			EventID:     eventID,
			Summary:     summary,
			StartTime:   startTime,
			EndTime:     endTime,
			Description: description,
			Location:    location,
		}

		event, err := client.UpdateEvent(params)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("日程更新成功！")
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
			if event.AppLink != "" {
				fmt.Printf("  链接:      %s\n", event.AppLink)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(updateEventCmd)
	updateEventCmd.Flags().StringP("summary", "s", "", "日程标题")
	updateEventCmd.Flags().String("start", "", "开始时间，RFC3339 格式")
	updateEventCmd.Flags().String("end", "", "结束时间，RFC3339 格式")
	updateEventCmd.Flags().StringP("description", "d", "", "日程描述")
	updateEventCmd.Flags().StringP("location", "l", "", "地点")
	updateEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
