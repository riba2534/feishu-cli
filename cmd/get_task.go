package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getTaskCmd = &cobra.Command{
	Use:   "get <task_id>",
	Short: "获取任务详情",
	Long: `获取指定任务的详细信息。

参数:
  task_id     任务 ID（必填）
  --output, -o  输出格式（json）

示例:
  # 获取任务详情
  feishu-cli task get e297ddff-06ca-4166-b917-4ce57cd3a7a0

  # JSON 格式输出
  feishu-cli task get e297ddff-06ca-4166-b917-4ce57cd3a7a0 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		taskGuid := args[0]

		task, err := client.GetTask(taskGuid)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(task, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("任务详情:\n")
			fmt.Printf("  任务 ID: %s\n", task.Guid)
			fmt.Printf("  标题: %s\n", task.Summary)
			if task.Description != "" {
				fmt.Printf("  描述: %s\n", task.Description)
			}
			if task.DueTime != "" {
				fmt.Printf("  截止时间: %s\n", task.DueTime)
			}
			if task.CompletedAt != "" {
				fmt.Printf("  完成时间: %s\n", task.CompletedAt)
			} else {
				fmt.Printf("  状态: 未完成\n")
			}
			if task.Creator != "" {
				fmt.Printf("  创建者: %s\n", task.Creator)
			}
			if task.OriginHref != "" {
				fmt.Printf("  来源链接: %s\n", task.OriginHref)
			}
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(getTaskCmd)
	getTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
