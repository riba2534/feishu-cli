package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取审批实例详情",
	Long: `获取单个审批实例详情，对齐官方 approval.instances.get。

底层接口:
  GET /open-apis/approval/v4/instances/uat_get

权限:
  User Token，scope: approval:instance:read

示例:
  feishu-cli approval instance get --instance-code <ic>
  feishu-cli approval instances get --instance-code <ic> --output json
  feishu-cli approval instance get --instance-code <ic> --output raw-json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		instanceCode, _ := cmd.Flags().GetString("instance-code")
		locale, _ := cmd.Flags().GetString("locale")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		if output != "" && output != "json" && output != "raw-json" {
			return fmt.Errorf("无效的 --output: %s，有效值: json/raw-json", output)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval instance get")
		if err != nil {
			return err
		}

		opts := client.GetApprovalInstanceOptions{
			InstanceCode: instanceCode,
			Locale:       locale,
			UserIDType:   userIDType,
		}
		if output == "raw-json" {
			raw, err := client.GetApprovalInstanceRaw(opts, token)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}

		data, err := client.GetApprovalInstance(opts, token)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(data)
		}

		fmt.Printf("审批实例详情\n")
		printApprovalInstanceField(data, "instance_code", "  实例 Code")
		printApprovalInstanceField(data, "definition_name", "  审批名称")
		printApprovalInstanceField(data, "status", "  状态")
		printApprovalInstanceField(data, "user_id", "  发起人")
		printApprovalInstanceField(data, "start_time", "  开始时间")
		printApprovalInstanceField(data, "end_time", "  结束时间")
		return nil
	},
}

func printApprovalInstanceField(data map[string]any, key, label string) {
	if v, ok := data[key]; ok && fmt.Sprint(v) != "" {
		fmt.Printf("%s: %v\n", label, v)
	}
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceGetCmd)
	approvalInstanceGetCmd.Flags().String("instance-code", "", "审批实例 Code（必填）")
	approvalInstanceGetCmd.Flags().String("locale", "", "语言，如 zh-CN / en-US / ja-JP")
	approvalInstanceGetCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalInstanceGetCmd.Flags().StringP("output", "o", "", "输出格式（json/raw-json）")
	approvalInstanceGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(approvalInstanceGetCmd, "instance-code")
}
