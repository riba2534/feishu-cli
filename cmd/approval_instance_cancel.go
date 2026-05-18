package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceCancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "取消（撤回）审批实例",
	Long: `撤回一条已发起的审批实例。需要 User Token + scope approval:approval。

参数:
  --approval-code    审批定义 code（必填）
  --instance-code    审批实例 code（必填）
  --user-id          执行撤回的用户 ID（必填，通常为发起人）
  --user-id-type     open_id（默认）/user_id/union_id

示例:
  feishu-cli approval instance cancel \
    --approval-code <code> --instance-code <ic> --user-id ou_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		approvalCode, _ := cmd.Flags().GetString("approval-code")
		if err := validateApprovalCode(approvalCode); err != nil {
			return err
		}

		instanceCode, _ := cmd.Flags().GetString("instance-code")
		if strings.TrimSpace(instanceCode) == "" {
			return fmt.Errorf("--instance-code 不能为空")
		}

		userID, _ := cmd.Flags().GetString("user-id")
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("--user-id 不能为空")
		}

		userIDType, _ := cmd.Flags().GetString("user-id-type")
		token := resolveOptionalUserTokenWithFallback(cmd)

		err := client.CancelApprovalInstance(client.CancelApprovalInstanceOptions{
			ApprovalCode: approvalCode,
			InstanceCode: instanceCode,
			UserID:       userID,
			UserIDType:   userIDType,
		}, token)
		if err != nil {
			return err
		}

		fmt.Printf("审批实例已撤回: %s\n", instanceCode)
		return nil
	},
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceCancelCmd)

	approvalInstanceCancelCmd.Flags().String("approval-code", "", "审批定义 code（必填）")
	approvalInstanceCancelCmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	approvalInstanceCancelCmd.Flags().String("user-id", "", "执行撤回的用户 ID（必填）")
	approvalInstanceCancelCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalInstanceCancelCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(approvalInstanceCancelCmd, "approval-code", "instance-code", "user-id")
}
