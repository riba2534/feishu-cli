package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarPrimaryCmd = &cobra.Command{
	Use:   "primary",
	Short: "获取主日历",
	Long: `获取当前用户的主日历信息。

示例:
  feishu-cli calendar primary
  feishu-cli calendar primary -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		output, _ := cmd.Flags().GetString("output")

		cal, err := client.GetPrimaryCalendar(token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(cal)
		}

		fmt.Printf("主日历信息:\n")
		fmt.Printf("  日历 ID:   %s\n", cal.CalendarID)
		fmt.Printf("  标题:      %s\n", cal.Summary)
		if cal.Description != "" {
			fmt.Printf("  描述:      %s\n", cal.Description)
		}
		fmt.Printf("  类型:      %s\n", cal.Type)
		fmt.Printf("  权限:      %s\n", cal.Permissions)
		fmt.Printf("  角色:      %s\n", cal.Role)

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarPrimaryCmd)
	calendarPrimaryCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarPrimaryCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
