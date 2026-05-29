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
	Short: "表单管理（create/list/get/patch/delete/detail/submit + field/questions CRUD）",
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
	Long: `PATCH /open-apis/bitable/v1/apps/{app_token}/tables/{table_id}/forms/{form_id}

便捷字段（仅显式设置的才提交）:
  --name                表单标题
  --description         表单描述
  --shared              是否开启共享（true/false）
  --shared-limit        分享范围: off | tenant_editable | anyone_editable
  --submit-limit-once   是否仅可提交一次（true/false）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。

注意：走 bitable/v1 端点——base/v3 的 forms patch 只收 name/description，
shared/shared_limit/submit_limit_once 是 bitable/v1 字段，故整体路由到 bitable/v1
（5 个字段同源，一次调用全生效）。`,
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
			return bitableReq{
				method: "PATCH",
				path:   client.BitableV1Path("apps", bt, "tables", tableID, "forms", formID),
				body:   body,
				useV1:  true,
			}
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
	Use:     "field",
	Aliases: []string{"questions"}, // form questions create/delete/list/patch 的别名（与 lark-cli base +form-questions-* 对齐）
	Short:   "表单问题管理（list/patch/create/delete）",
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
	Short: "批量更新表单问题（单次≤10）",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions

批量更新表单问题（collection 端点，单次最多 10 个，每个 item 必含 "id"）。
用 --questions 传问题数组，或 --config/--config-file 传完整请求体 {"questions":[...]}。
item 字段: id(必填)/title/description/required/visible/pre_field_id/option_display_mode 等。
示例: --questions '[{"id":"fldxxxxxx","title":"更新后的标题","required":true}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		body, err := buildFormQuestionsBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PATCH", path: formPath(bt, tableID, formID, "questions"), body: body}
		})
	},
}

// buildFormQuestionsBody 构造表单问题批量更新请求体 {"questions":[...]}。
// 优先 --config/--config-file（完整请求体）；否则用 --questions（仅问题数组，自动包一层 questions）。
func buildFormQuestionsBody(cmd *cobra.Command) (any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	if configJSON != "" || configFile != "" {
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
		if err != nil {
			return nil, err
		}
		var body any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, fmt.Errorf("解析 --config 失败: %w", err)
		}
		return body, nil
	}
	questionsJSON, _ := cmd.Flags().GetString("questions")
	if questionsJSON == "" {
		return nil, fmt.Errorf("需提供 --questions（问题数组）或 --config（完整请求体）")
	}
	var questions any
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return nil, fmt.Errorf("解析 --questions 失败: %w", err)
	}
	return map[string]any{"questions": questions}, nil
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
	bitableFormFieldPatchCmd.Flags().String("questions", "", "问题数组 JSON（每项必含 id，单次≤10）")
	bitableFormFieldPatchCmd.Flags().String("config", "", "完整 JSON 请求体 {\"questions\":[...]}（与 --questions 二选一）")
	bitableFormFieldPatchCmd.Flags().String("config-file", "", "JSON 请求体文件")
}
