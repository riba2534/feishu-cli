package cmd

import (
	"fmt"
	"strings"
	"time"

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
  --data            任意任务字段 JSON（内联或 @文件）；每个顶层键都计入 update_fields，显式 flag 覆盖同名字段
  --dry-run         只预览请求，不执行（不联网、不读凭据）
  --as              身份：auto（默认）| user | bot
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

  # 用 --data 更新自定义字段（先预览）
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --data '{"custom_fields":[{"guid":"CF_GUID","text_value":"done"}]}' --dry-run

  # JSON 格式输出
  feishu-cli task update e297ddff-06ca-4166-b917-4ce57cd3a7a0 --summary "新标题" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskGuid, guidErr := parseTaskGUIDArg(args[0])
		if guidErr != nil {
			return guidErr
		}

		summary, _ := cmd.Flags().GetString("summary")
		description, _ := cmd.Flags().GetString("description")
		dueStr, _ := cmd.Flags().GetString("due")
		completed, _ := cmd.Flags().GetBool("completed")
		data, _ := cmd.Flags().GetString("data")

		// 本地参数校验先于凭据解析：--due 非法不会触发 token 刷新
		body, err := buildTaskUpdateBody(taskUpdateInput{
			Summary:     summary,
			Description: description,
			Due:         dueStr,
			Completed:   completed,
			Data:        data,
			NowMs:       time.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if taskDryRunRequested(cmd) {
			return printDryRunPlan(cmd, "task update 预览（未执行）", nil, []dryRunStep{{
				Method: "PATCH", URL: taskAPIPath("tasks", taskGuid), Desc: "更新任务",
				Params: taskOpenIDParams(), Body: body,
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

		var task *client.TaskInfo
		if strings.TrimSpace(data) != "" {
			// --data 可写任意任务字段，按构造好的请求体原样发送
			task, err = client.PatchTaskWithBody(taskGuid, body, token)
		} else {
			opts := client.UpdateTaskOptions{
				Summary:     summary,
				Description: description,
				Completed:   completed,
			}
			opts.DueTimestamp, _ = parseTaskDueFlag(dueStr) // 已在上方校验过
			task, err = client.UpdateTask(taskGuid, opts, token)
		}
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(task); err != nil {
				return err
			}
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
	updateTaskCmd.Flags().String("data", "", "任意任务字段 JSON（内联或 @文件）；顶层键计入 update_fields")
	addTaskDryRunFlag(updateTaskCmd)
	updateTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	updateTaskCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(updateTaskCmd)
}
