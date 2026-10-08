package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

const (
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
  --output / -o          输出格式（json）
  --user-access-token    覆盖登录态

权限:
  - User Access Token
  - wiki:space:write_only
  - wiki:space:read

示例:
  feishu-cli wiki delete-space SPACE_ID --yes
  feishu-cli wiki delete-space SPACE_ID --yes -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		yes, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")

		if !yes {
			return clierr.ConfirmationRequiredf("delete-space 是高危且不可逆操作，请加 --yes 确认")
		}

		userToken := resolveOptionalUserTokenWithFallback(cmd)

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
		status, ready, err := pollDeleteSpaceTask(ctx, taskID, userToken)
		if err != nil {
			return err
		}
		result["ready"] = ready
		result["failed"] = status.Failed()
		result["status"] = status.Status
		result["status_msg"] = status.StatusMsg
		if !ready {
			result["timed_out"] = true
		}
		return printDeleteSpaceResult(result, output)
	},
}

func pollDeleteSpaceTask(ctx context.Context, taskID, userToken string) (*client.WikiDeleteSpaceTaskStatus, bool, error) {
	var last client.WikiDeleteSpaceTaskStatus
	for attempt := 1; attempt <= wikiDeleteSpacePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return &last, false, ctx.Err()
			case <-time.After(wikiDeleteSpacePollInterval):
			}
		}
		st, err := client.GetWikiDeleteSpaceTask(taskID, userToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [%d/%d] 查询失败: %v\n", attempt, wikiDeleteSpacePollAttempts, err)
			continue
		}
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
		fmt.Printf("⚠ 轮询超时，可稍后通过 status 重新查询\n")
	}
	return nil
}

func init() {
	wikiCmd.AddCommand(wikiDeleteSpaceCmd)
	wikiDeleteSpaceCmd.Flags().Bool("yes", false, "确认高危操作（必填）")
	wikiDeleteSpaceCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	wikiDeleteSpaceCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
