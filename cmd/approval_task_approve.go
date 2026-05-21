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
	Long: `通过指定的审批任务（同意）。需要 User Token + scope approval:task:write。

参数:
  --instance-code    审批实例 code（必填）
  --task-id          审批任务 ID（必填）
  --comment          审批意见（可选）

示例:
  feishu-cli approval task approve \
    --instance-code <ic> --task-id <task> --comment "同意"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		opts, err := readApprovalTaskActionFlags(cmd)
		if err != nil {
			return err
		}

		token, errToken := requireUserToken(cmd, "approval task approve")
		if errToken != nil {
			return errToken
		}
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
	instanceCode, _ := cmd.Flags().GetString("instance-code")
	if strings.TrimSpace(instanceCode) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--instance-code 不能为空")
	}

	taskID, _ := cmd.Flags().GetString("task-id")
	if strings.TrimSpace(taskID) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--task-id 不能为空")
	}

	userID, _ := cmd.Flags().GetString("user-id")
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
	cmd.Flags().String("approval-code", "", "兼容旧参数：当前接口不使用")
	cmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	cmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	cmd.Flags().String("user-id", "", "兼容旧参数：当前接口不使用")
	cmd.Flags().String("comment", "", "审批意见（可选）")
	cmd.Flags().String("user-id-type", "open_id", "兼容旧参数：当前接口不使用")
	cmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	_ = cmd.Flags().MarkHidden("approval-code")
	_ = cmd.Flags().MarkHidden("user-id")
	_ = cmd.Flags().MarkHidden("user-id-type")
	mustMarkFlagRequired(cmd, "instance-code", "task-id")
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskApproveCmd)
	registerApprovalTaskActionFlags(approvalTaskApproveCmd)
}
