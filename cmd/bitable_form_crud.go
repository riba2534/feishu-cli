package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// ==================== form create / delete ====================
// 端点 ground truth（已实测印证）：
//   POST   /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms
//   DELETE /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}
// form_id 即表单类型视图的 view_id。

var bitableFormCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建表单",
	Long: `POST /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms

便捷字段:
  --name          表单名称
  --description   表单描述（纯文本或 markdown 链接 [text](https://example.com)）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
		}
		body, err := buildFormCreateBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "tables", tableID, "forms"), body: body}
		})
	},
}

// buildFormCreateBody 优先 --config/--config-file；否则从便捷 flag 收集（只取显式设置的）。
func buildFormCreateBody(cmd *cobra.Command) (map[string]any, error) {
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
	if len(body) == 0 {
		return nil, fmt.Errorf("未提供任何字段（用 --name/--description 或 --config）")
	}
	return body, nil
}

var bitableFormDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除表单",
	Long: `DELETE /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}

form_id 即表单视图 view_id。删除表单为不可逆操作。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: formPath(bt, tableID, formID)}
		})
	},
}

var bitableFormListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出表单",
	Long: `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms

列出指定数据表下的所有表单（form 类型视图），与 form create（POST 同路径）成对。

分页:
  --page-size    每页数量（≤100）
  --page-token   翻页游标（从上一页返回的 page_token 取）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
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
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", bt, "tables", tableID, "forms"), params: params}
		})
	},
}

// ==================== form detail / submit（按分享 token，不含 base_token） ====================
// 端点 ground truth（已实测印证）：
//   POST /open-apis/base/v3/bases/tables/forms/detail   body {"share_token":"shr..."}
//   POST /open-apis/base/v3/bases/tables/forms/submit   body {"share_token":"shr...","content":{...字段...}}
// 注意：这两个端点路径不含 base_token / table_id / form_id，仅靠分享 token 定位表单。
// 因 bitableRun 强制要求 --base-token，这里用独立的 runFormShareToken。

func runFormShareToken(cmd *cobra.Command, segments []string, body any) error {
	if err := config.Validate(); err != nil {
		return err
	}
	path := client.BaseV3Path(segments...)
	req := bitableReq{method: "POST", path: path, body: body}

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		// dry-run 预览尊重 --format/--jq（与实调路径 renderBitableResult 一致）。
		o, oerr := output.ParseOptions(cmd)
		if oerr != nil {
			return oerr
		}
		return output.Render(o, map[string]any{
			"api":    "base/v3",
			"method": req.method,
			"path":   req.path,
			"body":   req.body,
		})
	}

	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	data, err := client.BaseV3Call(req.method, req.path, nil, req.body, token)
	if err != nil {
		return err
	}
	return renderBitableResult(cmd, data)
}

var bitableFormDetailCmd = &cobra.Command{
	Use:   "detail",
	Short: "按分享 token 获取表单详情",
	Long: `POST /open-apis/base/v3/bases/tables/forms/detail

通过表单分享链接中的 share_token 获取表单详情（无需 base_token）。
必填:
  --share-token   表单分享 token（shr 前缀，从分享链接提取）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shareToken, _ := cmd.Flags().GetString("share-token")
		if shareToken == "" {
			return fmt.Errorf("--share-token 必填")
		}
		body := map[string]any{"share_token": shareToken}
		return runFormShareToken(cmd, []string{"bases", "tables", "forms", "detail"}, body)
	},
}

var bitableFormSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "按分享 token 提交表单",
	Long: `POST /open-apis/base/v3/bases/tables/forms/submit

通过分享 token 提交表单数据（无需 base_token）。字段值放入请求体的 content 字段。

必填:
  --share-token   表单分享 token（shr 前缀）
  --content / --content-file   字段值 JSON（形如 {"评分":5,"评价":"很好"}）

注意:
  本命令不处理附件上传。如需提交附件，先用 record upload-attachment 思路
  上传得到 file_token，再把字段值写进 --content。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shareToken, _ := cmd.Flags().GetString("share-token")
		if shareToken == "" {
			return fmt.Errorf("--share-token 必填")
		}
		content, err := buildFormSubmitContent(cmd)
		if err != nil {
			return err
		}
		body := map[string]any{"share_token": shareToken, "content": content}
		return runFormShareToken(cmd, []string{"bases", "tables", "forms", "submit"}, body)
	},
}

// buildFormSubmitContent 解析 --content/--content-file 为字段值 map。
// content 直接就是「字段名→值」的裸 map（与飞书 form-submit 的 content 语义一致，区别于
// record 的 {"fields":{...}} 外层包装）。这里【不】做 fields 自动解包：对「恰好只有一个
// 名为 fields 的对象型字段」的表单，自动解包会静默丢掉外层、造成提交内容错误且难以察觉；
// 而误传 record 风格 {"fields":{...}} 的用户会收到飞书「无此字段」的明确报错，可立即纠正。
func buildFormSubmitContent(cmd *cobra.Command) (any, error) {
	contentJSON, _ := cmd.Flags().GetString("content")
	contentFile, _ := cmd.Flags().GetString("content-file")
	raw, err := loadJSONInput(contentJSON, contentFile, "content", "content-file", "字段值")
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("解析 --content 失败: %w", err)
	}
	return parsed, nil
}

// ==================== form questions create / delete ====================
// 端点 ground truth（已实测印证）：
//   POST   /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions  body {"questions":[...]}
//   DELETE /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions  body {"question_ids":[...]}
// 注意：均走 collection 端点（.../forms/{form_id}/questions），不是 per-question 路径。

var bitableFormQuestionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "为表单批量创建问题（单次≤10）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions

批量创建表单问题（collection 端点，单次最多 10 个）。
用 --questions 传问题数组，或 --config/--config-file 传完整请求体 {"questions":[...]}。
item 字段: title(必填)/type(text/number/select/datetime/user/attachment/location)/
  description/required/option_display_mode/multiple/options/style 等。
示例: --questions '[{"type":"text","title":"你的名字","required":true}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		body, err := buildFormQuestionsCreateBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: formPath(bt, tableID, formID, "questions"), body: body}
		})
	},
}

// buildFormQuestionsCreateBody 构造创建请求体 {"questions":[...]}。
// 优先 --config/--config-file（完整请求体）；否则用 --questions（仅问题数组，自动包一层）。
func buildFormQuestionsCreateBody(cmd *cobra.Command) (any, error) {
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

var bitableFormQuestionsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "批量删除表单问题（单次≤10）",
	Long: `DELETE /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/questions

批量删除表单问题（collection 端点，单次最多 10 个），body 字段为 question_ids。
用 --question-ids 传逗号分隔的问题 ID，或 --config/--config-file 传完整请求体。
示例: --question-ids fld001,fld002`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if tableID == "" || formID == "" {
			return fmt.Errorf("--table-id 和 --form-id 必填")
		}
		body, err := buildFormQuestionsDeleteBody(cmd)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: formPath(bt, tableID, formID, "questions"), body: body}
		})
	},
}

// buildFormQuestionsDeleteBody 构造删除请求体 {"question_ids":[...]}。
// 优先 --config/--config-file；否则用 --question-ids（逗号分隔，单次≤10）。
func buildFormQuestionsDeleteBody(cmd *cobra.Command) (any, error) {
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
	idsRaw, _ := cmd.Flags().GetString("question-ids")
	ids, err := parseQuestionIDs(idsRaw)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("需提供 --question-ids（逗号分隔或 JSON 数组）或 --config（完整请求体）")
	}
	if len(ids) > 10 {
		return nil, fmt.Errorf("单次最多 10 个，当前传入 %d 个", len(ids))
	}
	return map[string]any{"question_ids": ids}, nil
}

// parseQuestionIDs 兼容两种 --question-ids 输入：
//   - JSON 数组（TrimSpace 后以 '[' 开头，如 '["q1","q2"]'）→ json.Unmarshal 成 []string
//   - 逗号分隔（如 'q1,q2'）→ CSV 切分
//
// 避免用户直接粘 lark 风格的 JSON 数组时被当作单个含括号引号的脏 ID。
func parseQuestionIDs(s string) ([]string, error) {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "[") {
		var ids []string
		if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
			return nil, fmt.Errorf("解析 --question-ids JSON 数组失败: %w", err)
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if id = strings.TrimSpace(id); id != "" {
				out = append(out, id)
			}
		}
		return out, nil
	}
	return splitAndTrim(trimmed), nil
}

func init() {
	// form create / delete 挂到已有 form 组（bitable_form.go 的 bitableFormCmd）
	bitableFormCmd.AddCommand(bitableFormCreateCmd)
	addBitableWriteFlags(bitableFormCreateCmd)
	bitableFormCreateCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormCreateCmd.Flags().String("name", "", "表单名称")
	bitableFormCreateCmd.Flags().String("description", "", "表单描述")
	bitableFormCreateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableFormCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// form list（只读，base/v3 collection 端点）
	bitableFormCmd.AddCommand(bitableFormListCmd)
	addBitableCommonFlags(bitableFormListCmd)
	bitableFormListCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormListCmd.Flags().Int("page-size", 0, "分页大小（≤100）")
	bitableFormListCmd.Flags().String("page-token", "", "分页 token")

	bitableFormCmd.AddCommand(bitableFormDeleteCmd)
	addBitableWriteFlags(bitableFormDeleteCmd)
	bitableFormDeleteCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormDeleteCmd.Flags().String("form-id", "", "form_id（即表单视图 view_id，必填）")

	// form detail / submit（按分享 token，无 base-token）
	bitableFormCmd.AddCommand(bitableFormDetailCmd)
	bitableFormDetailCmd.Flags().String("share-token", "", "表单分享 token（shr 前缀，必填）")
	bitableFormDetailCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddFormatFlags(bitableFormDetailCmd)
	output.AddDryRunFlag(bitableFormDetailCmd)

	bitableFormCmd.AddCommand(bitableFormSubmitCmd)
	bitableFormSubmitCmd.Flags().String("share-token", "", "表单分享 token（shr 前缀，必填）")
	bitableFormSubmitCmd.Flags().String("content", "", "字段值 JSON（形如 {\"字段\":\"值\"}）")
	bitableFormSubmitCmd.Flags().String("content-file", "", "字段值 JSON 文件")
	bitableFormSubmitCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddFormatFlags(bitableFormSubmitCmd)
	output.AddDryRunFlag(bitableFormSubmitCmd)

	// form questions create / delete 挂到已有 form questions 组（bitable_form.go 的 bitableFormFieldCmd）
	bitableFormFieldCmd.AddCommand(bitableFormQuestionsCreateCmd)
	addBitableWriteFlags(bitableFormQuestionsCreateCmd)
	bitableFormQuestionsCreateCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormQuestionsCreateCmd.Flags().String("form-id", "", "form_id（必填）")
	bitableFormQuestionsCreateCmd.Flags().String("questions", "", "问题数组 JSON（每项含 title+type，单次≤10）")
	bitableFormQuestionsCreateCmd.Flags().String("config", "", "完整 JSON 请求体 {\"questions\":[...]}（与 --questions 二选一）")
	bitableFormQuestionsCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	bitableFormFieldCmd.AddCommand(bitableFormQuestionsDeleteCmd)
	addBitableWriteFlags(bitableFormQuestionsDeleteCmd)
	bitableFormQuestionsDeleteCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFormQuestionsDeleteCmd.Flags().String("form-id", "", "form_id（必填）")
	bitableFormQuestionsDeleteCmd.Flags().String("question-ids", "", "问题 ID 列表（逗号分隔，单次≤10）")
	bitableFormQuestionsDeleteCmd.Flags().String("config", "", "完整 JSON 请求体 {\"question_ids\":[...]}（与 --question-ids 二选一）")
	bitableFormQuestionsDeleteCmd.Flags().String("config-file", "", "JSON 请求体文件")
}
