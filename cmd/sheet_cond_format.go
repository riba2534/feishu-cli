package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// sheet_cond_format.go —— sheet cond-format list/create/update/delete：公开 v2 condition_formats 接口。

var sheetCondFormatCmd = &cobra.Command{
	Use:   "cond-format",
	Short: "条件格式（列出、创建、更新、删除）",
	Long: `管理电子表格的条件格式（公开 v2 condition_formats 接口）。

限制: 每个子表最多 20 条条件格式；create/update/delete 单次最多 10 条。

规则类型（--rule-type）:
  cellIs             数值比较，需 --operator 与 --formula
                     operator: equal, notEqual, greaterThan, greaterThanOrEqual, lessThan, lessThanOrEqual（1 个 --formula）
                               between, notBetween（2 个 --formula）
  containsText       文本匹配，需 --text；--operator: containsText（默认）, notContains, is, beginsWith, endsWith
  timePeriod         日期，需 --time-period: yesterday, today, tomorrow（last7Days 实测会被存成 tomorrow，暂不开放）
  containsBlanks     为空          notContainsBlanks  不为空
  duplicateValues    重复值        uniqueValues       唯一值（这四种不带参数）

子命令:
  list     列出条件格式
  create   创建条件格式
  update   更新条件格式（整条覆盖）
  delete   删除条件格式（不可撤销，需确认）`,
}

var sheetCondFormatListCmd = &cobra.Command{
	Use:   "list <spreadsheet_token|url> [sheet_id|子表名...]",
	Short: "列出条件格式",
	Long: `列出子表的条件格式。不指定子表时列出全部子表。

示例:
  feishu-cli sheet cond-format list shtcnxxxxxx
  feishu-cli sheet cond-format list shtcnxxxxxx 0b12 Sheet2 -o json`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output := flagString(cmd, "output")
		if err := validateSheetTextJSONOutput(output); err != nil {
			return err
		}
		if _, _, _, err := parseSpreadsheetArg(args[0], ""); err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		var sheetIDs []string
		if len(args) > 1 {
			for _, ref := range args[1:] {
				id, err := target.resolveSheetRef(ref)
				if err != nil {
					return err
				}
				sheetIDs = append(sheetIDs, id)
			}
		} else {
			sheets, err := target.listSheets()
			if err != nil {
				return fmt.Errorf("获取子表列表失败: %w", err)
			}
			for _, s := range sheets {
				sheetIDs = append(sheetIDs, s.SheetID)
			}
		}

		all := []map[string]any{}
		for start := 0; start < len(sheetIDs); start += client.MaxCondFormatsPerRequest {
			end := min(start+client.MaxCondFormatsPerRequest, len(sheetIDs))
			items, err := client.ListCondFormats(client.Context(), target.Token, sheetIDs[start:end], target.UAT)
			if err != nil {
				return err
			}
			all = append(all, items...)
		}

		if output == "json" {
			return printJSON(map[string]any{"sheet_condition_formats": all})
		}
		if len(all) == 0 {
			fmt.Println("没有条件格式")
			return nil
		}
		fmt.Printf("共 %d 条条件格式:\n", len(all))
		for i, item := range all {
			fmt.Printf("  [%d] %s\n", i+1, describeCondFormatItem(item))
		}
		return nil
	},
}

var sheetCondFormatCreateCmd = &cobra.Command{
	Use:   "create <spreadsheet_token|url> <sheet_id|子表名>",
	Short: "创建条件格式",
	Long: `在子表上创建一条条件格式（或用 --data 一次创建最多 10 条）。

范围（--range，可重复或逗号分隔）:
  A1:B10 / A:B / 1:5 / A2:C（到最后一行），不带前缀时自动补上子表；带前缀时必须是同一子表

--formula（cellIs）: 数字原样写；文本自动加双引号（也可自己写成 '"abc"'）。

样式（至少一项，接口要求必须带样式）:
  --bold / --italic / --text-decoration 0|1|2|3（无/下划线/删除线/两者）/ --fore-color / --back-color（#RRGGBB）

--data: condition_format 对象或对象数组的 JSON（字段 ranges/rule_type/attrs/style，与接口一致），
        与规则、样式 flag 互斥；范围缺子表前缀时同样自动补齐。

示例:
  # 数值在 1~10 之间时加粗、绿色背景
  feishu-cli sheet cond-format create shtcnxxxxxx 0b12 --range C2:C100 \
    --rule-type cellIs --operator between --formula 1 --formula 10 --bold --back-color "#e0f6dd"

  # 包含“紧急”时红字
  feishu-cli sheet cond-format create shtcnxxxxxx Sheet1 --range A:A --rule-type containsText --text 紧急 --fore-color "#f54a45"

  # 重复值标黄（先预览请求体）
  feishu-cli sheet cond-format create shtcnxxxxxx 0b12 --range B2:B50 --rule-type duplicateValues --back-color "#fdecd1" --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetCondFormatWrite(cmd, args, false)
	},
}

var sheetCondFormatUpdateCmd = &cobra.Command{
	Use:   "update <spreadsheet_token|url> <sheet_id|子表名> <cf_id>",
	Short: "更新条件格式（整条覆盖）",
	Long: `更新一条条件格式。接口为整条覆盖：范围、规则与样式都要完整给出，未传的样式会被清除。
cf_id 用 sheet cond-format list 查询。flag 含义与 create 相同；--data 只能是单个对象，cf_id 取位置参数。
整条覆盖带确认门禁：交互终端需确认，非交互环境需追加 --yes（否则 exit 10，不执行）；建议先 --dry-run 核对请求体。

示例:
  feishu-cli sheet cond-format update shtcnxxxxxx 0b12 cfxxxxxxxx --range C2:C100 \
    --rule-type cellIs --operator greaterThan --formula 60 --back-color "#e0f6dd" --dry-run
  feishu-cli sheet cond-format update shtcnxxxxxx 0b12 cfxxxxxxxx --range C2:C100 \
    --rule-type cellIs --operator greaterThan --formula 60 --back-color "#e0f6dd" --yes`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetCondFormatWrite(cmd, args, true)
	},
}

var sheetCondFormatDeleteCmd = &cobra.Command{
	Use:   "delete <spreadsheet_token|url> <sheet_id|子表名> <cf_id...>",
	Short: "删除条件格式（不可撤销）",
	Long: `删除子表上的条件格式（单次最多 10 条）。删除不可撤销：交互终端需确认，非交互环境需追加 --yes。

示例:
  feishu-cli sheet cond-format delete shtcnxxxxxx 0b12 cfxxxxxxxx --dry-run
  feishu-cli sheet cond-format delete shtcnxxxxxx 0b12 cfxxxxxxxx cfyyyyyyyy --yes`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		output := flagString(cmd, "output")
		if err := validateSheetTextJSONOutput(output); err != nil {
			return err
		}
		sheetRef := strings.TrimSpace(args[1])
		cfIDs := make([]string, 0, len(args)-2)
		seen := map[string]bool{}
		for _, id := range args[2:] {
			id = strings.TrimSpace(id)
			if id == "" {
				return clierr.Usagef("cf_id 不能为空")
			}
			if !seen[id] {
				seen[id] = true
				cfIDs = append(cfIDs, id)
			}
		}
		if len(cfIDs) > client.MaxCondFormatsPerRequest {
			return clierr.Usagef("单次最多删除 %d 条条件格式（当前 %d 条），请分批执行", client.MaxCondFormatsPerRequest, len(cfIDs))
		}
		if sheetRef == "" {
			return clierr.Usagef("子表 ID / 名称不能为空")
		}
		token, _, _, err := parseSpreadsheetArg(args[0], "")
		if err != nil {
			return err
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, fmt.Sprintf("删除 %d 条条件格式（不可撤销）", len(cfIDs)), map[string]any{
				"spreadsheet_token": token,
				"sheet":             sheetRef,
				"irreversible":      true,
			}, []dryRunStep{{
				Method: "DELETE",
				URL:    client.CondFormatPath(token, "batch_delete"),
				Desc:   "子表名会在执行时换算为 sheetId",
				Body:   map[string]any{"sheet_cf_ids": sheetCfIDs(sheetRef, cfIDs)},
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		sheetID, err := target.resolveSheetRef(sheetRef)
		if err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确认删除子表 %s 上的 %d 条条件格式（%s）？此操作不可撤销",
			sheetID, len(cfIDs), strings.Join(cfIDs, ", "))); err != nil {
			return err
		}
		results, err := client.BatchDeleteCondFormats(client.Context(), target.Token, sheetCfIDs(sheetID, cfIDs), target.UAT)
		if err != nil {
			if output == "json" && len(results) > 0 {
				_ = printJSON(map[string]any{"responses": results})
			}
			return err
		}
		if output == "json" {
			return printJSON(map[string]any{"responses": results})
		}
		fmt.Printf("删除成功！共删除 %d 条条件格式\n", len(results))
		return nil
	},
}

func sheetCfIDs(sheetID string, cfIDs []string) []client.SheetCfID {
	items := make([]client.SheetCfID, len(cfIDs))
	for i, id := range cfIDs {
		items[i] = client.SheetCfID{SheetID: sheetID, CfID: id}
	}
	return items
}

func validateSheetTextJSONOutput(output string) error {
	if output != "" && output != "text" && output != "json" {
		return clierr.Usagef("不支持的输出格式 %q，仅支持 text, json", output)
	}
	return nil
}

// condFormatRuleTypes 规则类型（规范写法），键为小写便于大小写不敏感匹配。
var condFormatRuleTypes = map[string]string{
	"cellis":            "cellIs",
	"containstext":      "containsText",
	"timeperiod":        "timePeriod",
	"containsblanks":    "containsBlanks",
	"notcontainsblanks": "notContainsBlanks",
	"duplicatevalues":   "duplicateValues",
	"uniquevalues":      "uniqueValues",
}

var condFormatCellIsOperators = []string{"equal", "notEqual", "greaterThan", "greaterThanOrEqual", "lessThan", "lessThanOrEqual", "between", "notBetween"}
var condFormatTextOperators = []string{"containsText", "notContains", "is", "beginsWith", "endsWith"}

// 公开文档还列出 last7Days，但实测服务端返回成功、读回却是 tomorrow（规则被静默改写），因此 flag 模式不开放。
var condFormatTimePeriods = []string{"yesterday", "today", "tomorrow"}

var condFormatHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// canonicalChoice 大小写不敏感地匹配候选值，返回规范写法。
func canonicalChoice(v string, choices []string) (string, bool) {
	for _, c := range choices {
		if strings.EqualFold(strings.TrimSpace(v), c) {
			return c, true
		}
	}
	return "", false
}

// condFormatRuleFlags 是与 --data 互斥的规则与样式 flag。
var condFormatRuleFlags = []string{"rule-type", "operator", "formula", "text", "time-period", "bold", "italic", "text-decoration", "fore-color", "back-color"}

// runSheetCondFormatWrite 是 create / update 的公共实现：本地校验 → dry-run → 身份与表格 → 执行。
func runSheetCondFormatWrite(cmd *cobra.Command, args []string, isUpdate bool) error {
	output := flagString(cmd, "output")
	if err := validateSheetTextJSONOutput(output); err != nil {
		return err
	}
	sheetRef := strings.TrimSpace(args[1])
	if sheetRef == "" {
		return clierr.Usagef("子表 ID / 名称不能为空")
	}
	cfID := ""
	if isUpdate {
		cfID = strings.TrimSpace(args[2])
		if cfID == "" {
			return clierr.Usagef("cf_id 不能为空")
		}
	}
	formats, err := condFormatsFromCmd(cmd, isUpdate)
	if err != nil {
		return err
	}
	for i := range formats {
		if isUpdate {
			formats[i].CfID = cfID
		}
		ranges, err := qualifyCondFormatRanges(formats[i].Ranges, sheetRef)
		if err != nil {
			return err
		}
		formats[i].Ranges = ranges
	}
	token, _, _, err := parseSpreadsheetArg(args[0], "")
	if err != nil {
		return err
	}

	action, label := "batch_create", "创建"
	if isUpdate {
		action, label = "batch_update", "更新"
	}
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		return printDryRunPlan(cmd, fmt.Sprintf("%s %d 条条件格式", label, len(formats)), map[string]any{
			"spreadsheet_token": token,
			"sheet":             sheetRef,
		}, []dryRunStep{{
			Method: "POST",
			URL:    client.CondFormatPath(token, action),
			Desc:   "子表名会在执行时换算为 sheetId",
			Body:   map[string]any{"sheet_condition_formats": sheetCondFormats(sheetRef, formats)},
		}})
	}

	if err := config.Validate(); err != nil {
		return err
	}
	target, err := newSheetTarget(cmd, args[0])
	if err != nil {
		return err
	}
	sheetID, err := target.resolveSheetRef(sheetRef)
	if err != nil {
		return err
	}
	for i := range formats {
		ranges, err := resolveCondFormatRangePrefixes(target, formats[i].Ranges, sheetRef, sheetID)
		if err != nil {
			return err
		}
		formats[i].Ranges = ranges
	}
	items := sheetCondFormats(sheetID, formats)
	if isUpdate {
		// 更新为整条覆盖（未传的样式会被清除），与删除同样走确认门禁
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确认整条覆盖子表 %s 上的 %d 条条件格式？未传的样式会被清除", sheetID, len(formats))); err != nil {
			return err
		}
	}
	var results []client.CondFormatResult
	if isUpdate {
		results, err = client.BatchUpdateCondFormats(client.Context(), target.Token, items, target.UAT)
	} else {
		results, err = client.BatchCreateCondFormats(client.Context(), target.Token, items, target.UAT)
	}
	if err != nil {
		if output == "json" && len(results) > 0 {
			_ = printJSON(map[string]any{"responses": results})
		}
		return err
	}
	if output == "json" {
		return printJSON(map[string]any{"responses": results})
	}
	fmt.Printf("%s成功！共 %d 条条件格式\n", label, len(results))
	for _, r := range results {
		fmt.Printf("  子表: %s  cf_id: %s\n", r.SheetID, r.CfID)
	}
	return nil
}

func sheetCondFormats(sheetID string, formats []client.CondFormat) []client.SheetCondFormat {
	items := make([]client.SheetCondFormat, len(formats))
	for i, f := range formats {
		items[i] = client.SheetCondFormat{SheetID: sheetID, ConditionFormat: f}
	}
	return items
}

// condFormatsFromCmd 从 --data 或规则/样式 flag 构造条件格式，全部离线校验。
func condFormatsFromCmd(cmd *cobra.Command, isUpdate bool) ([]client.CondFormat, error) {
	rangesRaw, _ := cmd.Flags().GetStringArray("range")
	var ranges []string
	for _, r := range rangesRaw {
		ranges = append(ranges, splitAndTrim(r)...)
	}

	dataStr := flagString(cmd, "data")
	dataFile := flagString(cmd, "data-file")
	if dataStr != "" || dataFile != "" {
		if dataStr != "" && dataFile != "" {
			return nil, clierr.Usagef("--data 与 --data-file 不能同时使用")
		}
		for _, name := range condFormatRuleFlags {
			if cmd.Flags().Changed(name) {
				return nil, clierr.Usagef("--data/--data-file 与 --%s 不能同时使用", name)
			}
		}
		raw, err := readSheetDataInput(cmd)
		if err != nil {
			return nil, err
		}
		formats, err := parseCondFormatData(raw)
		if err != nil {
			return nil, err
		}
		if isUpdate && len(formats) != 1 {
			return nil, clierr.Usagef("update 的 --data 只能是单个条件格式对象（当前 %d 条）", len(formats))
		}
		for i := range formats {
			if len(ranges) > 0 {
				formats[i].Ranges = append([]string(nil), ranges...)
			}
			if len(formats[i].Ranges) == 0 {
				return nil, clierr.Usagef("--data 第 %d 条缺少 ranges（也可用 --range 统一指定）", i+1)
			}
			if strings.TrimSpace(formats[i].RuleType) == "" {
				return nil, clierr.Usagef("--data 第 %d 条缺少 rule_type", i+1)
			}
			if condFormatStyleEmpty(formats[i].Style) {
				return nil, clierr.Usagef("--data 第 %d 条缺少 style（接口要求至少设置一项样式：font.bold/italic、text_decoration、fore_color、back_color）", i+1)
			}
		}
		return formats, nil
	}

	if len(ranges) == 0 {
		return nil, clierr.Usagef("--range 为必填项（如 A1:B10，可重复或逗号分隔）")
	}
	f, err := condFormatFromFlags(cmd)
	if err != nil {
		return nil, err
	}
	f.Ranges = ranges
	return []client.CondFormat{f}, nil
}

// parseCondFormatData 解析 --data：单个 condition_format 对象或对象数组（≤10），未知字段报错。
func parseCondFormatData(raw []byte) ([]client.CondFormat, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, clierr.Usagef("--data 不能为空")
	}
	if trimmed[0] != '[' {
		trimmed = append(append([]byte{'['}, trimmed...), ']')
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var formats []client.CondFormat
	if err := dec.Decode(&formats); err != nil {
		return nil, clierr.Usagef("--data 必须是条件格式对象或对象数组（字段 ranges/rule_type/attrs/style）: %v", err)
	}
	if len(formats) == 0 || len(formats) > client.MaxCondFormatsPerRequest {
		return nil, clierr.Usagef("--data 条件格式数量必须为 1-%d（当前 %d）", client.MaxCondFormatsPerRequest, len(formats))
	}
	return formats, nil
}

// condFormatFromFlags 由 --rule-type 等 flag 构造单条条件格式（不含范围）。
func condFormatFromFlags(cmd *cobra.Command) (client.CondFormat, error) {
	var f client.CondFormat
	ruleRaw := strings.TrimSpace(flagString(cmd, "rule-type"))
	if ruleRaw == "" {
		return f, clierr.Usagef("--rule-type 为必填项（cellIs, containsText, timePeriod, containsBlanks, notContainsBlanks, duplicateValues, uniqueValues）")
	}
	rule, ok := condFormatRuleTypes[strings.ToLower(ruleRaw)]
	if !ok {
		return f, clierr.Usagef("--rule-type 不支持 %q，可选: cellIs, containsText, timePeriod, containsBlanks, notContainsBlanks, duplicateValues, uniqueValues", ruleRaw)
	}
	f.RuleType = rule

	operator := strings.TrimSpace(flagString(cmd, "operator"))
	formulas, _ := cmd.Flags().GetStringArray("formula")
	text := flagString(cmd, "text")
	timePeriod := strings.TrimSpace(flagString(cmd, "time-period"))
	setOperator, setFormula, setText, setPeriod := cmd.Flags().Changed("operator"), cmd.Flags().Changed("formula"), cmd.Flags().Changed("text"), cmd.Flags().Changed("time-period")
	rejectExtra := func(names ...string) error {
		changed := map[string]bool{"operator": setOperator, "formula": setFormula, "text": setText, "time-period": setPeriod}
		for _, n := range names {
			if changed[n] {
				return clierr.Usagef("--rule-type %s 不使用 --%s", rule, n)
			}
		}
		return nil
	}

	switch rule {
	case "cellIs":
		if err := rejectExtra("text", "time-period"); err != nil {
			return f, err
		}
		op, ok := canonicalChoice(operator, condFormatCellIsOperators)
		if !ok {
			return f, clierr.Usagef("--rule-type cellIs 需要 --operator，可选: %s", strings.Join(condFormatCellIsOperators, ", "))
		}
		want := 1
		if op == "between" || op == "notBetween" {
			want = 2
		}
		if len(formulas) != want {
			return f, clierr.Usagef("--operator %s 需要 %d 个 --formula（当前 %d 个）", op, want, len(formulas))
		}
		values := make([]string, len(formulas))
		for i, v := range formulas {
			if strings.TrimSpace(v) == "" {
				return f, clierr.Usagef("--formula 不能为空")
			}
			values[i] = condFormatFormulaValue(v)
		}
		f.Attrs = []client.CondFormatAttr{{Operator: op, Formula: values}}
	case "containsText":
		if err := rejectExtra("formula", "time-period"); err != nil {
			return f, err
		}
		if text == "" {
			return f, clierr.Usagef("--rule-type containsText 需要 --text")
		}
		op := "containsText"
		if setOperator {
			var ok bool
			if op, ok = canonicalChoice(operator, condFormatTextOperators); !ok {
				return f, clierr.Usagef("--rule-type containsText 的 --operator 可选: %s", strings.Join(condFormatTextOperators, ", "))
			}
		}
		f.Attrs = []client.CondFormatAttr{{Operator: op, Text: text}}
	case "timePeriod":
		if err := rejectExtra("formula", "text"); err != nil {
			return f, err
		}
		if setOperator && !strings.EqualFold(operator, "is") {
			return f, clierr.Usagef("--rule-type timePeriod 的 --operator 只支持 is")
		}
		if strings.EqualFold(timePeriod, "last7Days") {
			return f, clierr.Usagef("--time-period last7Days 暂不支持：实测接口返回成功但规则被存成 tomorrow；可选: %s", strings.Join(condFormatTimePeriods, ", "))
		}
		period, ok := canonicalChoice(timePeriod, condFormatTimePeriods)
		if !ok {
			return f, clierr.Usagef("--rule-type timePeriod 需要 --time-period，可选: %s", strings.Join(condFormatTimePeriods, ", "))
		}
		f.Attrs = []client.CondFormatAttr{{Operator: "is", TimePeriod: period}}
	default:
		if err := rejectExtra("operator", "formula", "text", "time-period"); err != nil {
			return f, err
		}
	}

	style, err := condFormatStyleFromFlags(cmd)
	if err != nil {
		return f, err
	}
	if style == nil {
		return f, clierr.Usagef(condFormatStyleRequired)
	}
	f.Style = style
	return f, nil
}

// condFormatFormulaValue 规整 cellIs 的比较值：数字、已带引号或以 = 开头的原样保留，其余按文本加双引号。
func condFormatFormulaValue(v string) string {
	t := strings.TrimSpace(v)
	if isNumericString(t) || strings.HasPrefix(t, "=") || (len(t) >= 2 && strings.HasPrefix(t, `"`) && strings.HasSuffix(t, `"`)) {
		return t
	}
	return `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
}

func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	var n json.Number
	return json.Unmarshal([]byte(s), &n) == nil
}

// condFormatStyleRequired：实测接口要求 style 必填且非空（缺失 9499、空对象 res_code=90232 style is empty）。
const condFormatStyleRequired = "至少需要一个样式 flag（--bold、--italic、--text-decoration、--fore-color、--back-color）；接口要求条件格式必须带样式"

func condFormatStyleEmpty(st *client.CondFormatStyle) bool {
	return st == nil || (st.Font == nil && st.TextDecoration == nil && st.ForeColor == "" && st.BackColor == "")
}

func condFormatStyleFromFlags(cmd *cobra.Command) (*client.CondFormatStyle, error) {
	style := &client.CondFormatStyle{}
	used := false
	if cmd.Flags().Changed("bold") || cmd.Flags().Changed("italic") {
		font := &client.CondFormatFont{}
		if cmd.Flags().Changed("bold") {
			b, _ := cmd.Flags().GetBool("bold")
			font.Bold = &b
		}
		if cmd.Flags().Changed("italic") {
			b, _ := cmd.Flags().GetBool("italic")
			font.Italic = &b
		}
		style.Font = font
		used = true
	}
	if cmd.Flags().Changed("text-decoration") {
		td, _ := cmd.Flags().GetInt("text-decoration")
		if td < 0 || td > 3 {
			return nil, clierr.Usagef("--text-decoration 只支持 0（无）/1（下划线）/2（删除线）/3（两者），得到 %d", td)
		}
		style.TextDecoration = &td
		used = true
	}
	for _, c := range []struct {
		flag string
		dst  *string
	}{{"fore-color", &style.ForeColor}, {"back-color", &style.BackColor}} {
		if !cmd.Flags().Changed(c.flag) {
			continue
		}
		v := strings.TrimSpace(flagString(cmd, c.flag))
		if !condFormatHexColor.MatchString(v) {
			return nil, clierr.Usagef("--%s 必须是 #RRGGBB 十六进制颜色（如 #e0f6dd），得到 %q", c.flag, v)
		}
		*c.dst = strings.ToLower(v)
		used = true
	}
	if !used {
		return nil, nil
	}
	return style, nil
}

// qualifyCondFormatRanges 离线规整范围：不带前缀的 A1 写法补上 sheetRef；带前缀的保留（执行时校验同一子表）。
func qualifyCondFormatRanges(ranges []string, sheetRef string) ([]string, error) {
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		r = strings.TrimSpace(unescapeSheetRange(r))
		if r == "" {
			return nil, clierr.Usagef("范围不能为空")
		}
		if prefix, rest, ok := client.SplitSheetRangePrefix(r); ok {
			if !condFormatA1(rest) {
				return nil, clierr.Usagef("范围 %q 不是有效的 A1 写法（如 A1:B10、A:B、1:5、A2:C）", r)
			}
			out = append(out, prefix+"!"+rest)
			continue
		}
		if !condFormatA1(r) {
			return nil, clierr.Usagef("范围 %q 不是有效的 A1 写法（如 A1:B10、A:B、1:5、A2:C）", r)
		}
		out = append(out, sheetRef+"!"+r)
	}
	return out, nil
}

// condFormatA1 接受常规 A1 写法，另外接受条件格式特有的「起始单元格:结束列」（A2:C，到最后一行）。
func condFormatA1(s string) bool {
	if client.LooksLikeA1Range(s) {
		return true
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return false
	}
	start, ok1 := client.ParseA1Bounds(parts[0])
	end, ok2 := client.ParseA1Bounds(parts[1])
	return ok1 && ok2 && start.StartCol >= 0 && start.StartRow >= 0 && end.StartCol >= 0 && end.StartRow < 0
}

// resolveCondFormatRangePrefixes 把范围前缀（sheetRef 或其他子表名/ID）换成 sheetID，不同子表时报用法错误。
func resolveCondFormatRangePrefixes(target *sheetTarget, ranges []string, sheetRef, sheetID string) ([]string, error) {
	out := make([]string, len(ranges))
	for i, r := range ranges {
		prefix, rest, _ := client.SplitSheetRangePrefix(r)
		id := sheetID
		if prefix != sheetRef && prefix != sheetID {
			var err error
			if id, err = target.resolveSheetRef(prefix); err != nil {
				return nil, err
			}
		}
		if id != sheetID {
			return nil, clierr.Usagef("范围 %q 不属于子表 %s；条件格式的范围必须在同一子表内", r, sheetID)
		}
		out[i] = sheetID + "!" + rest
	}
	return out, nil
}

// describeCondFormatItem 渲染列表接口的一项为单行文本。
func describeCondFormatItem(item map[string]any) string {
	cf, _ := item["condition_format"].(map[string]any)
	sheetID, _ := item["sheet_id"].(string)
	if cf == nil {
		return fmt.Sprintf("子表 %s: %v", sheetID, item)
	}
	cfID, _ := cf["cf_id"].(string)
	rule, _ := cf["rule_type"].(string)
	var ranges []string
	if rs, ok := cf["ranges"].([]any); ok {
		for _, r := range rs {
			ranges = append(ranges, fmt.Sprint(r))
		}
	}
	parts := []string{fmt.Sprintf("cf_id=%s", cfID), fmt.Sprintf("子表=%s", sheetID), fmt.Sprintf("规则=%s", rule),
		fmt.Sprintf("范围=%s", strings.Join(ranges, ","))}
	if attrs, ok := cf["attrs"].([]any); ok && len(attrs) > 0 {
		b, _ := json.Marshal(attrs)
		parts = append(parts, "参数="+string(b))
	}
	if style, ok := cf["style"].(map[string]any); ok && len(style) > 0 {
		b, _ := json.Marshal(style)
		parts = append(parts, "样式="+string(b))
	}
	return strings.Join(parts, "  ")
}

func init() {
	sheetCmd.AddCommand(sheetCondFormatCmd)
	sheetCondFormatCmd.AddCommand(sheetCondFormatListCmd, sheetCondFormatCreateCmd, sheetCondFormatUpdateCmd, sheetCondFormatDeleteCmd)

	sheetCondFormatListCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetCondFormatListCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	for _, c := range []*cobra.Command{sheetCondFormatCreateCmd, sheetCondFormatUpdateCmd} {
		c.Flags().StringArray("range", nil, "应用范围（可重复或逗号分隔；不带子表前缀时自动补齐）")
		c.Flags().String("rule-type", "", "规则类型: cellIs, containsText, timePeriod, containsBlanks, notContainsBlanks, duplicateValues, uniqueValues")
		c.Flags().String("operator", "", "比较方式（cellIs / containsText 使用，见命令说明）")
		c.Flags().StringArray("formula", nil, "cellIs 比较值（可重复；between/notBetween 需 2 个）")
		c.Flags().String("text", "", "containsText 匹配文本")
		c.Flags().String("time-period", "", "timePeriod 日期范围: yesterday, today, tomorrow")
		c.Flags().Bool("bold", false, "命中时加粗")
		c.Flags().Bool("italic", false, "命中时斜体")
		c.Flags().Int("text-decoration", 0, "命中时文字装饰: 0 无, 1 下划线, 2 删除线, 3 两者")
		c.Flags().String("fore-color", "", "命中时文字颜色（#RRGGBB）")
		c.Flags().String("back-color", "", "命中时背景颜色（#RRGGBB）")
		c.Flags().StringP("data", "d", "", "条件格式 JSON（condition_format 对象或数组），与规则/样式 flag 互斥")
		c.Flags().String("data-file", "", "从文件读取 --data 内容")
		c.Flags().Bool("dry-run", false, "只打印将要发送的请求，不执行")
		c.Flags().StringP("output", "o", "", "输出格式: text（默认）, json；--dry-run 默认输出 JSON 预览")
		c.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	}

	sheetCondFormatDeleteCmd.Flags().Bool("dry-run", false, "只打印将要发送的删除请求，不执行")
	sheetCondFormatDeleteCmd.Flags().StringP("output", "o", "", "输出格式: text（默认）, json；--dry-run 默认输出 JSON 预览")
	sheetCondFormatDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
