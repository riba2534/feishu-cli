package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalGetCmd = &cobra.Command{
	Use:   "get <approval_code>",
	Short: "获取审批定义详情",
	Long: `获取指定审批定义的详细信息，包括名称、表单结构和审批节点。

底层接口:
  GET /open-apis/approval/v4/approvals/{approval_code}/detail

权限:
  User Token，scope: approval:approval:read

参数:
  approval_code   审批定义 Code（必填）
  --output, -o    输出格式，可选：json、raw-json

示例:
  # 获取审批定义详情
  feishu-cli approval get <approval_code>

  # 获取完整 JSON 输出
  feishu-cli approval get <approval_code> --output json

  # 获取原始 API 响应
  feishu-cli approval get <approval_code> --output raw-json

  # 指定语言
  feishu-cli approval get <approval_code> --locale zh-CN`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		approvalCode := args[0]
		if err := validateApprovalCode(approvalCode); err != nil {
			return err
		}

		locale, _ := cmd.Flags().GetString("locale")
		output, _ := cmd.Flags().GetString("output")
		token, err := requireUserToken(cmd, "approval get")
		if err != nil {
			return err
		}

		opts := client.GetApprovalOptions{Locale: locale}

		if output == "raw-json" {
			raw, err := client.GetApprovalDefinitionRaw(approvalCode, opts, token)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}

		approval, err := client.GetApprovalDefinition(approvalCode, opts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(approval)
		}

		fmt.Printf("审批定义详情:\n")
		fmt.Printf("  Code: %s\n", approval.ApprovalCode)
		fmt.Printf("  名称: %s\n", approval.ApprovalName)
		fmt.Printf("  节点数: %d\n", len(approval.NodeList))
		if len(approval.NodeList) > 0 {
			fmt.Printf("  节点列表:\n")
			for idx, node := range approval.NodeList {
				fmt.Printf("    %d. %s [%s]\n", idx+1, node.Name, node.NodeType)
			}
		}
		fmt.Printf("  使用 --output json 查看归一化结果，或 --output raw-json 查看飞书 API 原始响应\n")

		return nil
	},
}

func validateApprovalCode(approvalCode string) error {
	if !isValidToken(approvalCode) {
		return fmt.Errorf("无效的审批定义 code: %s", approvalCode)
	}
	return nil
}

func init() {
	approvalCmd.AddCommand(approvalGetCmd)

	approvalGetCmd.Flags().StringP("output", "o", "", "输出格式（json/raw-json）")
	approvalGetCmd.Flags().String("locale", "zh-CN", "返回结果语言，如 zh-CN、en-US")
	approvalGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
