package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarGetCmd = &cobra.Command{
	Use:   "get <calendar_id>",
	Short: "获取日历详情",
	Long: `获取指定日历的详细信息。

参数:
  calendar_id   日历 ID（必填，位置参数）

示例:
  feishu-cli calendar get CAL_xxxx
  feishu-cli calendar get CAL_xxxx -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID := args[0]
		output, _ := cmd.Flags().GetString("output")

		cal, err := client.GetCalendar(calendarID, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(cal)
		}

		fmt.Printf("日历 ID:   %s\n", cal.CalendarID)
		fmt.Printf("标题:      %s\n", cal.Summary)
		if cal.Description != "" {
			fmt.Printf("描述:      %s\n", cal.Description)
		}
		fmt.Printf("类型:      %s\n", cal.Type)
		fmt.Printf("权限:      %s\n", cal.Permissions)
		fmt.Printf("角色:      %s\n", cal.Role)
		if cal.SummaryAlias != "" {
			fmt.Printf("备注名:    %s\n", cal.SummaryAlias)
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarGetCmd)
	calendarGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
