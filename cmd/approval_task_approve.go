package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalTaskApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "通过审批任务",
	Long: `通过指定的审批任务（同意）。需要 User Token + scope approval:approval。

参数:
  --approval-code    审批定义 code（必填）
  --instance-code    审批实例 code（必填）
  --task-id          审批任务 ID（必填）
  --user-id          操作人用户 ID（必填）
  --comment          审批意见（可选）
  --user-id-type     open_id（默认）/user_id/union_id

示例:
  feishu-cli approval task approve \
    --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx \
    --comment "同意"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		opts, err := readApprovalTaskActionFlags(cmd)
		if err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)
		if err := client.ApproveApprovalTask(opts, token); err != nil {
			return err
		}

		fmt.Printf("已通过审批任务: %s\n", opts.TaskID)
		return nil
	},
}

// readApprovalTaskActionFlags 共享 approve/reject 两条命令的 flag 解析与校验。
func readApprovalTaskActionFlags(cmd *cobra.Command) (client.ApprovalTaskActionOptions, error) {
	approvalCode, _ := cmd.Flags().GetString("approval-code")
	if err := validateApprovalCode(approvalCode); err != nil {
		return client.ApprovalTaskActionOptions{}, err
	}

	instanceCode, _ := cmd.Flags().GetString("instance-code")
	if strings.TrimSpace(instanceCode) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--instance-code 不能为空")
	}

	taskID, _ := cmd.Flags().GetString("task-id")
	if strings.TrimSpace(taskID) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--task-id 不能为空")
	}

	userID, _ := cmd.Flags().GetString("user-id")
	if strings.TrimSpace(userID) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--user-id 不能为空")
	}

	comment, _ := cmd.Flags().GetString("comment")
	userIDType, _ := cmd.Flags().GetString("user-id-type")

	return client.ApprovalTaskActionOptions{
		ApprovalCode: approvalCode,
		InstanceCode: instanceCode,
		TaskID:       taskID,
		UserID:       userID,
		Comment:      comment,
		UserIDType:   userIDType,
	}, nil
}

func registerApprovalTaskActionFlags(cmd *cobra.Command) {
	cmd.Flags().String("approval-code", "", "审批定义 code（必填）")
	cmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	cmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	cmd.Flags().String("user-id", "", "操作人用户 ID（必填）")
	cmd.Flags().String("comment", "", "审批意见（可选）")
	cmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	cmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(cmd, "approval-code", "instance-code", "task-id", "user-id")
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskApproveCmd)
	registerApprovalTaskActionFlags(approvalTaskApproveCmd)
}
