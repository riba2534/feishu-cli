package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalTaskTransferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "转交审批任务",
	Long: `将审批任务转交给另一位审批人，对齐官方 approval.tasks.transfer。

底层接口:
  POST /open-apis/approval/v4/tasks/uat_transfer

权限:
  User Token，scope: approval:task:write

示例:
  feishu-cli approval task transfer --instance-code <ic> --task-id <task> --transfer-user-id ou_xxx
  feishu-cli approval tasks transfer --instance-code <ic> --task-id <task> --transfer-user-id ou_xxx --comment "请代审"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		instanceCode, _ := cmd.Flags().GetString("instance-code")
		taskID, _ := cmd.Flags().GetString("task-id")
		transferUserID, _ := cmd.Flags().GetString("transfer-user-id")
		comment, _ := cmd.Flags().GetString("comment")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		if output != "" && output != "json" {
			return fmt.Errorf("无效的 --output: %s，有效值: json", output)
		}
		if err := validateApprovalWriteUserIDType(userIDType); err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval task transfer")
		if err != nil {
			return err
		}

		opts := client.TransferApprovalTaskOptions{
			InstanceCode:   instanceCode,
			TaskID:         taskID,
			TransferUserID: transferUserID,
			Comment:        comment,
			UserIDType:     userIDType,
		}
		if err := client.TransferApprovalTask(opts, token); err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"ok":               true,
				"instance_code":    instanceCode,
				"task_id":          taskID,
				"transfer_user_id": transferUserID,
			})
		}
		fmt.Printf("审批任务已转交: %s -> %s\n", taskID, transferUserID)
		return nil
	},
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskTransferCmd)
	approvalTaskTransferCmd.Flags().String("instance-code", "", "审批实例 Code（必填）")
	approvalTaskTransferCmd.Flags().String("task-id", "", "审批任务 ID（必填）")
	approvalTaskTransferCmd.Flags().String("transfer-user-id", "", "被转交用户 ID（必填）")
	approvalTaskTransferCmd.Flags().String("comment", "", "审批意见（可选）")
	approvalTaskTransferCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalTaskTransferCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	approvalTaskTransferCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(approvalTaskTransferCmd, "instance-code", "task-id", "transfer-user-id")
}
