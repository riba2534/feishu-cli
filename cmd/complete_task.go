package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var completeTaskCmd = &cobra.Command{
	Use:   "complete <task_id>",
	Short: "完成任务",
	Long: `将指定的任务标记为已完成。

幂等：先读取任务，已完成的任务直接返回（JSON 带 already_completed=true），不会改写原完成时间。
身份 --as 默认 auto（已登录用本人身份；此前版本默认 Bot，Bot 不是任务成员时读写会失败）。

参数:
  task_id       任务 ID（必填）
  --output, -o  输出格式（json）

示例:
  # 完成任务
  feishu-cli task complete e297ddff-06ca-4166-b917-4ce57cd3a7a0

  # JSON 格式输出
  feishu-cli task complete e297ddff-06ca-4166-b917-4ce57cd3a7a0 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

		taskGuid := args[0]

		task, err := client.CompleteTask(taskGuid, token)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(task); err != nil {
				return err
			}
		} else {
			if task.AlreadyCompleted {
				fmt.Printf("任务此前已完成，未重复修改（完成时间保持不变）\n")
			} else {
				fmt.Printf("任务已完成！\n")
			}
			fmt.Printf("  任务 ID: %s\n", task.Guid)
			fmt.Printf("  标题: %s\n", task.Summary)
			if task.CompletedAt != "" {
				fmt.Printf("  完成时间: %s\n", task.CompletedAt)
			}
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(completeTaskCmd)
	completeTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	completeTaskCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(completeTaskCmd)
}
