package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var reopenTaskCmd = &cobra.Command{
	Use:   "reopen <task_guid>",
	Short: "重新打开已完成的任务",
	Long: `将已完成的任务重新打开，恢复为未完成状态。

参数:
  task_guid     任务 ID（必填）
  --output, -o  输出格式（json）

示例:
  # 重新打开任务
  feishu-cli task reopen e297ddff-06ca-4166-b917-4ce57cd3a7a0

  # JSON 格式输出
  feishu-cli task reopen e297ddff-06ca-4166-b917-4ce57cd3a7a0 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

		taskGuid, guidErr := parseTaskGUIDArg(args[0])
		if guidErr != nil {
			return guidErr
		}

		task, err := client.ReopenTask(taskGuid, token)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(task); err != nil {
				return err
			}
		} else {
			fmt.Printf("任务已重新打开！\n")
			fmt.Printf("  任务 ID: %s\n", task.Guid)
			fmt.Printf("  标题: %s\n", task.Summary)
			if task.DueTime != "" {
				fmt.Printf("  截止时间: %s\n", task.DueTime)
			}
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(reopenTaskCmd)
	reopenTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	reopenTaskCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(reopenTaskCmd)
}
