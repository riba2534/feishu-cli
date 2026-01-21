package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listCalendarsCmd = &cobra.Command{
	Use:   "list",
	Short: "列出日历",
	Long: `列出当前用户/应用有权限访问的日历列表。

日历类型（type）:
  unknown     未知类型
  primary     用户或应用的主日历
  shared      共享日历
  google      Google 日历
  resource    资源日历
  exchange    Exchange 日历

角色（role）:
  unknown          未知角色
  free_busy_reader 游客
  reader           订阅者
  writer           编辑者
  owner            管理员

示例:
  # 列出所有日历
  feishu-cli calendar list

  # JSON 格式输出
  feishu-cli calendar list --output json

  # 指定每页数量
  feishu-cli calendar list --page-size 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		calendars, nextToken, hasMore, err := client.ListCalendars(pageSize, pageToken)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]interface{}{
				"calendars": calendars,
				"has_more":  hasMore,
			}
			if nextToken != "" {
				result["page_token"] = nextToken
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(calendars) == 0 {
				fmt.Println("未找到日历（可能没有访问权限）")
				return nil
			}

			fmt.Printf("共找到 %d 个日历:\n\n", len(calendars))
			for i, cal := range calendars {
				displayName := cal.Summary
				if cal.SummaryAlias != "" {
					displayName = cal.SummaryAlias
				}
				fmt.Printf("[%d] %s\n", i+1, displayName)
				fmt.Printf("    日历 ID:   %s\n", cal.CalendarID)
				if cal.Type != "" {
					fmt.Printf("    类型:      %s\n", cal.Type)
				}
				if cal.Role != "" {
					fmt.Printf("    角色:      %s\n", cal.Role)
				}
				if cal.Permissions != "" {
					fmt.Printf("    公开范围:  %s\n", cal.Permissions)
				}
				if cal.Description != "" {
					fmt.Printf("    描述:      %s\n", cal.Description)
				}
				fmt.Println()
			}

			if hasMore {
				fmt.Printf("还有更多日历，使用 --page-token %s 获取下一页\n", nextToken)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(listCalendarsCmd)
	listCalendarsCmd.Flags().Int("page-size", 50, "每页数量")
	listCalendarsCmd.Flags().String("page-token", "", "分页标记")
	listCalendarsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
