package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskSubtaskCmd = &cobra.Command{
	Use:   "subtask",
	Short: "子任务管理",
	Long: `管理任务的子任务，支持创建和列出子任务。

子命令:
  create    创建子任务
  list      列出子任务

示例:
  feishu-cli task subtask create TASK_GUID --summary "子任务标题"
  feishu-cli task subtask list TASK_GUID`,
}

var taskSubtaskCreateCmd = &cobra.Command{
	Use:   "create <task_guid>",
	Short: "创建子任务",
	Long: `为指定任务创建子任务。

参数:
  task_guid         父任务 GUID（位置参数）
  --summary, -s     子任务标题（必填）

示例:
  feishu-cli task subtask create TASK_GUID --summary "完成单元测试"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		summary, _ := cmd.Flags().GetString("summary")
		output, _ := cmd.Flags().GetString("output")

		task, err := client.CreateSubtask(taskGuid, summary, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(task)
		}

		fmt.Println("子任务创建成功！")
		fmt.Printf("  任务 ID: %s\n", task.Guid)
		fmt.Printf("  标题: %s\n", task.Summary)

		return nil
	},
}

var taskSubtaskListCmd = &cobra.Command{
	Use:   "list <task_guid>",
	Short: "列出子任务",
	Long: `列出指定任务的所有子任务。

参数:
  task_guid     父任务 GUID（位置参数）

示例:
  feishu-cli task subtask list TASK_GUID
  feishu-cli task subtask list TASK_GUID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		tasks, nextPageToken, hasMore, err := client.ListSubtasks(taskGuid, pageSize, pageToken, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"tasks":           tasks,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(tasks) == 0 {
			fmt.Println("暂无子任务")
			return nil
		}

		fmt.Printf("子任务列表（共 %d 个）:\n\n", len(tasks))
		for i, task := range tasks {
			status := "未完成"
			if task.CompletedAt != "" {
				status = "已完成"
			}
			fmt.Printf("[%d] %s [%s]\n", i+1, task.Summary, status)
			fmt.Printf("    ID: %s\n", task.Guid)
			if task.DueTime != "" {
				fmt.Printf("    截止时间: %s\n", task.DueTime)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskSubtaskCmd)

	taskSubtaskCmd.AddCommand(taskSubtaskCreateCmd)
	taskSubtaskCreateCmd.Flags().StringP("summary", "s", "", "子任务标题（必填）")
	taskSubtaskCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskSubtaskCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskSubtaskCreateCmd, "summary")

	taskSubtaskCmd.AddCommand(taskSubtaskListCmd)
	taskSubtaskListCmd.Flags().Int("page-size", 0, "每页数量")
	taskSubtaskListCmd.Flags().String("page-token", "", "分页标记")
	taskSubtaskListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskSubtaskListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
