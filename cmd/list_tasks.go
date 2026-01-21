package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listTasksCmd = &cobra.Command{
	Use:   "list",
	Short: "列出任务",
	Long: `列出用户的任务列表。

参数:
  --page-size       每页数量（默认: 50）
  --page-token      分页标记
  --completed       只显示已完成的任务
  --uncompleted     只显示未完成的任务
  --output, -o      输出格式（json）

示例:
  # 列出所有任务
  feishu-cli task list

  # 列出已完成的任务
  feishu-cli task list --completed

  # 列出未完成的任务
  feishu-cli task list --uncompleted

  # 分页查询
  feishu-cli task list --page-size 10

  # JSON 格式输出
  feishu-cli task list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		completedFlag, _ := cmd.Flags().GetBool("completed")
		uncompletedFlag, _ := cmd.Flags().GetBool("uncompleted")

		var completed *bool
		if completedFlag {
			t := true
			completed = &t
		} else if uncompletedFlag {
			f := false
			completed = &f
		}

		result, err := client.ListTasks(pageSize, pageToken, completed)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(result.Tasks) == 0 {
				fmt.Println("没有找到任务")
				return nil
			}

			fmt.Printf("共找到 %d 个任务:\n\n", len(result.Tasks))
			for i, task := range result.Tasks {
				status := "[ ]"
				if task.CompletedAt != "" {
					status = "[x]"
				}
				fmt.Printf("[%d] %s %s\n", i+1, status, task.Summary)
				fmt.Printf("    ID: %s\n", task.Guid)
				if task.Description != "" {
					// Truncate long descriptions
					desc := task.Description
					if len(desc) > 50 {
						desc = desc[:50] + "..."
					}
					fmt.Printf("    描述: %s\n", desc)
				}
				if task.DueTime != "" {
					fmt.Printf("    截止: %s\n", task.DueTime)
				}
				if task.CompletedAt != "" {
					fmt.Printf("    完成: %s\n", task.CompletedAt)
				}
				fmt.Println()
			}

			if result.HasMore {
				fmt.Printf("还有更多任务，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(listTasksCmd)
	listTasksCmd.Flags().Int("page-size", 50, "每页数量")
	listTasksCmd.Flags().String("page-token", "", "分页标记")
	listTasksCmd.Flags().Bool("completed", false, "只显示已完成的任务")
	listTasksCmd.Flags().Bool("uncompleted", false, "只显示未完成的任务")
	listTasksCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
