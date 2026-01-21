package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getEventCmd = &cobra.Command{
	Use:   "get-event <calendar_id> <event_id>",
	Short: "获取日程详情",
	Long: `获取指定日程的详细信息。

参数:
  calendar_id   日历 ID
  event_id      日程 ID

日程状态（status）:
  tentative     暂定
  confirmed     确认
  cancelled     取消

可见性（visibility）:
  default       默认
  public        公开
  private       私密

示例:
  # 获取日程详情
  feishu-cli calendar get-event CAL_ID EVENT_ID

  # JSON 格式输出
  feishu-cli calendar get-event CAL_ID EVENT_ID --output json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID := args[0]
		eventID := args[1]
		output, _ := cmd.Flags().GetString("output")

		event, err := client.GetEvent(calendarID, eventID)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("日程详情:")
			fmt.Printf("  日程 ID:     %s\n", event.EventID)
			fmt.Printf("  标题:        %s\n", event.Summary)
			fmt.Printf("  开始时间:    %s\n", event.StartTime)
			fmt.Printf("  结束时间:    %s\n", event.EndTime)
			if event.TimeZone != "" {
				fmt.Printf("  时区:        %s\n", event.TimeZone)
			}
			if event.Description != "" {
				fmt.Printf("  描述:        %s\n", event.Description)
			}
			if event.Location != "" {
				fmt.Printf("  地点:        %s\n", event.Location)
			}
			if event.Status != "" {
				fmt.Printf("  状态:        %s\n", event.Status)
			}
			if event.Visibility != "" {
				fmt.Printf("  可见性:      %s\n", event.Visibility)
			}
			if event.OrganizerID != "" {
				fmt.Printf("  组织者日历:  %s\n", event.OrganizerID)
			}
			if event.CreateTime != "" {
				fmt.Printf("  创建时间:    %s\n", event.CreateTime)
			}
			if event.RecurringID != "" {
				fmt.Printf("  重复日程 ID: %s\n", event.RecurringID)
			}
			if event.IsException {
				fmt.Printf("  是否例外:    是\n")
			}
			if event.AppLink != "" {
				fmt.Printf("  链接:        %s\n", event.AppLink)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(getEventCmd)
	getEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
