package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createTaskCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新任务",
	Long: `创建新的飞书任务。

参数:
  --summary, -s       任务标题（必填）
  --description, -d   任务描述
  --due               截止时间（格式: 2006-01-02 15:04:05 或 2006-01-02）
  --origin-href       任务来源链接
  --origin-platform   任务来源平台名称（默认: feishu-cli）
  --assignee          负责人，逗号分隔：ou_ 用户 / cli_ 应用（创建时一并指派）
  --follower          关注人，逗号分隔：ou_ 用户 / cli_ 应用
  --tasklist-id       放入的清单 GUID 或清单 applink
  --idempotency-key   幂等键（写入 client_token；同 key 重试不会重复建任务）
  --data              任意任务字段 JSON（内联或 @文件，如 custom_fields）；显式 flag 覆盖同名字段
  --dry-run           只预览请求，不执行（不联网、不读凭据）
  --as                身份：auto（默认）| user | bot
  --output, -o        输出格式（json）

示例:
  # 创建简单任务
  feishu-cli task create --summary "完成项目文档"

  # 创建带描述的任务
  feishu-cli task create --summary "代码审查" --description "审查 PR #123"

  # 创建带截止时间的任务
  feishu-cli task create --summary "提交报告" --due "2024-12-31 18:00:00"

  # 创建带来源链接的任务
  feishu-cli task create --summary "处理 Issue" --origin-href "https://github.com/example/repo/issues/1"

  # 创建时指派给自己并放进清单（幂等键防重试重复创建）
  feishu-cli task create --summary "跟进上线" --assignee ou_xxx --tasklist-id TASKLIST_GUID --idempotency-key deploy-0601

  # 用 --data 写自定义字段（先 dry-run 核对请求体）
  feishu-cli task create --summary "需求评审" --data '{"custom_fields":[{"guid":"CF_GUID","number_value":"3"}]}' --dry-run

  # JSON 格式输出
  feishu-cli task create --summary "测试任务" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
		description, _ := cmd.Flags().GetString("description")
		dueStr, _ := cmd.Flags().GetString("due")
		originHref, _ := cmd.Flags().GetString("origin-href")
		originPlatform, _ := cmd.Flags().GetString("origin-platform")
		in := taskCreateInput{
			Summary:        summary,
			Description:    description,
			Due:            dueStr,
			OriginHref:     originHref,
			OriginPlatform: originPlatform,
		}
		in.Assignee, _ = cmd.Flags().GetString("assignee")
		in.Follower, _ = cmd.Flags().GetString("follower")
		in.TasklistID, _ = cmd.Flags().GetString("tasklist-id")
		in.IdempotencyKey, _ = cmd.Flags().GetString("idempotency-key")
		in.Data, _ = cmd.Flags().GetString("data")

		// 本地参数校验先于凭据解析：--due 非法不会触发 token 刷新
		body, dueMs, err := buildTaskCreateBody(in)
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if taskDryRunRequested(cmd) {
			return printDryRunPlan(cmd, "task create 预览（未执行）", nil, []dryRunStep{{
				Method: "POST", URL: taskAPIPath("tasks"), Desc: "创建任务",
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
		if in.extended() {
			// 成员/清单/幂等键/--data 超出 SDK 结构体，按构造好的请求体原样发送
			task, err = client.CreateTaskWithBody(body, token)
		} else {
			task, err = client.CreateTask(client.CreateTaskOptions{
				Summary:        summary,
				Description:    description,
				DueTimestamp:   dueMs,
				OriginHref:     originHref,
				OriginPlatform: originPlatform,
			}, token)
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
			fmt.Printf("任务创建成功！\n")
			fmt.Printf("  任务 ID: %s\n", task.Guid)
			fmt.Printf("  标题: %s\n", task.Summary)
			if task.Description != "" {
				fmt.Printf("  描述: %s\n", task.Description)
			}
			if task.DueTime != "" {
				fmt.Printf("  截止时间: %s\n", task.DueTime)
			}
			if task.OriginHref != "" {
				fmt.Printf("  来源链接: %s\n", task.OriginHref)
			}
		}

		return nil
	},
}

// parseTime parses a time string in various formats
func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("无法识别的时间格式: %s", s)
}

func init() {
	taskCmd.AddCommand(createTaskCmd)
	createTaskCmd.Flags().StringP("summary", "s", "", "任务标题（必填）")
	createTaskCmd.Flags().StringP("description", "d", "", "任务描述")
	createTaskCmd.Flags().String("due", "", "截止时间（格式: 2006-01-02 15:04:05）")
	createTaskCmd.Flags().String("origin-href", "", "任务来源链接")
	createTaskCmd.Flags().String("origin-platform", "", "任务来源平台名称")
	createTaskCmd.Flags().String("assignee", "", "负责人，逗号分隔：ou_ 用户 / cli_ 应用")
	createTaskCmd.Flags().String("follower", "", "关注人，逗号分隔：ou_ 用户 / cli_ 应用")
	createTaskCmd.Flags().String("tasklist-id", "", "放入的清单 GUID 或清单 applink")
	createTaskCmd.Flags().String("idempotency-key", "", "幂等键（client_token），重试不重复创建")
	createTaskCmd.Flags().String("data", "", "任意任务字段 JSON（内联或 @文件）；显式 flag 覆盖同名字段")
	addTaskDryRunFlag(createTaskCmd)
	createTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	createTaskCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(createTaskCmd)
	mustMarkFlagRequired(createTaskCmd, "summary")
}
