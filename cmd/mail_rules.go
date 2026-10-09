package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// 收信规则（对齐官方 mail +rule-*）：用语义别名描述条件与动作，CLI 负责编码为服务端整数枚举。
//
// 条件语法：field[:operator[:value]]，例如 from:contains:boss@example.com、subject:starts_with:[周报]、has_attachment、
// to_or_cc:contains_self。动作语法：kind[:key=value]，例如 mark_read、move_folder:folder_id=7001（也接受 move_folder:7001）。

type mailRuleAlias struct {
	Canonical string
	Code      int
	Label     string
	NeedsArg  bool
	Aliases   []string
}

var mailRuleFields = []mailRuleAlias{
	{Canonical: "from", Code: 1, Label: "发件人", NeedsArg: true, Aliases: []string{"sender"}},
	{Canonical: "to", Code: 2, Label: "收件人", NeedsArg: true, Aliases: []string{"recipient"}},
	{Canonical: "cc", Code: 3, Label: "抄送人", NeedsArg: true},
	{Canonical: "to_or_cc", Code: 4, Label: "收件人或抄送人", NeedsArg: true, Aliases: []string{"recipient_or_cc"}},
	{Canonical: "subject", Code: 6, Label: "主题", NeedsArg: true, Aliases: []string{"title"}},
	{Canonical: "body", Code: 7, Label: "正文", NeedsArg: true, Aliases: []string{"content"}},
	{Canonical: "attachment_name", Code: 8, Label: "附件名称", NeedsArg: true, Aliases: []string{"attach_name"}},
	{Canonical: "attachment_type", Code: 9, Label: "附件类型", NeedsArg: true, Aliases: []string{"attach_type"}},
	{Canonical: "any_address", Code: 10, Label: "任意地址", NeedsArg: true, Aliases: []string{"any_recipient"}},
	{Canonical: "all_mail", Code: 12, Label: "所有邮件", Aliases: []string{"all"}},
	{Canonical: "external", Code: 13, Label: "外部邮件", Aliases: []string{"external_mail"}},
	{Canonical: "spam", Code: 14, Label: "垃圾邮件", Aliases: []string{"is_spam"}},
	{Canonical: "not_spam", Code: 15, Label: "非垃圾邮件", Aliases: []string{"is_not_spam"}},
	{Canonical: "has_attachment", Code: 16, Label: "带附件", Aliases: []string{"has_attach"}},
}

var mailRuleOperators = []mailRuleAlias{
	{Canonical: "contains", Code: 1, Label: "包含", NeedsArg: true, Aliases: []string{"include"}},
	{Canonical: "not_contains", Code: 2, Label: "不包含", NeedsArg: true, Aliases: []string{"exclude"}},
	{Canonical: "starts_with", Code: 3, Label: "开头是", NeedsArg: true, Aliases: []string{"prefix"}},
	{Canonical: "ends_with", Code: 4, Label: "结尾是", NeedsArg: true, Aliases: []string{"suffix"}},
	{Canonical: "equals", Code: 5, Label: "等于", NeedsArg: true, Aliases: []string{"eq", "is"}},
	{Canonical: "not_equals", Code: 6, Label: "不等于", NeedsArg: true, Aliases: []string{"ne"}},
	{Canonical: "contains_self", Code: 7, Label: "包含自己", Aliases: []string{"self"}},
	{Canonical: "empty", Code: 10, Label: "为空", Aliases: []string{"is_empty"}},
}

var mailRuleActions = []mailRuleAlias{
	{Canonical: "archive", Code: 1, Label: "归档"},
	{Canonical: "delete_mail", Code: 2, Label: "删除邮件", Aliases: []string{"trash"}},
	{Canonical: "mark_read", Code: 3, Label: "标为已读", Aliases: []string{"read"}},
	{Canonical: "move_spam", Code: 4, Label: "移入垃圾邮件", Aliases: []string{"spam"}},
	{Canonical: "not_spam", Code: 5, Label: "永不视为垃圾邮件", Aliases: []string{"never_spam"}},
	{Canonical: "star", Code: 9, Label: "添加旗标", Aliases: []string{"flag", "add_flag", "add_star"}},
	{Canonical: "mute_notification", Code: 10, Label: "不弹出通知", Aliases: []string{"mute"}},
	{Canonical: "move_folder", Code: 11, Label: "移动到文件夹", NeedsArg: true, Aliases: []string{"folder", "move_to"}},
}

func mailRuleAliasByName(items []mailRuleAlias, name string) (mailRuleAlias, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, it := range items {
		if it.Canonical == name {
			return it, true
		}
		for _, a := range it.Aliases {
			if a == name {
				return it, true
			}
		}
	}
	return mailRuleAlias{}, false
}

func mailRuleAliasByCode(items []mailRuleAlias, code int) (mailRuleAlias, bool) {
	for _, it := range items {
		if it.Code == code {
			return it, true
		}
	}
	return mailRuleAlias{}, false
}

func mailRuleAliasNames(items []mailRuleAlias) string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Canonical)
	}
	return strings.Join(names, ", ")
}

// parseMailRuleCondition 解析条件语法为服务端条目 {type, operator?, input?}。
func parseMailRuleCondition(raw string) (map[string]any, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 3)
	field, ok := mailRuleAliasByName(mailRuleFields, parts[0])
	if !ok {
		return nil, clierr.Usagef("未知的条件字段 %q，可选: %s", parts[0], mailRuleAliasNames(mailRuleFields))
	}
	item := map[string]any{"type": field.Code}
	if !field.NeedsArg {
		if len(parts) > 1 && strings.TrimSpace(strings.Join(parts[1:], ":")) != "" {
			return nil, clierr.Usagef("条件 %s 不带操作符和值（得到 %q）", field.Canonical, raw)
		}
		return item, nil
	}
	if len(parts) < 2 {
		return nil, clierr.Usagef("条件 %s 需要操作符，例如 %s:contains:xxx（可选操作符: %s）", field.Canonical, field.Canonical, mailRuleAliasNames(mailRuleOperators))
	}
	op, ok := mailRuleAliasByName(mailRuleOperators, parts[1])
	if !ok {
		return nil, clierr.Usagef("未知的条件操作符 %q，可选: %s", parts[1], mailRuleAliasNames(mailRuleOperators))
	}
	item["operator"] = op.Code
	value := ""
	if len(parts) == 3 {
		value = strings.TrimSpace(parts[2])
	}
	if op.NeedsArg && value == "" {
		return nil, clierr.Usagef("条件 %s:%s 需要非空的值", field.Canonical, op.Canonical)
	}
	if !op.NeedsArg && value != "" {
		return nil, clierr.Usagef("操作符 %s 不带值", op.Canonical)
	}
	if value != "" {
		item["input"] = value
	}
	return item, nil
}

// parseMailRuleAction 解析动作语法为服务端条目 {type, input?}。
func parseMailRuleAction(raw string) (map[string]any, error) {
	kind, tail, hasTail := strings.Cut(strings.TrimSpace(raw), ":")
	action, ok := mailRuleAliasByName(mailRuleActions, kind)
	if !ok {
		return nil, clierr.Usagef("未知的动作 %q，可选: %s（自动转发/分享到会话/添加用户标签服务端暂不支持）", kind, mailRuleAliasNames(mailRuleActions))
	}
	item := map[string]any{"type": action.Code}
	tail = strings.TrimSpace(tail)
	if !action.NeedsArg {
		if hasTail && tail != "" {
			return nil, clierr.Usagef("动作 %s 不带参数", action.Canonical)
		}
		return item, nil
	}
	if key, val, ok := strings.Cut(tail, "="); ok {
		if strings.TrimSpace(key) != "folder_id" {
			return nil, clierr.Usagef("动作 %s 只接受 folder_id 参数", action.Canonical)
		}
		tail = strings.TrimSpace(val)
	}
	if tail == "" {
		return nil, clierr.Usagef("动作 %s 需要文件夹 ID，例如 move_folder:folder_id=7001（ID 可用 mail triage --list-folders 查看）", action.Canonical)
	}
	item["input"] = tail
	return item, nil
}

// parseMailRuleJSONItems 解析 --conditions/--actions 的 JSON 数组：
// 每项可以是语法字符串、服务端原始格式 {type, operator, input}，或语义对象（条件 {field, operator, value}，动作 {kind, folder_id}）。
func parseMailRuleJSONItems(raw string, isCondition bool) ([]map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, clierr.Usagef("JSON 解析失败: %v", err)
	}
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	var out []map[string]any
	for _, it := range list {
		switch val := it.(type) {
		case string:
			var item map[string]any
			var err error
			if isCondition {
				item, err = parseMailRuleCondition(val)
			} else {
				item, err = parseMailRuleAction(val)
			}
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		case map[string]any:
			if _, ok := val["type"]; ok {
				code, err := strconv.Atoi(mailAnyString(val["type"]))
				if err != nil {
					return nil, clierr.Usagef("原始条目的 type 必须是整数: %v", val["type"])
				}
				item := map[string]any{"type": code}
				if op := mailAnyString(val["operator"]); op != "" {
					n, err := strconv.Atoi(op)
					if err != nil {
						return nil, clierr.Usagef("原始条目的 operator 必须是整数: %v", val["operator"])
					}
					item["operator"] = n
				}
				if in := mailAnyString(val["input"]); in != "" {
					item["input"] = in
				}
				out = append(out, item)
				continue
			}
			var grammar string
			if isCondition {
				grammar = strings.TrimRight(strings.Join([]string{mailAnyString(val["field"]), mailAnyString(val["operator"]), mailAnyString(val["value"])}, ":"), ":")
			} else {
				grammar = mailAnyString(val["kind"])
				if f := mailAnyString(val["folder_id"]); f != "" {
					grammar += ":folder_id=" + f
				}
			}
			var item map[string]any
			var err error
			if isCondition {
				item, err = parseMailRuleCondition(grammar)
			} else {
				item, err = parseMailRuleAction(grammar)
			}
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		default:
			return nil, clierr.Usagef("不支持的条目类型: %v", it)
		}
	}
	return out, nil
}

// collectMailRuleItems 合并 --condition/--action（可重复）与 --conditions/--actions（JSON 或 @文件）。
func collectMailRuleItems(cmd *cobra.Command, grammarFlag, jsonFlag string, isCondition bool) ([]map[string]any, bool, error) {
	var items []map[string]any
	changed := cmd.Flags().Changed(grammarFlag) || cmd.Flags().Changed(jsonFlag)
	grammars, _ := cmd.Flags().GetStringArray(grammarFlag)
	for _, g := range grammars {
		var item map[string]any
		var err error
		if isCondition {
			item, err = parseMailRuleCondition(g)
		} else {
			item, err = parseMailRuleAction(g)
		}
		if err != nil {
			return nil, changed, err
		}
		items = append(items, item)
	}
	if raw, _ := cmd.Flags().GetString(jsonFlag); strings.TrimSpace(raw) != "" {
		text := raw
		if strings.HasPrefix(strings.TrimSpace(raw), "@") {
			data, err := readLocalInputFile(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
			if err != nil {
				return nil, changed, clierr.Usage(fmt.Errorf("读取 --%s 文件失败: %w", jsonFlag, err))
			}
			text = string(data)
		}
		more, err := parseMailRuleJSONItems(text, isCondition)
		if err != nil {
			return nil, changed, err
		}
		items = append(items, more...)
	}
	if len(items) > 100 {
		return nil, changed, clierr.Usagef("--%s 条目过多（%d > 100）", grammarFlag, len(items))
	}
	return items, changed, nil
}

// ==================== 规则读取与描述 ====================

type mailRuleView struct {
	RuleID      string         `json:"rule_id"`
	Name        string         `json:"name"`
	Enabled     bool           `json:"enabled"`
	Order       int            `json:"order"`
	Description string         `json:"description"`
	Raw         map[string]any `json:"raw"`
}

func listMailRuleViews(mailbox, token string) ([]mailRuleView, error) {
	data, err := client.ListMailRules(mailbox, token)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析收信规则列表失败: %w", err)
	}
	var list []any
	for _, key := range []string{"items", "rules"} {
		if l, ok := payload[key].([]any); ok {
			list = append(list, l...)
		}
	}
	views := make([]mailRuleView, 0, len(list))
	for i, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		enabled, _ := m["is_enable"].(bool)
		views = append(views, mailRuleView{
			RuleID:      mailAnyString(m["id"]),
			Name:        sanitizeMailSingleLine(mailAnyString(m["name"])),
			Enabled:     enabled,
			Order:       i + 1,
			Description: describeMailRule(m),
			Raw:         m,
		})
	}
	return views, nil
}

func findMailRule(mailbox, ruleID, token string) (*mailRuleView, error) {
	views, err := listMailRuleViews(mailbox, token)
	if err != nil {
		return nil, err
	}
	for i := range views {
		if views[i].RuleID == ruleID {
			return &views[i], nil
		}
	}
	return nil, clierr.Usagef("未找到收信规则 %s；可用 `feishu-cli mail rule-list` 查看", ruleID)
}

func mailRuleInt(v any) int {
	n, _ := strconv.Atoi(mailAnyString(v))
	return n
}

// describeMailRule 生成中文描述：满足全部/任一条件：A；B → 动作：X、Y。未知枚举原样标注。
func describeMailRule(m map[string]any) string {
	cond, _ := m["condition"].(map[string]any)
	act, _ := m["action"].(map[string]any)
	matchText := "满足全部条件"
	if mailRuleInt(cond["match_type"]) == 2 {
		matchText = "满足任一条件"
	}
	var conds []string
	if items, ok := cond["items"].([]any); ok {
		for _, it := range items {
			c, _ := it.(map[string]any)
			field, ok := mailRuleAliasByCode(mailRuleFields, mailRuleInt(c["type"]))
			text := fmt.Sprintf("未知字段(%v)", c["type"])
			if ok {
				text = field.Label
			}
			if opCode := mailRuleInt(c["operator"]); opCode != 0 {
				if op, ok := mailRuleAliasByCode(mailRuleOperators, opCode); ok {
					text += " " + op.Label
				} else {
					text += fmt.Sprintf(" 未知操作符(%d)", opCode)
				}
			}
			if in := mailAnyString(c["input"]); in != "" {
				text += " " + sanitizeMailSingleLine(in)
			}
			conds = append(conds, text)
		}
	}
	var acts []string
	if items, ok := act["items"].([]any); ok {
		for _, it := range items {
			a, _ := it.(map[string]any)
			text := fmt.Sprintf("未知动作(%v)", a["type"])
			if action, ok := mailRuleAliasByCode(mailRuleActions, mailRuleInt(a["type"])); ok {
				text = action.Label
			}
			if in := mailAnyString(a["input"]); in != "" {
				text += "(" + sanitizeMailSingleLine(in) + ")"
			}
			acts = append(acts, text)
		}
	}
	desc := matchText + "：" + strings.Join(conds, "；") + " → " + strings.Join(acts, "、")
	if stop, _ := m["ignore_the_rest_of_rules"].(bool); stop {
		desc += "（命中后不再执行后续规则）"
	}
	return desc
}

func printMailRuleViews(views []mailRuleView, asJSON bool) error {
	if asJSON {
		return printJSON(map[string]any{"rules": views, "count": len(views)})
	}
	if len(views) == 0 {
		fmt.Fprintln(os.Stderr, "没有收信规则。")
		return nil
	}
	for _, v := range views {
		state := "停用"
		if v.Enabled {
			state = "启用"
		}
		fmt.Printf("[%d] %s  %s（%s）\n    %s\n", v.Order, v.RuleID, v.Name, state, v.Description)
	}
	return nil
}

// ==================== 命令 ====================

func addMailRuleCommonFlags(cmd *cobra.Command) {
	cmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	cmd.Flags().StringP("output", "o", "", "输出格式（json）")
	cmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}

func addMailRuleWriteFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "规则名称")
	cmd.Flags().StringArray("condition", nil, "条件（可重复），语法 field[:operator[:value]]，如 from:contains:boss@example.com、has_attachment")
	cmd.Flags().String("conditions", "", "条件 JSON 数组（语法字符串 / {field,operator,value} / 原始 {type,operator,input}），或 @文件")
	cmd.Flags().StringArray("action", nil, "动作（可重复），如 mark_read、star、move_folder:folder_id=7001")
	cmd.Flags().String("actions", "", "动作 JSON 数组（语法字符串 / {kind,folder_id} / 原始 {type,input}），或 @文件")
	cmd.Flags().String("match", "all", "条件匹配方式：all（满足全部）| any（满足任一）")
	cmd.Flags().Bool("stop-after-match", false, "命中后不再执行后续规则")
	cmd.Flags().Bool("dry-run", false, "只打印将要发送的请求，不实际调用")
}

func mailRuleMatchType(cmd *cobra.Command) (int, error) {
	match, _ := cmd.Flags().GetString("match")
	switch strings.ToLower(strings.TrimSpace(match)) {
	case "", "all":
		return 1, nil
	case "any":
		return 2, nil
	}
	return 0, clierr.Usagef("--match 仅支持 all|any，得到 %q", match)
}

var mailRuleListCmd = &cobra.Command{
	Use:   "rule-list",
	Short: "列出收信规则（含中文描述）",
	Long: `列出邮箱收信规则，按执行顺序输出规则 ID、名称、启用状态与中文描述。

可选:
  --name-contains   按名称本地过滤
  --mailbox         邮箱地址（默认 me）
  -o json           JSON 输出（rules[].raw 为服务端原始规则）

示例:
  feishu-cli mail rule-list
  feishu-cli mail rule-list --name-contains 周报 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")
		filter, _ := cmd.Flags().GetString("name-contains")
		token, err := requireUserToken(cmd, "mail rule-list")
		if err != nil {
			return err
		}
		views, err := listMailRuleViews(mailbox, token)
		if err != nil {
			return err
		}
		if f := strings.TrimSpace(filter); f != "" {
			kept := views[:0]
			for _, v := range views {
				if strings.Contains(strings.ToLower(v.Name), strings.ToLower(f)) {
					kept = append(kept, v)
				}
			}
			views = kept
		}
		return printMailRuleViews(views, output == "json")
	},
}

var mailRuleGetCmd = &cobra.Command{
	Use:   "rule-get",
	Short: "查看单条收信规则",
	Long: `按 rule_id 查看收信规则（服务端无单条查询接口，CLI 从规则列表中查找）。

示例:
  feishu-cli mail rule-get --rule-id 7012345678901234567`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")
		ruleID, _ := cmd.Flags().GetString("rule-id")
		token, err := requireUserToken(cmd, "mail rule-get")
		if err != nil {
			return err
		}
		view, err := findMailRule(mailbox, strings.TrimSpace(ruleID), token)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(view)
		}
		return printMailRuleViews([]mailRuleView{*view}, false)
	},
}

var mailRuleCreateCmd = &cobra.Command{
	Use:   "rule-create",
	Short: "创建收信规则（语义化条件/动作）",
	Long: `创建收信规则。条件与动作用语义别名描述，CLI 负责编码为服务端枚举。

必填:
  --name                规则名称
  --condition/--conditions  至少一个条件
  --action/--actions        至少一个动作

条件字段: from, to, cc, to_or_cc, subject, body, attachment_name, attachment_type, any_address,
          all_mail, external, spam, not_spam, has_attachment（后 5 个不带操作符）
操作符:   contains, not_contains, starts_with, ends_with, equals, not_equals, contains_self, empty
动作:     archive, delete_mail, mark_read, move_spam, not_spam, star, mute_notification,
          move_folder:folder_id=<文件夹 ID>

可选:
  --match all|any       条件匹配方式（默认 all）
  --stop-after-match    命中后不再执行后续规则
  --disable             创建为停用状态
  --dry-run             只打印请求体，不实际创建

示例:
  feishu-cli mail rule-create --name "老板邮件加旗标" --condition from:contains:boss@example.com --action star --dry-run
  feishu-cli mail rule-create --name "周报归档" --condition subject:starts_with:[周报] --action mark_read --action move_folder:folder_id=7001`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")
		name, _ := cmd.Flags().GetString("name")
		disable, _ := cmd.Flags().GetBool("disable")
		stop, _ := cmd.Flags().GetBool("stop-after-match")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if strings.TrimSpace(name) == "" {
			return clierr.Usagef("--name 必填")
		}
		matchType, err := mailRuleMatchType(cmd)
		if err != nil {
			return err
		}
		conds, _, err := collectMailRuleItems(cmd, "condition", "conditions", true)
		if err != nil {
			return err
		}
		acts, _, err := collectMailRuleItems(cmd, "action", "actions", false)
		if err != nil {
			return err
		}
		if len(conds) == 0 || len(acts) == 0 {
			return clierr.Usagef("至少需要一个条件（--condition/--conditions）和一个动作（--action/--actions）")
		}
		body := map[string]any{
			"name":                     strings.TrimSpace(name),
			"is_enable":                !disable,
			"ignore_the_rest_of_rules": stop,
			"condition":                map[string]any{"match_type": matchType, "items": conds},
			"action":                   map[string]any{"items": acts},
		}
		if dryRun {
			return printJSON(map[string]any{"dry_run": true, "method": "POST", "path": client.MailRulesPath(mailbox), "body": body, "description": describeMailRule(normalizeMailRuleBody(body))})
		}
		token, err := requireUserToken(cmd, "mail rule-create")
		if err != nil {
			return err
		}
		data, err := client.CreateMailRule(mailbox, body, token)
		if err != nil {
			return err
		}
		var resp struct {
			Rule map[string]any `json:"rule"`
		}
		_ = json.Unmarshal(data, &resp)
		ruleID := mailAnyString(resp.Rule["id"])
		if output == "json" {
			return printJSON(map[string]any{"rule_id": ruleID, "rule": resp.Rule, "description": describeMailRule(normalizeMailRuleBody(body))})
		}
		fmt.Printf("收信规则已创建: %s\n  %s\n", ruleID, describeMailRule(normalizeMailRuleBody(body)))
		return nil
	},
}

// normalizeMailRuleBody 经 JSON round-trip 统一数字类型，供 describeMailRule 使用。
func normalizeMailRuleBody(body map[string]any) map[string]any {
	raw, _ := json.Marshal(body)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	_ = dec.Decode(&m)
	return m
}

// mergeMailRuleUpdate 读取当前规则后按显式传入的 flag 覆盖，其余字段保持不变（服务端为 PUT 全量语义）。
func mergeMailRuleUpdate(cmd *cobra.Command, current map[string]any) (map[string]any, []string, error) {
	body := map[string]any{}
	for _, key := range []string{"name", "is_enable", "ignore_the_rest_of_rules", "condition", "action"} {
		if v, ok := current[key]; ok {
			body[key] = v
		}
	}
	var changed []string
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		if strings.TrimSpace(name) == "" {
			return nil, nil, clierr.Usagef("--name 不能为空")
		}
		body["name"] = strings.TrimSpace(name)
		changed = append(changed, "name")
	}
	if cmd.Flags().Changed("enable") || cmd.Flags().Changed("disable") {
		enable, _ := cmd.Flags().GetBool("enable")
		disable, _ := cmd.Flags().GetBool("disable")
		if enable == disable {
			return nil, nil, clierr.Usagef("--enable 与 --disable 只能指定一个")
		}
		body["is_enable"] = enable
		changed = append(changed, "is_enable")
	}
	if cmd.Flags().Changed("stop-after-match") || cmd.Flags().Changed("continue-after-match") {
		stop, _ := cmd.Flags().GetBool("stop-after-match")
		cont, _ := cmd.Flags().GetBool("continue-after-match")
		if stop == cont {
			return nil, nil, clierr.Usagef("--stop-after-match 与 --continue-after-match 只能指定一个")
		}
		body["ignore_the_rest_of_rules"] = stop
		changed = append(changed, "ignore_the_rest_of_rules")
	}
	cond, _ := body["condition"].(map[string]any)
	if cond == nil {
		cond = map[string]any{}
	} else {
		copied := map[string]any{}
		for k, v := range cond {
			copied[k] = v
		}
		cond = copied
	}
	if cmd.Flags().Changed("match") {
		mt, err := mailRuleMatchType(cmd)
		if err != nil {
			return nil, nil, err
		}
		cond["match_type"] = mt
		changed = append(changed, "match")
	}
	conds, condChanged, err := collectMailRuleItems(cmd, "condition", "conditions", true)
	if err != nil {
		return nil, nil, err
	}
	if condChanged {
		if len(conds) == 0 {
			return nil, nil, clierr.Usagef("替换条件时至少需要一个条件")
		}
		cond["items"] = conds
		changed = append(changed, "conditions")
	}
	if len(cond) > 0 {
		body["condition"] = cond
	}
	acts, actChanged, err := collectMailRuleItems(cmd, "action", "actions", false)
	if err != nil {
		return nil, nil, err
	}
	if actChanged {
		if len(acts) == 0 {
			return nil, nil, clierr.Usagef("替换动作时至少需要一个动作")
		}
		body["action"] = map[string]any{"items": acts}
		changed = append(changed, "actions")
	}
	return body, changed, nil
}

var mailRuleUpdateCmd = &cobra.Command{
	Use:   "rule-update",
	Short: "更新收信规则（未指定的字段保持不变）",
	Long: `更新一条收信规则：先读取当前规则，只覆盖显式传入的字段，再整体写回（服务端为 PUT 全量语义）。
--condition/--conditions 传入时整体替换条件列表，--action/--actions 同理。

示例:
  feishu-cli mail rule-update --rule-id 701xxx --name "新名称" --dry-run
  feishu-cli mail rule-update --rule-id 701xxx --action mark_read --action star`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMailRuleUpdate(cmd, "mail rule-update", nil)
	},
}

func runMailRuleUpdate(cmd *cobra.Command, cmdName string, forceEnable *bool) error {
	if err := config.Validate(); err != nil {
		return err
	}
	mailbox, _ := cmd.Flags().GetString("mailbox")
	output, _ := cmd.Flags().GetString("output")
	ruleID, _ := cmd.Flags().GetString("rule-id")
	ruleID = strings.TrimSpace(ruleID)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if ruleID == "" {
		return clierr.Usagef("--rule-id 必填")
	}
	if forceEnable == nil {
		// 先离线校验 flag，避免无效输入触发网络请求
		if _, changed, err := mergeMailRuleUpdate(cmd, map[string]any{}); err != nil {
			return err
		} else if len(changed) == 0 {
			return clierr.Usagef("至少指定一项修改：--name / --enable / --disable / --match / --stop-after-match / --continue-after-match / --condition(s) / --action(s)")
		}
	}
	if dryRun {
		plan := map[string]any{"dry_run": true, "steps": []map[string]any{
			{"method": "GET", "path": client.MailRulesPath(mailbox), "note": "读取当前规则"},
			{"method": "PUT", "path": client.MailRulesPath(mailbox, ruleID), "note": "写回合并后的完整规则"},
		}}
		if forceEnable != nil {
			plan["is_enable"] = *forceEnable
		} else {
			patch, changed, _ := mergeMailRuleUpdate(cmd, map[string]any{})
			plan["changed"] = changed
			plan["patch"] = patch
		}
		return printJSON(plan)
	}
	token, err := requireUserToken(cmd, cmdName)
	if err != nil {
		return err
	}
	current, err := findMailRule(mailbox, ruleID, token)
	if err != nil {
		return err
	}
	var body map[string]any
	var changed []string
	if forceEnable != nil {
		body, _, err = mergeMailRuleUpdate(cmd, current.Raw)
		if err != nil {
			return err
		}
		body["is_enable"] = *forceEnable
		changed = []string{"is_enable"}
	} else {
		body, changed, err = mergeMailRuleUpdate(cmd, current.Raw)
		if err != nil {
			return err
		}
	}
	if _, err := client.UpdateMailRule(mailbox, ruleID, body, token); err != nil {
		return err
	}
	desc := describeMailRule(normalizeMailRuleBody(body))
	if output == "json" {
		return printJSON(map[string]any{"rule_id": ruleID, "updated": true, "changed": changed, "before": current.Description, "after": desc})
	}
	fmt.Printf("收信规则已更新: %s（修改: %s）\n  %s\n", ruleID, strings.Join(changed, ", "), desc)
	return nil
}

func newMailRuleToggleCmd(use string, enable bool) *cobra.Command {
	word := "停用"
	if enable {
		word = "启用"
	}
	return &cobra.Command{
		Use:   use,
		Short: word + "收信规则",
		Long: fmt.Sprintf(`%s一条收信规则（读取当前规则后只改启用状态再写回）。

示例:
  feishu-cli mail %s --rule-id 701xxx
  feishu-cli mail %s --rule-id 701xxx --dry-run`, word, use, use),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMailRuleUpdate(cmd, "mail "+use, &enable)
		},
	}
}

var mailRuleEnableCmd = newMailRuleToggleCmd("rule-enable", true)
var mailRuleDisableCmd = newMailRuleToggleCmd("rule-disable", false)

var mailRuleDeleteCmd = &cobra.Command{
	Use:   "rule-delete",
	Short: "删除收信规则（需确认）",
	Long: `删除一条收信规则。删除前会读取规则并在确认提示中展示其描述；
非交互环境需加 --yes；--dry-run 只打印请求，不调用接口（优先于确认）。

示例:
  feishu-cli mail rule-delete --rule-id 701xxx --dry-run
  feishu-cli mail rule-delete --rule-id 701xxx --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")
		ruleID, _ := cmd.Flags().GetString("rule-id")
		ruleID = strings.TrimSpace(ruleID)
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if ruleID == "" {
			return clierr.Usagef("--rule-id 必填")
		}
		if dryRun {
			return printJSON(map[string]any{"dry_run": true, "method": "DELETE", "path": client.MailRulesPath(mailbox, ruleID)})
		}
		token, err := requireUserToken(cmd, "mail rule-delete")
		if err != nil {
			return err
		}
		view, err := findMailRule(mailbox, ruleID, token)
		if err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("将删除收信规则 %s「%s」：%s，确认?", ruleID, view.Name, view.Description)); err != nil {
			return err
		}
		if err := client.DeleteMailRule(mailbox, ruleID, token); err != nil {
			return err
		}
		if output == "json" {
			return printJSON(map[string]any{"rule_id": ruleID, "deleted": true, "rule": view})
		}
		fmt.Printf("收信规则已删除: %s\n", ruleID)
		return nil
	},
}

// buildMailRuleOrder 计算目标顺序：--rule-ids 完整列表（必须包含全部现有规则各一次），
// 或 --move-rule-id + --before-rule-id/--after-rule-id/--to-top/--to-bottom 之一。
func buildMailRuleOrder(cmd *cobra.Command, current []string) ([]string, error) {
	full, _ := cmd.Flags().GetString("rule-ids")
	moveID, _ := cmd.Flags().GetString("move-rule-id")
	before, _ := cmd.Flags().GetString("before-rule-id")
	after, _ := cmd.Flags().GetString("after-rule-id")
	toTop, _ := cmd.Flags().GetBool("to-top")
	toBottom, _ := cmd.Flags().GetBool("to-bottom")
	if strings.TrimSpace(full) != "" {
		if strings.TrimSpace(moveID) != "" {
			return nil, clierr.Usagef("--rule-ids 与 --move-rule-id 只能二选一")
		}
		target := splitAndTrim(full)
		if current != nil {
			want := append([]string(nil), current...)
			got := append([]string(nil), target...)
			sort.Strings(want)
			sort.Strings(got)
			if strings.Join(want, ",") != strings.Join(got, ",") {
				return nil, clierr.Usagef("--rule-ids 必须恰好包含全部现有规则各一次（当前规则: %s）", strings.Join(current, ","))
			}
		}
		return target, nil
	}
	moveID = strings.TrimSpace(moveID)
	if moveID == "" {
		return nil, clierr.Usagef("请指定 --rule-ids，或 --move-rule-id 配合 --before-rule-id/--after-rule-id/--to-top/--to-bottom")
	}
	n := 0
	for _, set := range []bool{before != "", after != "", toTop, toBottom} {
		if set {
			n++
		}
	}
	if n != 1 {
		return nil, clierr.Usagef("--move-rule-id 需要且只能搭配 --before-rule-id / --after-rule-id / --to-top / --to-bottom 之一")
	}
	if current == nil {
		return nil, nil
	}
	idx := -1
	rest := make([]string, 0, len(current))
	for i, id := range current {
		if id == moveID {
			idx = i
			continue
		}
		rest = append(rest, id)
	}
	if idx < 0 {
		return nil, clierr.Usagef("未找到要移动的规则 %s", moveID)
	}
	switch {
	case toTop:
		return append([]string{moveID}, rest...), nil
	case toBottom:
		return append(rest, moveID), nil
	}
	anchor := strings.TrimSpace(before)
	isAfter := false
	if anchor == "" {
		anchor, isAfter = strings.TrimSpace(after), true
	}
	for i, id := range rest {
		if id == anchor {
			pos := i
			if isAfter {
				pos = i + 1
			}
			out := append([]string{}, rest[:pos]...)
			out = append(out, moveID)
			return append(out, rest[pos:]...), nil
		}
	}
	return nil, clierr.Usagef("未找到锚点规则 %s", anchor)
}

var mailRuleReorderCmd = &cobra.Command{
	Use:   "rule-reorder",
	Short: "调整收信规则执行顺序",
	Long: `调整收信规则的执行顺序。

二选一:
  --rule-ids a,b,c       完整目标顺序（必须恰好包含全部现有规则各一次）
  --move-rule-id X       移动单条规则，搭配 --before-rule-id Y / --after-rule-id Y / --to-top / --to-bottom 之一

可选:
  --dry-run              只打印请求；--rule-ids 模式直接给出请求体，移动模式需读取当前顺序后才能计算

示例:
  feishu-cli mail rule-reorder --move-rule-id 701b --to-top --dry-run
  feishu-cli mail rule-reorder --rule-ids 701b,701a,701c`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		// 离线校验 flag 组合
		offline, err := buildMailRuleOrder(cmd, nil)
		if err != nil {
			return err
		}
		if dryRun {
			body := any("<读取当前顺序后计算>")
			if offline != nil {
				body = map[string]any{"rule_ids": offline}
			}
			return printJSON(map[string]any{"dry_run": true, "steps": []map[string]any{
				{"method": "GET", "path": client.MailRulesPath(mailbox), "note": "读取当前顺序并校验"},
				{"method": "POST", "path": client.MailRulesPath(mailbox, "reorder"), "body": body},
			}})
		}
		token, err := requireUserToken(cmd, "mail rule-reorder")
		if err != nil {
			return err
		}
		views, err := listMailRuleViews(mailbox, token)
		if err != nil {
			return err
		}
		current := make([]string, 0, len(views))
		for _, v := range views {
			current = append(current, v.RuleID)
		}
		target, err := buildMailRuleOrder(cmd, current)
		if err != nil {
			return err
		}
		if err := client.ReorderMailRules(mailbox, target, token); err != nil {
			return err
		}
		if output == "json" {
			return printJSON(map[string]any{"before_rule_ids": current, "after_rule_ids": target})
		}
		fmt.Printf("已调整 %d 条收信规则的顺序: %s\n", len(target), strings.Join(target, " → "))
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{mailRuleListCmd, mailRuleGetCmd, mailRuleCreateCmd, mailRuleUpdateCmd, mailRuleEnableCmd, mailRuleDisableCmd, mailRuleDeleteCmd, mailRuleReorderCmd} {
		mailCmd.AddCommand(c)
		addMailRuleCommonFlags(c)
	}
	mailRuleListCmd.Flags().String("name-contains", "", "按规则名称本地过滤")

	mailRuleGetCmd.Flags().String("rule-id", "", "规则 ID（必填）")
	mustMarkFlagRequired(mailRuleGetCmd, "rule-id")

	addMailRuleWriteFlags(mailRuleCreateCmd)
	mailRuleCreateCmd.Flags().Bool("disable", false, "创建为停用状态")

	addMailRuleWriteFlags(mailRuleUpdateCmd)
	mailRuleUpdateCmd.Flags().String("rule-id", "", "规则 ID（必填）")
	mailRuleUpdateCmd.Flags().Bool("enable", false, "启用规则")
	mailRuleUpdateCmd.Flags().Bool("disable", false, "停用规则")
	mailRuleUpdateCmd.Flags().Bool("continue-after-match", false, "命中后继续执行后续规则")
	mustMarkFlagRequired(mailRuleUpdateCmd, "rule-id")

	for _, c := range []*cobra.Command{mailRuleEnableCmd, mailRuleDisableCmd, mailRuleDeleteCmd} {
		c.Flags().String("rule-id", "", "规则 ID（必填）")
		c.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
		mustMarkFlagRequired(c, "rule-id")
	}

	mailRuleReorderCmd.Flags().String("rule-ids", "", "完整目标顺序，逗号分隔")
	mailRuleReorderCmd.Flags().String("move-rule-id", "", "要移动的规则 ID")
	mailRuleReorderCmd.Flags().String("before-rule-id", "", "移动到该规则之前")
	mailRuleReorderCmd.Flags().String("after-rule-id", "", "移动到该规则之后")
	mailRuleReorderCmd.Flags().Bool("to-top", false, "移动到最前")
	mailRuleReorderCmd.Flags().Bool("to-bottom", false, "移动到最后")
	mailRuleReorderCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
}
