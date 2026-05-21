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
	Long: `通过指定的审批任务（同意），对齐官方 approval.tasks.approve。

底层接口:
  POST /open-apis/approval/v4/tasks/uat_approval

权限:
  User Token，scope: approval:task:write

参数:
  --instance-code    审批实例 code（必填）
  --task-id          审批任务 ID（必填）
  --comment          审批意见（可选）
  --form             表单数据 JSON 字符串（可选）

示例:
  feishu-cli approval task approve \
    --instance-code <ic> --task-id <task> --comment "同意"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := readApprovalTaskActionFlags(cmd)
		if err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval task approve")
		if err != nil {
			return err
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
	instanceCode, _ := cmd.Flags().GetString("instance-code")
	if strings.TrimSpace(instanceCode) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--instance-code 不能为空")
	}

	taskID, _ := cmd.Flags().GetString("task-id")
	if strings.TrimSpace(taskID) == "" {
		return client.ApprovalTaskActionOptions{}, fmt.Errorf("--task-id 不能为空")
	}

	comment, _ := cmd.Flags().GetString("comment")
	var form string
	if cmd.Flags().Lookup("form") != nil {
		form, _ = cmd.Flags().GetString("form")
	}

	return client.ApprovalTaskActionOptions{
		InstanceCode: instanceCode,
		TaskID:       taskID,
		Comment:      comment,
		Form:         form,
	}, nil
}

func validateApprovalWriteUserIDType(userIDType string) error {
	switch strings.TrimSpace(userIDType) {
	case "", "open_id", "user_id", "union_id":
		return nil
	default:
		return fmt.Errorf("user_id_type 不支持 %q，仅支持 open_id / user_id / union_id", userIDType)
	}
}

func validateApprovalCreateUserIDType(userIDType string) error {
	switch strings.TrimSpace(userIDType) {
	case "", "open_id", "user_id":
		return nil
	case "union_id":
		return fmt.Errorf("approval/v4/instances 不支持 user_id_type=%q（仅支持 open_id / user_id；该端点 body 只有 open_id 和 user_id 两个字段，参 SDK InstanceCreate struct）", userIDType)
	default:
		return fmt.Errorf("user_id_type 不支持 %q，仅支持 open_id / user_id", userIDType)
	}
}

func registerApprovalTaskActionFlags(cmd *cobra.Command, includeForm bool) {
	cmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	cmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	cmd.Flags().String("comment", "", "审批意见（可选）")
	if includeForm {
		cmd.Flags().String("form", "", "表单数据 JSON 字符串（可选）")
	}
	cmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(cmd, "instance-code", "task-id")
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskApproveCmd)
	registerApprovalTaskActionFlags(approvalTaskApproveCmd, true)
}
