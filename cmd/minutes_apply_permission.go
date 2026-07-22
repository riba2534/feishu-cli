package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var minutesApplyPermissionCmd = &cobra.Command{
	Use:   "apply-permission",
	Short: "申请妙记查看 / 编辑权限",
	Long: `为指定妙记申请查看或编辑权限。

使用飞书 POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/apply API。
当你对某个妙记没有访问权限（例如 minutes get 返回 2091005 无权限）时，用本命令发起权限申请。

参数:
  --minute-token  妙记 Token（必填）
  --perm          申请的权限类型：view（查看）/ edit（编辑），必填
  --output, -o    输出格式（json）

权限:
  需要 User Access Token + minutes:permission:apply 权限

示例:
  # 申请查看权限
  feishu-cli minutes apply-permission --minute-token obcnxxxx --perm view

  # 申请编辑权限（JSON 输出）
  feishu-cli minutes apply-permission --minute-token obcnxxxx --perm edit -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "minutes apply-permission")
		if err != nil {
			return err
		}

		minuteToken, _ := cmd.Flags().GetString("minute-token")
		perm, _ := cmd.Flags().GetString("perm")
		output, _ := cmd.Flags().GetString("output")

		if err := ensureMinuteToken(minuteToken); err != nil {
			return err
		}
		if err := validateEnum(perm, "权限类型", []string{"view", "edit"}); err != nil {
			return err
		}

		data, err := client.ApplyMinutePermission(minuteToken, perm, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"minute_token": minuteToken,
				"perm":         perm,
				"data":         json.RawMessage(data),
			})
		}

		fmt.Printf("已提交妙记权限申请:\n")
		fmt.Printf("  minute_token: %s\n", minuteToken)
		fmt.Printf("  申请权限:     %s\n", perm)
		return nil
	},
}

func init() {
	minutesCmd.AddCommand(minutesApplyPermissionCmd)
	minutesApplyPermissionCmd.Flags().String("minute-token", "", "妙记 Token（必填）")
	minutesApplyPermissionCmd.Flags().String("perm", "", "申请的权限类型：view / edit（必填）")
	minutesApplyPermissionCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	minutesApplyPermissionCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(minutesApplyPermissionCmd, "minute-token", "perm")
}
