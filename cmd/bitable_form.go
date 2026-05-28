package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== form 表单 ====================
var bitableFormCmd = &cobra.Command{
	Use:   "form",
	Short: "表单管理（get/patch + field list/patch）",
}

// formPath 构造 base/v3 表单路径。form_id 即对应表单视图的 view_id。
func formPath(baseToken, tableID, formID string, extra ...string) string {
	parts := []string{"bases", baseToken, "tables", tableID, "forms", formID}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

var bitableFormGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取表单元数据",
	Long: `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}

form_id 即表单类型视图的 view_id。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: formPath(bt, tableID, formID)}
		})
	},
}

var bitableFormPatchCmd = &cobra.Command{
	Use:   "patch",
	Short: "更新表单",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}

便捷字段（仅显式设置的才提交）:
  --name                表单标题
  --description         表单描述
  --shared              是否开启共享（true/false）
  --shared-limit        分享范围: off | tenant_editable | anyone_editable
  --submit-limit-once   是否仅可提交一次（true/false）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}

		body, err := buildFormPatchBody(cmd)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("未提供任何更新字段（用 --name/--description/--shared/... 或 --config）")
		}

		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PATCH", path: formPath(bt, tableID, formID), body: body}
		})
	},
}

// buildFormPatchBody 优先用 --config/--config-file；否则从便捷 flag 收集（只取显式设置的）。
func buildFormPatchBody(cmd *cobra.Command) (map[string]any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	if configJSON != "" || configFile != "" {
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, fmt.Errorf("解析 --config 失败: %w", err)
		}
		return body, nil
	}

	body := map[string]any{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		body["name"] = v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		body["description"] = v
	}
	if cmd.Flags().Changed("shared") {
		v, _ := cmd.Flags().GetBool("shared")
		body["shared"] = v
	}
	if cmd.Flags().Changed("shared-limit") {
		v, _ := cmd.Flags().GetString("shared-limit")
		if err := validateEnum(v, "shared-limit", []string{"off", "tenant_editable", "anyone_editable"}); err != nil {
			return nil, err
		}
		body["shared_limit"] = v
	}
	if cmd.Flags().Changed("submit-limit-once") {
		v, _ := cmd.Flags().GetBool("submit-limit-once")
		body["submit_limit_once"] = v
	}
	return body, nil
}

// ==================== form field（表单问题） ====================
var bitableFormFieldCmd = &cobra.Command{
	Use:   "field",
	Short: "表单问题管理（list/patch）",
}

var bitableFormFieldListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出表单问题",
	Long: `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions

注意：base/v3 路径段是 questions（bitable/v1 是 fields），语义同为表单问题。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		params := map[string]any{}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: formPath(bt, tableID, formID, "questions"), params: params}
		})
	},
}

var bitableFormFieldPatchCmd = &cobra.Command{
	Use:   "patch",
	Short: "更新表单问题",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions/{question_id}

通过 --config/--config-file 传 JSON 请求体（title/description/required/visible/pre_field_id 等）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		questionID, _ := cmd.Flags().GetString("question-id")
		if tableID == "" || formID == "" || questionID == "" {
			return fmt.Errorf("--table-id / --form-id / --question-id 必填")
		}
		configJSON, _ := cmd.Flags().GetString("config")
		configFile, _ := cmd.Flags().GetString("config-file")
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
		if err != nil {
			return err
		}
		var body any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return fmt.Errorf("解析 --config 失败: %w", err)
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PATCH", path: formPath(bt, tableID, formID, "questions", questionID), body: body}
		})
	},
}

func init() {
	bitableCmd.AddCommand(bitableFormCmd)

	// form get
	bitableFormCmd.AddCommand(bitableFormGetCmd)
	addBitableCommonFlags(bitableFormGetCmd)
	bitableFormGetCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormGetCmd.Flags().String("form-id", "", "form_id（即表单视图 view_id，必填）")

	// form patch
	bitableFormCmd.AddCommand(bitableFormPatchCmd)
	addBitableWriteFlags(bitableFormPatchCmd)
	bitableFormPatchCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormPatchCmd.Flags().String("form-id", "", "form_id（必填）")
	bitableFormPatchCmd.Flags().String("name", "", "表单标题")
	bitableFormPatchCmd.Flags().String("description", "", "表单描述")
	bitableFormPatchCmd.Flags().Bool("shared", false, "是否开启共享")
	bitableFormPatchCmd.Flags().String("shared-limit", "", "分享范围: off|tenant_editable|anyone_editable")
	bitableFormPatchCmd.Flags().Bool("submit-limit-once", false, "是否仅可提交一次")
	bitableFormPatchCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableFormPatchCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// form field
	bitableFormCmd.AddCommand(bitableFormFieldCmd)

	bitableFormFieldCmd.AddCommand(bitableFormFieldListCmd)
	addBitableCommonFlags(bitableFormFieldListCmd)
	bitableFormFieldListCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormFieldListCmd.Flags().String("form-id", "", "form_id（必填）")
	bitableFormFieldListCmd.Flags().Int("page-size", 0, "分页大小（≤100）")
	bitableFormFieldListCmd.Flags().String("page-token", "", "分页 token")

	bitableFormFieldCmd.AddCommand(bitableFormFieldPatchCmd)
	addBitableWriteFlags(bitableFormFieldPatchCmd)
	bitableFormFieldPatchCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormFieldPatchCmd.Flags().String("form-id", "", "form_id（必填）")
	bitableFormFieldPatchCmd.Flags().String("question-id", "", "question_id（必填）")
	bitableFormFieldPatchCmd.Flags().String("config", "", "JSON 请求体")
	bitableFormFieldPatchCmd.Flags().String("config-file", "", "JSON 请求体文件")
}
