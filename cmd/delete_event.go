package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteEventCmd = &cobra.Command{
	Use:   "delete-event <calendar_id> <event_id>",
	Short: "删除日程",
	Long: `删除指定日历中的日程。

参数:
  calendar_id   日历 ID
  event_id      日程 ID

注意:
  - 删除操作不可恢复
  - 如果日程是重复日程的一部分，只会删除该实例

示例:
  # 删除日程
  feishu-cli calendar delete-event CAL_ID EVENT_ID`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID := args[0]
		eventID := args[1]

		if err := client.DeleteEvent(calendarID, eventID); err != nil {
			return err
		}

		fmt.Printf("日程删除成功！（日程 ID: %s）\n", eventID)
		return nil
	},
}

func init() {
	calendarCmd.AddCommand(deleteEventCmd)
}
