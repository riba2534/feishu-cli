package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建审批实例（发起审批）",
	Long: `创建一条审批实例。需要 User Token + scope approval:instance:write。

底层接口:
  POST /open-apis/approval/v4/instances/initiate

发起人身份取当前 User Token，不传 user_id。

参数:
  --approval-code   审批定义 code（必填）
  --form            表单数据 JSON 字符串（与 --form-file 二选一，可选）
  --form-file       表单数据 JSON 文件路径（与 --form 二选一，可选）
  --node-approver   节点指定审批人 JSON，如 [{"key":"n1","value":["ou_xxx"]}]（可选）
  --node-cc         节点指定抄送人 JSON，格式同 --node-approver（可选）
  --uuid            幂等 uuid（可选）
  --output, -o      输出格式：json

示例:
  # 通过文件提交表单
  feishu-cli approval instance create --approval-code <code> --form-file form.json

  # 直接传 JSON 字符串
  feishu-cli approval instance create --approval-code <code> \
    --form '[{"id":"widget_1","type":"input","value":"内容"}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		approvalCode, _ := cmd.Flags().GetString("approval-code")
		if err := validateApprovalCode(approvalCode); err != nil {
			return err
		}

		formInline, _ := cmd.Flags().GetString("form")
		formFile, _ := cmd.Flags().GetString("form-file")
		var formData string
		if formInline != "" || formFile != "" {
			var err error
			formData, err = loadJSONInput(formInline, formFile, "form", "form-file", "表单数据")
			if err != nil {
				return err
			}
			var arr []any
			if err := json.Unmarshal([]byte(formData), &arr); err != nil {
				return fmt.Errorf("表单数据必须是 JSON 数组，解析失败: %w", err)
			}
		}

		nodeApproverRaw, nodeCCRaw, err := loadNodeApproverCC(cmd)
		if err != nil {
			return err
		}
		uuid, _ := cmd.Flags().GetString("uuid")
		output, _ := cmd.Flags().GetString("output")

		opts := client.CreateApprovalInstanceOptions{
			ApprovalCode:     approvalCode,
			Form:             formData,
			NodeApproverList: nodeApproverRaw,
			NodeCCList:       nodeCCRaw,
			UUID:             uuid,
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return approvalWriteDryRun(cmd, "approval instance create", "create", opts)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval instance create")
		if err != nil {
			return err
		}

		result, err := client.CreateApprovalInstance(opts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("审批实例已创建\n  instance_code: %s\n", result.InstanceCode)
		if result.InstanceLink != "" {
			fmt.Printf("  instance_link: %s\n", result.InstanceLink)
		}
		return nil
	},
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceCreateCmd)
	approvalInstanceCreateCmd.Flags().Bool("dry-run", false, "只预览请求，不执行（不联网、不解析身份）")

	approvalInstanceCreateCmd.Flags().String("approval-code", "", "审批定义 code（必填）")
	approvalInstanceCreateCmd.Flags().String("form", "", "表单数据 JSON 字符串（与 --form-file 二选一）")
	approvalInstanceCreateCmd.Flags().String("form-file", "", "表单数据 JSON 文件路径")
	approvalInstanceCreateCmd.Flags().String("node-approver", "", "节点指定审批人 JSON，如 [{\"key\":\"n1\",\"value\":[\"ou_xxx\"]}]（可选）")
	approvalInstanceCreateCmd.Flags().String("node-approver-file", "", "节点指定审批人 JSON 文件路径")
	approvalInstanceCreateCmd.Flags().String("node-cc", "", "节点指定抄送人 JSON，格式同 --node-approver（可选）")
	approvalInstanceCreateCmd.Flags().String("node-cc-file", "", "节点指定抄送人 JSON 文件路径")
	approvalInstanceCreateCmd.Flags().String("uuid", "", "幂等 uuid（可选）")
	approvalInstanceCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	approvalInstanceCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(approvalInstanceCreateCmd, "approval-code")
}

// loadNodeApproverCC 读取 --node-approver/--node-approver-file 与 --node-cc/--node-cc-file，
// 返回节点指定审批人/抄送人的 JSON 原文；两者都为空时对应返回 nil（不写入 body）。
func loadNodeApproverCC(cmd *cobra.Command) (json.RawMessage, json.RawMessage, error) {
	approverRaw, err := loadNodeJSONArrayFlag(cmd, "node-approver", "node-approver-file", "节点指定审批人")
	if err != nil {
		return nil, nil, err
	}
	ccRaw, err := loadNodeJSONArrayFlag(cmd, "node-cc", "node-cc-file", "节点指定抄送人")
	if err != nil {
		return nil, nil, err
	}
	return approverRaw, ccRaw, nil
}

func loadNodeJSONArrayFlag(cmd *cobra.Command, inlineFlag, fileFlag, label string) (json.RawMessage, error) {
	inline, _ := cmd.Flags().GetString(inlineFlag)
	file, _ := cmd.Flags().GetString(fileFlag)
	if inline == "" && file == "" {
		return nil, nil
	}
	s, err := loadJSONInput(inline, file, inlineFlag, fileFlag, label)
	if err != nil {
		return nil, err
	}
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, fmt.Errorf("--%s 必须是 JSON 数组: %w", inlineFlag, err)
	}
	return json.RawMessage(s), nil
}
