package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskReminderCmd = &cobra.Command{
	Use:   "reminder",
	Short: "任务提醒管理",
	Long: `管理任务提醒，支持添加和移除提醒。

子命令:
  add       添加提醒
  remove    移除提醒

示例:
  feishu-cli task reminder add TASK_GUID --minutes 30
  feishu-cli task reminder remove TASK_GUID --ids REMINDER_ID1,REMINDER_ID2`,
}

var taskReminderAddCmd = &cobra.Command{
	Use:   "add <task_guid>",
	Short: "添加任务提醒",
	Long: `为任务添加提醒。提醒时间为相对于截止时间的提前分钟数。

参数:
  task_guid     任务 GUID（位置参数）
  --minutes     提前提醒的分钟数（必填），0 表示截止时提醒

示例:
  feishu-cli task reminder add TASK_GUID --minutes 30
  feishu-cli task reminder add TASK_GUID --minutes 0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		minutes, _ := cmd.Flags().GetInt("minutes")

		if err := client.AddTaskReminders(taskGuid, minutes, token); err != nil {
			return err
		}

		if minutes == 0 {
			fmt.Println("成功添加提醒：截止时提醒")
		} else {
			fmt.Printf("成功添加提醒：截止时间前 %d 分钟\n", minutes)
		}
		return nil
	},
}

var taskReminderRemoveCmd = &cobra.Command{
	Use:   "remove <task_guid>",
	Short: "移除任务提醒",
	Long: `移除任务的提醒。

参数:
  task_guid     任务 GUID（位置参数）
  --ids         提醒 ID 列表，逗号分隔（必填）

示例:
  feishu-cli task reminder remove TASK_GUID --ids REMINDER_ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		idsStr, _ := cmd.Flags().GetString("ids")

		var ids []string
		for _, id := range strings.Split(idsStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				ids = append(ids, id)
			}
		}

		if len(ids) == 0 {
			return fmt.Errorf("提醒 ID 列表不能为空")
		}

		if err := client.RemoveTaskReminders(taskGuid, ids, token); err != nil {
			return err
		}

		fmt.Printf("成功移除 %d 个提醒\n", len(ids))
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskReminderCmd)

	taskReminderCmd.AddCommand(taskReminderAddCmd)
	taskReminderAddCmd.Flags().Int("minutes", 0, "提前提醒的分钟数（必填），0 表示截止时提醒")
	taskReminderAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskReminderAddCmd, "minutes")

	taskReminderCmd.AddCommand(taskReminderRemoveCmd)
	taskReminderRemoveCmd.Flags().String("ids", "", "提醒 ID 列表，逗号分隔（必填）")
	taskReminderRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskReminderRemoveCmd, "ids")
}
