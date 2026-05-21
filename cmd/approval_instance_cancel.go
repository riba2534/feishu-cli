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
	Long: `撤回一条已发起的审批实例。需要 User Token + scope approval:instance:write。

参数:
  --instance-code    审批实例 code（必填）

示例:
  feishu-cli approval instance cancel \
    --instance-code <ic>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		approvalCode, _ := cmd.Flags().GetString("approval-code")
		instanceCode, _ := cmd.Flags().GetString("instance-code")
		if strings.TrimSpace(instanceCode) == "" {
			return fmt.Errorf("--instance-code 不能为空")
		}
		userID, _ := cmd.Flags().GetString("user-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		token, errToken := requireUserToken(cmd, "approval instance cancel")
		if errToken != nil {
			return errToken
		}

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

	approvalInstanceCancelCmd.Flags().String("approval-code", "", "兼容旧参数：当前接口不使用")
	approvalInstanceCancelCmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	approvalInstanceCancelCmd.Flags().String("user-id", "", "兼容旧参数：当前接口不使用")
	approvalInstanceCancelCmd.Flags().String("user-id-type", "open_id", "兼容旧参数：当前接口不使用")
	approvalInstanceCancelCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	_ = approvalInstanceCancelCmd.Flags().MarkHidden("approval-code")
	_ = approvalInstanceCancelCmd.Flags().MarkHidden("user-id")
	_ = approvalInstanceCancelCmd.Flags().MarkHidden("user-id-type")
	mustMarkFlagRequired(approvalInstanceCancelCmd, "instance-code")
}
