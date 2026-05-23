package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var tasklistCmd = &cobra.Command{
	Use:   "tasklist",
	Short: "任务清单操作命令",
	Long: `任务清单操作命令，用于创建、查看、列出和删除任务清单。

子命令:
  create    创建任务清单
  get       获取清单详情
  list      列出所有清单
  delete    删除清单

示例:
  feishu-cli tasklist create --name "项目清单"
  feishu-cli tasklist get GUID
  feishu-cli tasklist list
  feishu-cli tasklist delete GUID`,
}

var tasklistCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建任务清单",
	Long: `创建新的任务清单。

参数:
  --name, -n    清单名称（必填）

示例:
  feishu-cli tasklist create --name "项目清单"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")

		tl, err := client.CreateTasklist(name, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Println("任务清单创建成功！")
		fmt.Printf("  清单 ID: %s\n", tl.Guid)
		fmt.Printf("  名称: %s\n", tl.Name)
		if tl.Url != "" {
			fmt.Printf("  链接: %s\n", tl.Url)
		}

		return nil
	},
}

var tasklistGetCmd = &cobra.Command{
	Use:   "get <tasklist_guid>",
	Short: "获取清单详情",
	Long: `获取任务清单的详细信息。

示例:
  feishu-cli tasklist get GUID
  feishu-cli tasklist get GUID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		tasklistGuid := args[0]
		output, _ := cmd.Flags().GetString("output")

		tl, err := client.GetTasklist(tasklistGuid, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Printf("清单 ID:   %s\n", tl.Guid)
		fmt.Printf("名称:      %s\n", tl.Name)
		if tl.Creator != "" {
			fmt.Printf("创建者:    %s\n", tl.Creator)
		}
		if tl.Owner != "" {
			fmt.Printf("负责人:    %s\n", tl.Owner)
		}
		if tl.Url != "" {
			fmt.Printf("链接:      %s\n", tl.Url)
		}
		if tl.CreatedAt != "" {
			fmt.Printf("创建时间:  %s\n", tl.CreatedAt)
		}
		if tl.UpdatedAt != "" {
			fmt.Printf("更新时间:  %s\n", tl.UpdatedAt)
		}

		return nil
	},
}

var tasklistListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出任务清单",
	Long: `列出所有任务清单。

示例:
  feishu-cli tasklist list
  feishu-cli tasklist list -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		lists, nextPageToken, hasMore, err := client.ListTasklists(pageSize, pageToken, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"tasklists":       lists,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(lists) == 0 {
			fmt.Println("暂无任务清单")
			return nil
		}

		fmt.Printf("任务清单列表（共 %d 个）:\n\n", len(lists))
		for i, tl := range lists {
			fmt.Printf("[%d] %s\n", i+1, tl.Name)
			fmt.Printf("    ID: %s\n", tl.Guid)
			if tl.CreatedAt != "" {
				fmt.Printf("    创建时间: %s\n", tl.CreatedAt)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

var tasklistDeleteCmd = &cobra.Command{
	Use:   "delete <tasklist_guid>",
	Short: "删除任务清单",
	Long: `删除指定的任务清单。

示例:
  feishu-cli tasklist delete GUID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		tasklistGuid := args[0]

		if err := client.DeleteTasklist(tasklistGuid, token); err != nil {
			return err
		}

		fmt.Println("任务清单删除成功")
		return nil
	},
}

var tasklistTaskAddCmd = &cobra.Command{
	Use:   "task-add <tasklist_guid>",
	Short: "将任务添加到清单",
	Long: `将一个或多个已有任务添加到指定的任务清单。

参数:
  tasklist_guid     清单 GUID（位置参数）
  --task-ids        任务 GUID 列表，逗号分隔（必填）

示例:
  feishu-cli tasklist task-add TASKLIST_GUID --task-ids TASK_GUID1
  feishu-cli tasklist task-add TASKLIST_GUID --task-ids TASK_GUID1,TASK_GUID2`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		tasklistGuid := args[0]
		taskIDsStr, _ := cmd.Flags().GetString("task-ids")
		output, _ := cmd.Flags().GetString("output")

		taskIDs := splitAndTrim(taskIDsStr)

		if len(taskIDs) == 0 {
			return fmt.Errorf("任务 ID 列表不能为空")
		}

		var lastTask *client.TaskInfo
		for _, taskID := range taskIDs {
			task, err := client.AddTaskToTasklist(taskID, tasklistGuid, token)
			if err != nil {
				return fmt.Errorf("添加任务 %s 到清单失败: %w", taskID, err)
			}
			lastTask = task
		}

		if output == "json" {
			return printJSON(lastTask)
		}

		fmt.Printf("成功将 %d 个任务添加到清单\n", len(taskIDs))
		return nil
	},
}

var tasklistTaskRemoveCmd = &cobra.Command{
	Use:   "task-remove <tasklist_guid>",
	Short: "将任务从清单中移除",
	Long: `将一个或多个任务从指定的任务清单中移除。

参数:
  tasklist_guid     清单 GUID（位置参数）
  --task-ids        任务 GUID 列表，逗号分隔（必填）

示例:
  feishu-cli tasklist task-remove TASKLIST_GUID --task-ids TASK_GUID1
  feishu-cli tasklist task-remove TASKLIST_GUID --task-ids TASK_GUID1,TASK_GUID2`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		tasklistGuid := args[0]
		taskIDsStr, _ := cmd.Flags().GetString("task-ids")
		output, _ := cmd.Flags().GetString("output")

		taskIDs := splitAndTrim(taskIDsStr)

		if len(taskIDs) == 0 {
			return fmt.Errorf("任务 ID 列表不能为空")
		}

		var lastTask *client.TaskInfo
		for _, taskID := range taskIDs {
			task, err := client.RemoveTaskFromTasklist(taskID, tasklistGuid, token)
			if err != nil {
				return fmt.Errorf("从清单移除任务 %s 失败: %w", taskID, err)
			}
			lastTask = task
		}

		if output == "json" {
			return printJSON(lastTask)
		}

		fmt.Printf("成功从清单中移除 %d 个任务\n", len(taskIDs))
		return nil
	},
}

var tasklistTasksCmd = &cobra.Command{
	Use:   "tasks <tasklist_guid>",
	Short: "列出清单中的任务",
	Long: `列出指定任务清单中的所有任务。

参数:
  tasklist_guid     清单 GUID（位置参数）

示例:
  feishu-cli tasklist tasks TASKLIST_GUID
  feishu-cli tasklist tasks TASKLIST_GUID --completed
  feishu-cli tasklist tasks TASKLIST_GUID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		tasklistGuid := args[0]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		var completed *bool
		if cmd.Flags().Changed("completed") {
			v, _ := cmd.Flags().GetBool("completed")
			completed = &v
		}

		tasks, nextPageToken, hasMore, err := client.ListTasklistTasks(tasklistGuid, pageSize, pageToken, completed, token)
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
			fmt.Println("清单中暂无任务")
			return nil
		}

		fmt.Printf("清单任务列表（共 %d 个）:\n\n", len(tasks))
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
			if task.SubtaskCount > 0 {
				fmt.Printf("    子任务数: %d\n", task.SubtaskCount)
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
	rootCmd.AddCommand(tasklistCmd)

	tasklistCmd.AddCommand(tasklistCreateCmd)
	tasklistCreateCmd.Flags().StringP("name", "n", "", "清单名称（必填）")
	tasklistCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(tasklistCreateCmd, "name")

	tasklistCmd.AddCommand(tasklistGetCmd)
	tasklistGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	tasklistCmd.AddCommand(tasklistListCmd)
	tasklistListCmd.Flags().Int("page-size", 0, "每页数量")
	tasklistListCmd.Flags().String("page-token", "", "分页标记")
	tasklistListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	tasklistCmd.AddCommand(tasklistDeleteCmd)
	tasklistDeleteCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	tasklistCmd.AddCommand(tasklistTaskAddCmd)
	tasklistTaskAddCmd.Flags().String("task-ids", "", "任务 GUID 列表，逗号分隔（必填）")
	tasklistTaskAddCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistTaskAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(tasklistTaskAddCmd, "task-ids")

	tasklistCmd.AddCommand(tasklistTaskRemoveCmd)
	tasklistTaskRemoveCmd.Flags().String("task-ids", "", "任务 GUID 列表，逗号分隔（必填）")
	tasklistTaskRemoveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistTaskRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(tasklistTaskRemoveCmd, "task-ids")

	tasklistCmd.AddCommand(tasklistTasksCmd)
	tasklistTasksCmd.Flags().Int("page-size", 0, "每页数量")
	tasklistTasksCmd.Flags().String("page-token", "", "分页标记")
	tasklistTasksCmd.Flags().Bool("completed", false, "只显示已完成的任务")
	tasklistTasksCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistTasksCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
