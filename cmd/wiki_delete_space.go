package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// 轮询参数为 var 以便测试缩短等待。
var (
	wikiDeleteSpacePollAttempts = 30
	wikiDeleteSpacePollInterval = 2 * time.Second
)

var wikiDeleteSpaceCmd = &cobra.Command{
	Use:   "delete-space <space_id>",
	Short: "删除知识空间（异步任务，自动轮询直至完成）",
	Long: `删除整个知识空间。后端可能同步完成或转为异步任务，本命令会自动轮询任务状态直至成功 / 失败 / 超时。

参数:
  space_id    知识空间 ID（位置参数）

可选:
  --yes                  确认删除（高危操作，不传则拒绝执行）
  --as                   身份: bot|user|auto（默认 auto：User 优先，未配置 User 时用 Bot；
                         已配置 User 但解析/刷新失败时 fail-closed 报错，不会静默降级为 Bot）
  --output / -o          输出格式（json）
  --user-access-token    覆盖登录态

权限:
  - User Access Token（或 --as bot 的应用身份，需应用是该空间管理员）
  - wiki:space:write_only
  - wiki:space:read

续查:
  轮询窗口内未完成时输出 task_id 与续查命令：
  feishu-cli drive task-result --scenario wiki_delete_space --task-id <task_id> --as <身份>

示例:
  feishu-cli wiki delete-space SPACE_ID --yes
  feishu-cli wiki delete-space SPACE_ID --yes -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := strings.TrimSpace(args[0])
		if err := validateResourceIdentifier(spaceID, "space_id"); err != nil {
			return err
		}
		yes, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")

		if !yes {
			return clierr.ConfirmationRequiredf("delete-space 是高危且不可逆操作，请加 --yes 确认")
		}

		// 删除空间是不可逆写操作：不能用读类 helper 在 User Token 不可用时静默降级为 Bot
		// （身份不同、权限不同，失败原因会被掩盖）。auto 下已配置但不可用的 User Token 直接 fail-closed。
		userToken, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		identity := "bot"
		if userToken != "" {
			identity = "user"
		}

		fmt.Fprintf(os.Stderr, "提交删除请求 space_id=%s ...\n", spaceID)
		taskID, err := client.DeleteWikiSpace(spaceID, userToken)
		if err != nil {
			return err
		}

		result := map[string]any{
			"space_id": spaceID,
			"ready":    false,
			"failed":   false,
			"status":   "success",
		}

		if taskID == "" {
			// 同步删除完成
			result["ready"] = true
			result["status"] = "success"
			return printDeleteSpaceResult(result, output)
		}

		// 异步任务：轮询
		result["task_id"] = taskID
		result["status"] = "processing"
		fmt.Fprintf(os.Stderr, "后端转为异步任务 task_id=%s，开始轮询...\n", taskID)

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		resumeCmd := buildWikiDeleteSpaceResumeCmd(taskID, identity)
		status, ready, err := pollDeleteSpaceTask(ctx, taskID, userToken)
		if err != nil {
			return fmt.Errorf("%w\n可通过以下命令继续查询: %s", err, resumeCmd)
		}
		result["ready"] = ready
		result["failed"] = status.Failed()
		result["status"] = status.Status
		result["status_msg"] = status.StatusMsg
		if !ready {
			result["timed_out"] = true
			result["resume_command"] = resumeCmd
		}
		return printDeleteSpaceResult(result, output)
	},
}

// buildWikiDeleteSpaceResumeCmd 生成删除空间任务的续查命令（drive task-result 的 wiki_delete_space 场景）。
func buildWikiDeleteSpaceResumeCmd(taskID, identity string) string {
	return fmt.Sprintf("feishu-cli drive task-result --scenario wiki_delete_space --task-id %s --as %s", quotePOSIXShell(taskID), identity)
}

func pollDeleteSpaceTask(ctx context.Context, taskID, userToken string) (*client.WikiDeleteSpaceTaskStatus, bool, error) {
	var last client.WikiDeleteSpaceTaskStatus
	var lastErr error
	hadSuccessfulPoll := false
	for attempt := 1; attempt <= wikiDeleteSpacePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return &last, false, fmt.Errorf("知识空间删除轮询被取消 (task_id=%s): %w", taskID, ctx.Err())
			case <-time.After(wikiDeleteSpacePollInterval):
			}
		}
		st, err := client.GetWikiDeleteSpaceTask(taskID, userToken)
		if err != nil {
			// 单次查询失败视为瞬时错误继续轮询；限流时立即停止，避免继续加压
			if client.IsRateLimitError(err) {
				return &last, false, fmt.Errorf("查询 delete_space 任务被限流 (task_id=%s): %w", taskID, err)
			}
			lastErr = err
			fmt.Fprintf(os.Stderr, "  [%d/%d] 查询失败: %v\n", attempt, wikiDeleteSpacePollAttempts, err)
			continue
		}
		hadSuccessfulPoll = true
		last = *st
		if st.Ready() {
			fmt.Fprintf(os.Stderr, "任务完成 ✅\n")
			return st, true, nil
		}
		if st.Failed() {
			return st, false, fmt.Errorf("delete_space 任务失败: status=%s, msg=%s", st.Status, st.StatusMsg)
		}
		fmt.Fprintf(os.Stderr, "  [%d/%d] status=%s\n", attempt, wikiDeleteSpacePollAttempts, st.Status)
	}
	if !hadSuccessfulPoll && lastErr != nil {
		return &last, false, fmt.Errorf("知识空间删除任务已提交，但状态查询全部失败 (task_id=%s): %w", taskID, lastErr)
	}
	return &last, false, nil
}

func printDeleteSpaceResult(result map[string]any, output string) error {
	if output == "json" {
		return printJSON(result)
	}
	fmt.Printf("space_id:   %s\n", result["space_id"])
	fmt.Printf("ready:      %v\n", result["ready"])
	if v, ok := result["task_id"]; ok {
		fmt.Printf("task_id:    %s\n", v)
	}
	fmt.Printf("status:     %v\n", result["status"])
	if v, ok := result["status_msg"]; ok && v != "" {
		fmt.Printf("status_msg: %v\n", v)
	}
	if v, ok := result["timed_out"]; ok && v.(bool) {
		fmt.Printf("⚠ 轮询超时，任务可能仍在执行，请勿重复提交；续查: %v\n", result["resume_command"])
	}
	return nil
}

func init() {
	wikiCmd.AddCommand(wikiDeleteSpaceCmd)
	wikiDeleteSpaceCmd.Flags().Bool("yes", false, "确认高危操作（必填）")
	wikiDeleteSpaceCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	wikiDeleteSpaceCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addAsFlag(wikiDeleteSpaceCmd)
}
