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
	Long: `撤回一条已发起的审批实例，对齐官方 approval.instances.cancel。

底层接口:
  POST /open-apis/approval/v4/instances/uat_cancel

权限:
  User Token，scope: approval:instance:write

参数:
  --instance-code    审批实例 code（必填）

示例:
  feishu-cli approval instance cancel --instance-code <ic>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		instanceCode, _ := cmd.Flags().GetString("instance-code")
		if strings.TrimSpace(instanceCode) == "" {
			return fmt.Errorf("--instance-code 不能为空")
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval instance cancel")
		if err != nil {
			return err
		}

		err = client.CancelApprovalInstance(client.CancelApprovalInstanceOptions{
			InstanceCode: instanceCode,
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

	approvalInstanceCancelCmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	approvalInstanceCancelCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(approvalInstanceCancelCmd, "instance-code")
}
