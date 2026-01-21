package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updateTaskCmd = &cobra.Command{
	Use:   "update <task_id>",
	Short: "更新任务",
	Long: `更新指定任务的信息。

参数:
  task_id           任务 ID（必填）
  --summary, -s     新的任务标题
  --description, -d 新的任务描述
  --due             新的截止时间（格式: 2006-01-02 15:04:05 或 2006-01-02）
  --completed       标记任务为已完成
  --output, -o      输出格式（json）

示例:
  # 更新任务标题
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --summary "新标题"

  # 更新任务描述
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --description "新描述"

  # 更新截止时间
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --due "2024-12-31 18:00:00"

  # 标记任务为已完成
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --completed

  # JSON 格式输出
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --summary "新标题" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		taskGuid := args[0]

		summary, _ := cmd.Flags().GetString("summary")
		description, _ := cmd.Flags().GetString("description")
		dueStr, _ := cmd.Flags().GetString("due")
		completed, _ := cmd.Flags().GetBool("completed")

		opts := client.UpdateTaskOptions{
			Summary:     summary,
			Description: description,
			Completed:   completed,
		}

		// Parse due time
		if dueStr != "" {
			dueTime, err := parseTime(dueStr)
			if err != nil {
				return fmt.Errorf("解析截止时间失败: %w", err)
			}
			opts.DueTimestamp = dueTime.UnixMilli()
		}

		// Check if any update is specified
		if summary == "" && description == "" && dueStr == "" && !completed {
			return fmt.Errorf("请指定要更新的字段（--summary, --description, --due 或 --completed）")
		}

		task, err := client.UpdateTask(taskGuid, opts)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(task, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("任务更新成功！\n")
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
			}
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(updateTaskCmd)
	updateTaskCmd.Flags().StringP("summary", "s", "", "新的任务标题")
	updateTaskCmd.Flags().StringP("description", "d", "", "新的任务描述")
	updateTaskCmd.Flags().String("due", "", "新的截止时间（格式: 2006-01-02 15:04:05）")
	updateTaskCmd.Flags().Bool("completed", false, "标记任务为已完成")
	updateTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
