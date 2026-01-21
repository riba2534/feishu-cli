package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var completeTaskCmd = &cobra.Command{
	Use:   "complete <task_id>",
	Short: "完成任务",
	Long: `将指定的任务标记为已完成。

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

		taskGuid := args[0]

		task, err := client.CompleteTask(taskGuid)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(task, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("任务已完成！\n")
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
}
