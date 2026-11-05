package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ==================== field extension：字段 AI 扩展（LLM 补全） ====================
// 对齐官方 base +field-extension-get/update/update-cells（shortcuts/base/field_extension.go）：
//   GET  /bases/{bt}/tables/{tid}/fields/{fid}/field_extensions               读取扩展配置（base:field:read）
//   PUT  /bases/{bt}/tables/{tid}/fields/{fid}/field_extensions               安装/更新/清除扩展（base:field:update，高危）
//   POST /bases/{bt}/tables/{tid}/fields/{fid}/field_extensions/update_cells  触发整列/指定行重算（base:record:update，高危）

const fieldExtensionBuiltinLLM = "builtin_llm_completion"

type fieldExtensionUpdateBody struct {
	ExtensionID string                    `json:"extension_id"`
	Inputs      *fieldExtensionInputsBody `json:"inputs"`
}

type fieldExtensionInputsBody struct {
	Prompt []fieldExtensionPromptSegment `json:"prompt"`
}

type fieldExtensionPromptSegment struct {
	Type  string  `json:"type"`
	Text  *string `json:"text,omitempty"`
	Field *string `json:"field,omitempty"`
}

var bitableFieldExtensionCmd = &cobra.Command{
	Use:   "extension",
	Short: "字段 AI 扩展（get/update/update-cells）：给字段配置 LLM 生成并触发重算",
}

func bitableFieldExtensionPath(cmd *cobra.Command, baseToken string, extra ...string) string {
	tableID, _ := cmd.Flags().GetString("table-id")
	fieldID, _ := cmd.Flags().GetString("field-id")
	return bitableFieldPath(baseToken, strings.TrimSpace(tableID), append([]string{strings.TrimSpace(fieldID), "field_extensions"}, extra...)...)
}

// requireTableAndField 本地校验 --table-id / --field-id 非空白（cobra required 只拦未传，不拦空白）。
func requireTableAndField(cmd *cobra.Command) error {
	tableID, _ := cmd.Flags().GetString("table-id")
	fieldID, _ := cmd.Flags().GetString("field-id")
	if strings.TrimSpace(tableID) == "" {
		return clierr.Usagef("--table-id 必填")
	}
	if strings.TrimSpace(fieldID) == "" {
		return clierr.Usagef("--field-id 必填")
	}
	return nil
}

var bitableFieldExtensionGetCmd = &cobra.Command{
	Use:   "get",
	Short: "读取字段的 AI 扩展配置",
	Long: `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/field_extensions

返回 current_extension；为 null 表示该字段没有可识别的扩展配置。scope: base:field:read

示例:
  feishu-cli bitable field extension get --base-token bascnxxx --table-id tblxxx --field-id fldxxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTableAndField(cmd); err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: bitableFieldExtensionPath(cmd, bt)}
		})
	},
}

var bitableFieldExtensionUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "安装、更新或清除字段的 AI 扩展（高危，需确认）",
	Long: `PUT /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/field_extensions

--config / --config-file 传扩展配置 JSON 对象（本地严格校验，未知字段直接报错）:
  {"extension_id":"builtin_llm_completion",
   "inputs":{"prompt":[{"type":"text","text":"总结："},{"type":"field_ref","field":"描述"}]}}
  - extension_id 目前只接受 builtin_llm_completion（LLM 补全）
  - prompt 由若干段组成：type=text 带 text；type=field_ref 带 field（字段名或 ID）
  - 传 {} 表示清除该字段的扩展

会改变字段的生成逻辑，属于高危写操作：交互终端会二次确认；非交互环境必须加 --yes，否则 exit 10。
scope: base:field:update

示例:
  feishu-cli bitable field extension update --base-token bascnxxx --table-id tblxxx --field-id fldxxx \
    --config '{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"总结："},{"type":"field_ref","field":"描述"}]}}' --yes
  # 清除扩展
  feishu-cli bitable field extension update --base-token bascnxxx --table-id tblxxx --field-id fldxxx --config '{}' --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTableAndField(cmd); err != nil {
			return err
		}
		configJSON, _ := cmd.Flags().GetString("config")
		configFile, _ := cmd.Flags().GetString("config-file")
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "字段扩展配置")
		if err != nil {
			return err
		}
		body, err := parseFieldExtensionUpdateBody(raw)
		if err != nil {
			return err
		}
		fieldID, _ := cmd.Flags().GetString("field-id")
		return bitableExecuteConfirmed(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PUT", path: bitableFieldExtensionPath(cmd, bt), body: body}
		}, func(bitableReq) string {
			if _, clear := body.(map[string]any); clear {
				return fmt.Sprintf("确认清除字段 %s 的 AI 扩展配置？", strings.TrimSpace(fieldID))
			}
			return fmt.Sprintf("确认覆盖字段 %s 的 AI 扩展配置？", strings.TrimSpace(fieldID))
		}, nil)
	},
}

// parseFieldExtensionUpdateBody 按官方 schema 严格解析扩展配置：{} 表示清除，
// 否则必须是 builtin_llm_completion + 非空 prompt，未知字段报用法错误。
func parseFieldExtensionUpdateBody(raw string) (any, error) {
	var keys map[string]json.RawMessage
	if err := decodeFieldExtensionJSON(raw, &keys); err != nil {
		return nil, err
	}
	if keys == nil {
		return nil, clierr.Usagef("--config 必须是 JSON 对象（清除扩展传 {}）")
	}
	if len(keys) == 0 {
		return map[string]any{}, nil
	}
	var body fieldExtensionUpdateBody
	if err := decodeFieldExtensionJSON(raw, &body); err != nil {
		return nil, err
	}
	if body.ExtensionID == "" {
		return nil, clierr.Usagef("--config.extension_id 必须是 %q；清除扩展请传 {}", fieldExtensionBuiltinLLM)
	}
	if body.ExtensionID != fieldExtensionBuiltinLLM {
		return nil, clierr.Usagef("--config.extension_id 必须是 %q，当前 %q", fieldExtensionBuiltinLLM, body.ExtensionID)
	}
	if body.Inputs == nil {
		return nil, clierr.Usagef("--config.inputs 必填（%s 需要 prompt）", fieldExtensionBuiltinLLM)
	}
	if len(body.Inputs.Prompt) == 0 {
		return nil, clierr.Usagef("--config.inputs.prompt 至少需要一段")
	}
	for i, seg := range body.Inputs.Prompt {
		switch seg.Type {
		case "text":
			if seg.Text == nil {
				return nil, clierr.Usagef("--config.inputs.prompt[%d] 的 type 是 text 时必须带 text", i)
			}
			if seg.Field != nil {
				return nil, clierr.Usagef("--config.inputs.prompt[%d] 的 field 只能用于 type=field_ref", i)
			}
		case "field_ref":
			if seg.Field == nil || strings.TrimSpace(*seg.Field) == "" {
				return nil, clierr.Usagef("--config.inputs.prompt[%d] 的 type 是 field_ref 时必须带 field", i)
			}
			if seg.Text != nil {
				return nil, clierr.Usagef("--config.inputs.prompt[%d] 的 text 只能用于 type=text", i)
			}
		default:
			return nil, clierr.Usagef("--config.inputs.prompt[%d].type 只能是 text 或 field_ref，当前 %q", i, seg.Type)
		}
	}
	return body, nil
}

func decodeFieldExtensionJSON(raw string, dst any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return clierr.Usagef("解析 --config 失败（字段扩展配置）: %v", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return clierr.Usagef("--config 只能包含一个 JSON 对象")
	}
	return nil
}

var bitableFieldExtensionUpdateCellsCmd = &cobra.Command{
	Use:   "update-cells",
	Short: "触发字段 AI 扩展重算（整列或指定行，高危，需确认）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/field_extensions/update_cells

启动一次单元格重算任务：
  --type column   整列重算（可用 --view-id 限定视图内可见记录）；会改写视图内每条记录的该字段
  --type row      指定行重算（--record-id 可重复，必填）

会批量改写单元格，属于高危写操作：交互终端会二次确认；非交互环境必须加 --yes，否则 exit 10。
scope: base:record:update

示例:
  feishu-cli bitable field extension update-cells --base-token bascnxxx --table-id tblxxx --field-id fldxxx --type column --view-id vewxxx --yes
  feishu-cli bitable field extension update-cells --base-token bascnxxx --table-id tblxxx --field-id fldxxx --type row --record-id recA --record-id recB --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTableAndField(cmd); err != nil {
			return err
		}
		body, err := buildFieldExtensionUpdateCellsBody(cmd)
		if err != nil {
			return err
		}
		fieldID, _ := cmd.Flags().GetString("field-id")
		return bitableExecuteConfirmed(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: bitableFieldExtensionPath(cmd, bt, "update_cells"), body: body}
		}, func(bitableReq) string {
			if body["type"] == "column" {
				return fmt.Sprintf("确认对字段 %s 整列重算（会改写视图内全部记录的该字段）？", strings.TrimSpace(fieldID))
			}
			return fmt.Sprintf("确认对字段 %s 的 %d 条记录重算？", strings.TrimSpace(fieldID), len(body["record_ids"].([]string)))
		}, nil)
	},
}

func buildFieldExtensionUpdateCellsBody(cmd *cobra.Command) (map[string]any, error) {
	updateType, _ := cmd.Flags().GetString("type")
	updateType = strings.TrimSpace(updateType)
	viewID, _ := cmd.Flags().GetString("view-id")
	viewID = strings.TrimSpace(viewID)
	rawIDs, _ := cmd.Flags().GetStringArray("record-id")
	seen := map[string]bool{}
	ids := make([]string, 0, len(rawIDs))
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, clierr.Usagef("--record-id 不能为空")
		}
		if seen[id] {
			return nil, clierr.Usagef("--record-id 重复: %s", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	body := map[string]any{"type": updateType}
	switch updateType {
	case "column":
		if len(ids) > 0 {
			return nil, clierr.Usagef("--record-id 只能用于 --type row")
		}
		if viewID != "" {
			body["view_id"] = viewID
		}
	case "row":
		if viewID != "" {
			return nil, clierr.Usagef("--view-id 只能用于 --type column")
		}
		if len(ids) == 0 {
			return nil, clierr.Usagef("--type row 必须提供至少一个 --record-id")
		}
		body["record_ids"] = ids
	default:
		return nil, clierr.Usagef("--type 只能是 column 或 row，当前 %q", updateType)
	}
	return body, nil
}

// ==================== field button-rule：按钮字段与工作流绑定 ====================
// 对齐官方 base +button-rule-bind/get/unbind（shortcuts/base/button_rule.go）：
//   [1] GET /bases/{bt}/tables/{tid}/fields/{field}   把 --field-id（字段名或 ID）解析为规范 fld ID
//   [2] PUT|GET /bases/{bt}/tables/{tid}/fields/{fld}/button_rule   body {workflow_id}（unbind 传空串）

var bitableFieldButtonRuleCmd = &cobra.Command{
	Use:   "button-rule",
	Short: "按钮字段与工作流绑定（bind/get/unbind）",
}

func newButtonRuleCmd(action string) *cobra.Command {
	c := &cobra.Command{Use: action}
	switch action {
	case "bind":
		c.Short = "把按钮字段绑定到工作流（点按钮触发该工作流）"
		c.Long = `PUT /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/button_rule  body {"workflow_id":"wkf..."}

先 GET 字段把 --field-id（字段名或 ID）解析成规范 fld ID，再写绑定。
--workflow-id 必须是 workflow create/list 返回的公开 wkf 前缀 ID（不是内部数字 ID）。
绑定与工作流启停无关：绑定后用 button-rule get 读回确认，需要生效时再 workflow enable。
绑定失败时保留已建的工作流和字段，直接重试本命令，不要重建。
scope: base:field:read、base:field:update

示例:
  feishu-cli bitable field button-rule bind --base-token bascnxxx --table-id tblxxx --field-id 审批按钮 --workflow-id wkfxxx`
	case "get":
		c.Short = "读取按钮字段绑定的目标"
		c.Long = `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/button_rule

未绑定时返回 bound=false、target=null；target.type=workflow 时 target.id 是可用于 workflow get/enable 的 wkf ID。
非按钮字段服务端报 800004098（field is not button field）。scope: base:field:read

示例:
  feishu-cli bitable field button-rule get --base-token bascnxxx --table-id tblxxx --field-id fldxxx`
	case "unbind":
		c.Short = "解除按钮字段的工作流绑定（不删除字段与工作流）"
		c.Long = `PUT /open-apis/base/v3/bases/{base_token}/tables/{table_id}/fields/{field_id}/button_rule  body {"workflow_id":""}

只移除按钮与工作流的关联，字段和工作流都保留；重复执行是安全的。scope: base:field:read、base:field:update

示例:
  feishu-cli bitable field button-rule unbind --base-token bascnxxx --table-id tblxxx --field-id fldxxx`
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if err := requireTableAndField(cmd); err != nil {
			return err
		}
		tableID, _ := cmd.Flags().GetString("table-id")
		tableID = strings.TrimSpace(tableID)
		fieldRef, _ := cmd.Flags().GetString("field-id")
		fieldRef = strings.TrimSpace(fieldRef)
		var body map[string]any
		method := "PUT"
		switch action {
		case "bind":
			workflowID, _ := cmd.Flags().GetString("workflow-id")
			workflowID = strings.TrimSpace(workflowID)
			if workflowID == "" {
				return clierr.Usagef("--workflow-id 必填")
			}
			if !strings.HasPrefix(workflowID, "wkf") {
				return clierr.Usagef("--workflow-id 必须是 wkf 前缀的公开工作流 ID（来自 workflow create/list），不能传内部数字 ID")
			}
			body = map[string]any{"workflow_id": workflowID}
		case "unbind":
			body = map[string]any{"workflow_id": ""}
		default:
			method = "GET"
		}
		resolvedBT := ""
		return bitableExecute(cmd, func(bt string) bitableReq {
			resolvedBT = bt
			second := map[string]any{
				"method": method,
				"path":   bitableFieldPath(bt, tableID) + "/<resolved_field_id>/button_rule",
				"desc":   "[2] 用第 1 步返回的规范字段 ID 读写按钮绑定",
			}
			if body != nil {
				second["body"] = body
			}
			req := bitableReq{
				method: method,
				path:   bitableFieldPath(bt, tableID, fieldRef),
				steps: []map[string]any{
					{"method": "GET", "path": bitableFieldPath(bt, tableID, fieldRef), "desc": "[1] 把 --field-id（字段名或 ID）解析为规范 fld ID"},
					second,
				},
			}
			if body != nil {
				req.body = body // 避免 GET 带上 typed-nil body
			}
			return req
		}, func(req bitableReq, token string) (any, error) {
			field, err := client.BaseV3Call("GET", req.path, nil, nil, token)
			if err != nil {
				return nil, err
			}
			fieldID := bitableStr(field, "id", "field_id")
			if !strings.HasPrefix(fieldID, "fld") {
				return nil, fmt.Errorf("字段解析响应缺少规范字段 ID（得到 %q）", fieldID)
			}
			return client.BaseV3Call(req.method, bitableFieldPath(resolvedBT, tableID, fieldID, "button_rule"), nil, req.body, token)
		})
	}
	return c
}

func init() {
	bitableFieldCmd.AddCommand(bitableFieldExtensionCmd)
	for _, c := range []*cobra.Command{bitableFieldExtensionGetCmd, bitableFieldExtensionUpdateCmd, bitableFieldExtensionUpdateCellsCmd} {
		bitableFieldExtensionCmd.AddCommand(c)
		addBitableWriteFlags(c)
		c.Flags().String("table-id", "", "table_id 或数据表名称（必填）")
		c.Flags().String("field-id", "", "field_id 或字段名称（必填）")
		mustMarkFlagRequired(c, "table-id", "field-id")
	}
	bitableFieldExtensionUpdateCmd.Flags().String("config", "", "字段扩展配置 JSON 对象（{} 表示清除）")
	bitableFieldExtensionUpdateCmd.Flags().String("config-file", "", "字段扩展配置 JSON 文件")
	bitableFieldExtensionUpdateCellsCmd.Flags().String("type", "", "重算范围: column | row（必填）")
	bitableFieldExtensionUpdateCellsCmd.Flags().String("view-id", "", "视图 ID（仅 --type column）")
	bitableFieldExtensionUpdateCellsCmd.Flags().StringArray("record-id", nil, "记录 ID（可重复；--type row 必填）")
	mustMarkFlagRequired(bitableFieldExtensionUpdateCellsCmd, "type")

	bitableFieldCmd.AddCommand(bitableFieldButtonRuleCmd)
	for _, action := range []string{"bind", "get", "unbind"} {
		c := newButtonRuleCmd(action)
		bitableFieldButtonRuleCmd.AddCommand(c)
		addBitableWriteFlags(c)
		c.Flags().String("table-id", "", "table_id 或数据表名称（必填）")
		c.Flags().String("field-id", "", "按钮字段 ID 或名称（必填）")
		mustMarkFlagRequired(c, "table-id", "field-id")
		if action == "bind" {
			c.Flags().String("workflow-id", "", "wkf 前缀的公开工作流 ID（必填）")
			mustMarkFlagRequired(c, "workflow-id")
		}
	}
}
