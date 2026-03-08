package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarFreebusyCmd = &cobra.Command{
	Use:   "freebusy",
	Short: "查询忙闲信息",
	Long: `查询用户在指定时间段内的忙闲信息。

参数:
  --start       起始时间，RFC3339 格式（必填）
  --end         结束时间，RFC3339 格式（必填）
  --user-id     用户 ID（可选）

示例:
  feishu-cli calendar freebusy \
    --start 2024-01-21T00:00:00+08:00 \
    --end 2024-01-21T23:59:59+08:00
  feishu-cli calendar freebusy \
    --start 2024-01-21T00:00:00+08:00 \
    --end 2024-01-21T23:59:59+08:00 \
    --user-id ou_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		userID, _ := cmd.Flags().GetString("user-id")
		output, _ := cmd.Flags().GetString("output")

		result, err := client.ListFreebusy(startTime, endTime, userID, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result) == 0 {
			fmt.Println("该时间段内无忙碌时间")
			return nil
		}

		fmt.Printf("忙碌时间段（共 %d 个）:\n\n", len(result))
		for i, fb := range result {
			fmt.Printf("[%d] %s ~ %s\n", i+1, fb.StartTime, fb.EndTime)
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarFreebusyCmd)
	calendarFreebusyCmd.Flags().String("start", "", "起始时间，RFC3339 格式（必填）")
	calendarFreebusyCmd.Flags().String("end", "", "结束时间，RFC3339 格式（必填）")
	calendarFreebusyCmd.Flags().String("user-id", "", "用户 ID")
	calendarFreebusyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarFreebusyCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	mustMarkFlagRequired(calendarFreebusyCmd, "start", "end")
}
