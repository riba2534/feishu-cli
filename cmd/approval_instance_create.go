package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建审批实例（发起审批）",
	Long: `创建一条审批实例。需要 User Token + scope approval:approval。

参数:
  --approval-code   审批定义 code（必填）
  --user-id         发起人 ID（必填，open_id/user_id/union_id）
  --form            表单数据 JSON 字符串（与 --form-file 二选一）
  --form-file       表单数据 JSON 文件路径（与 --form 二选一）
  --user-id-type    open_id（默认）/user_id/union_id
  --department-id   发起人部门 ID（可选）
  --open-chat-id    审批结果推送到的群（可选）
  --output, -o      输出格式：json

示例:
  # 通过文件提交表单
  feishu-cli approval instance create --approval-code <code> --user-id ou_xxx --form-file form.json

  # 直接传 JSON 字符串
  feishu-cli approval instance create --approval-code <code> --user-id ou_xxx \
    --form '[{"id":"widget_1","type":"input","value":"内容"}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		approvalCode, _ := cmd.Flags().GetString("approval-code")
		if err := validateApprovalCode(approvalCode); err != nil {
			return err
		}

		userID, _ := cmd.Flags().GetString("user-id")
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("--user-id 不能为空")
		}

		formInline, _ := cmd.Flags().GetString("form")
		formFile, _ := cmd.Flags().GetString("form-file")
		formData, err := loadJSONInput(formInline, formFile, "form", "form-file", "表单数据")
		if err != nil {
			return err
		}
		// 校验为合法 JSON 数组，飞书 form 必须是数组（[{"id":...}]），
		// 否则服务端会返回不友好的参数错误。
		var arr []any
		if err := json.Unmarshal([]byte(formData), &arr); err != nil {
			return fmt.Errorf("表单数据必须是 JSON 数组，解析失败: %w", err)
		}

		userIDType, _ := cmd.Flags().GetString("user-id-type")
		departmentID, _ := cmd.Flags().GetString("department-id")
		openChatID, _ := cmd.Flags().GetString("open-chat-id")
		output, _ := cmd.Flags().GetString("output")

		opts := client.CreateApprovalInstanceOptions{
			ApprovalCode: approvalCode,
			UserID:       userID,
			Form:         formData,
			UserIDType:   userIDType,
			DepartmentID: departmentID,
			OpenChatID:   openChatID,
		}

		token := resolveOptionalUserTokenWithFallback(cmd)
		result, err := client.CreateApprovalInstance(opts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("审批实例已创建\n  instance_code: %s\n", result.InstanceCode)
		return nil
	},
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceCreateCmd)

	approvalInstanceCreateCmd.Flags().String("approval-code", "", "审批定义 code（必填）")
	approvalInstanceCreateCmd.Flags().String("user-id", "", "发起人用户 ID（必填）")
	approvalInstanceCreateCmd.Flags().String("form", "", "表单数据 JSON 字符串（与 --form-file 二选一）")
	approvalInstanceCreateCmd.Flags().String("form-file", "", "表单数据 JSON 文件路径")
	approvalInstanceCreateCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalInstanceCreateCmd.Flags().String("department-id", "", "发起人部门 ID（可选）")
	approvalInstanceCreateCmd.Flags().String("open-chat-id", "", "结果推送的群 ID（可选）")
	approvalInstanceCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	approvalInstanceCreateCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(approvalInstanceCreateCmd, "approval-code", "user-id")
}
