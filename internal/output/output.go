// Package output 提供与 lark-cli 对齐的统一输出工程化能力：
//
//	--format json|pretty|table|ndjson|csv   输出格式
//	--jq <expr>                             内置 gojq 过滤（无需外部 jq）
//	-o/--output <path>                       写入文件而非 stdout
//	--dry-run                                只打印将发送的请求（写命令预览）
//	--page-all/--page-size/--page-limit      自动翻页（由命令的拉取循环消费）
//
// 新命令（api / 增强后的 search / bitable 新增项）统一通过本包出口，
// 旧命令保持原样不强制改造，避免大范围回归。
package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// 输出格式枚举。
const (
	FormatJSON   = "json"   // 缩进 JSON（默认）
	FormatPretty = "pretty" // 人类可读缩进 JSON（当前等价 json，保留语义位）
	FormatTable  = "table"  // 对象数组 → 对齐表格
	FormatNDJSON = "ndjson" // 每行一个紧凑 JSON（数组按元素拆行）
	FormatCSV    = "csv"    // 对象数组 → CSV
)

// validFormats 列出所有合法 --format 取值，用于校验与错误提示。
var validFormats = []string{FormatJSON, FormatPretty, FormatTable, FormatNDJSON, FormatCSV}

// Options 汇总输出/分页相关选项。Format/JQ/OutputFile 由 Render 消费；
// 分页字段由各命令自己的拉取循环消费（本包不发请求）。
type Options struct {
	Format     string
	JQ         string
	OutputFile string
	DryRun     bool

	PageAll   bool
	PageSize  int
	PageLimit int
}

// AddOutputFlags 给命令注册 --format/--jq/-o。
// 若命令已存在 output flag（部分旧命令自带 -o），则不重复注册，避免 panic。
func AddOutputFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("format") == nil {
		f.String("format", FormatJSON, "输出格式: json | pretty | table | ndjson | csv")
	}
	if f.Lookup("jq") == nil {
		f.String("jq", "", "用 jq 表达式过滤 JSON 输出（内置 gojq，无需外部 jq）")
	}
	if f.Lookup("output") == nil {
		f.StringP("output", "o", "", "输出写入文件（默认 stdout）")
	}
}

// AddFormatFlags 仅注册 --format/--jq（不含 -o），用于已自带 -o/--output 语义的旧命令改造，
// 避免 -o 文件路径语义与旧命令的 -o 冲突。
func AddFormatFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("format") == nil {
		f.String("format", FormatJSON, "输出格式: json | pretty | table | ndjson | csv")
	}
	if f.Lookup("jq") == nil {
		f.String("jq", "", "用 jq 表达式过滤 JSON 输出（内置 gojq，无需外部 jq）")
	}
}

// NewOptions 构造并校验 Options（供旧命令手工构建，不经 flag 解析）。
func NewOptions(format, jq string) (*Options, error) {
	if format == "" {
		format = FormatJSON
	}
	if !isValidFormat(format) {
		return nil, fmt.Errorf("不支持的 --format %q，可选值: %s", format, strings.Join(validFormats, ", "))
	}
	return &Options{Format: format, JQ: jq}, nil
}

// AddPaginationFlags 给命令注册 --page-all/--page-size/--page-limit。
func AddPaginationFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("page-all") == nil {
		f.Bool("page-all", false, "自动翻页拉取全部结果")
	}
	if f.Lookup("page-size") == nil {
		f.Int("page-size", 0, "单页数量（0=使用 API 默认）")
	}
	if f.Lookup("page-limit") == nil {
		f.Int("page-limit", 0, "自动翻页最大页数（0=不限；配合 --page-all 防失控）")
	}
}

// AddDryRunFlag 给写命令注册 --dry-run。
func AddDryRunFlag(cmd *cobra.Command) {
	if cmd.Flags().Lookup("dry-run") == nil {
		cmd.Flags().Bool("dry-run", false, "只打印将发送的请求，不实际调用")
	}
}

// ParseOptions 从 cmd 读取所有已注册的输出/分页 flag。
// 未注册的 flag 取零值（例如某命令没接分页 flag 时 PageAll=false）。
func ParseOptions(cmd *cobra.Command) (*Options, error) {
	o := &Options{Format: FormatJSON}
	f := cmd.Flags()
	if f.Lookup("format") != nil {
		o.Format, _ = f.GetString("format")
	}
	if f.Lookup("jq") != nil {
		o.JQ, _ = f.GetString("jq")
	}
	if f.Lookup("output") != nil {
		o.OutputFile, _ = f.GetString("output")
	}
	if f.Lookup("dry-run") != nil {
		o.DryRun, _ = f.GetBool("dry-run")
	}
	if f.Lookup("page-all") != nil {
		o.PageAll, _ = f.GetBool("page-all")
	}
	if f.Lookup("page-size") != nil {
		o.PageSize, _ = f.GetInt("page-size")
	}
	if f.Lookup("page-limit") != nil {
		o.PageLimit, _ = f.GetInt("page-limit")
	}
	if o.Format == "" {
		o.Format = FormatJSON
	}
	if !isValidFormat(o.Format) {
		return nil, fmt.Errorf("不支持的 --format %q，可选值: %s", o.Format, strings.Join(validFormats, ", "))
	}
	return o, nil
}

func isValidFormat(f string) bool {
	for _, v := range validFormats {
		if v == f {
			return true
		}
	}
	return false
}

// Render 把 data 按 Options 渲染并写到 stdout 或 -o 文件。
// 流程：normalize（JSON round-trip 成纯 map/slice）→ 可选 jq 过滤 → 按 format 编码。
func Render(o *Options, data any) error {
	text, err := RenderString(o, data)
	if err != nil {
		return err
	}
	return writeOut(o.OutputFile, text)
}

// RenderString 与 Render 同逻辑但返回字符串，便于测试与复用。
func RenderString(o *Options, data any) (string, error) {
	norm, err := normalize(data)
	if err != nil {
		return "", err
	}

	results := []any{norm}
	if strings.TrimSpace(o.JQ) != "" {
		results, err = applyJQ(o.JQ, norm)
		if err != nil {
			return "", err
		}
	}

	switch o.Format {
	case FormatJSON, FormatPretty:
		return formatJSONResults(results)
	case FormatNDJSON:
		return formatNDJSON(results)
	case FormatTable:
		return formatTable(results)
	case FormatCSV:
		return formatCSV(results)
	default:
		return "", fmt.Errorf("不支持的 --format %q", o.Format)
	}
}

// normalize 把任意 Go 值通过 JSON round-trip 转成 map[string]any / []any / 标量，
// 使 gojq 与表格/CSV 渲染面对统一形态。数字保留为 json.Number 以免大整数精度丢失。
func normalize(data any) (any, error) {
	if data == nil {
		return nil, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("序列化失败: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("反序列化失败: %w", err)
	}
	return out, nil
}

// applyJQ 用 gojq 对 input 求值，返回所有输出结果。
// 先经 toJQInput 把 json.Number 转成 gojq 精确数字类型（保大整数精度）。
func applyJQ(expr string, input any) ([]any, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("jq 表达式解析失败: %w", err)
	}
	jqInput, err := toJQInput(input)
	if err != nil {
		return nil, err
	}
	iter := query.Run(jqInput)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, isErr := v.(error); isErr {
			if e == nil {
				continue
			}
			return nil, fmt.Errorf("jq 求值失败: %w", e)
		}
		results = append(results, v)
	}
	return results, nil
}

// toJQInput 把含 json.Number 的归一化值转成 gojq 可精确处理的类型。
// gojq 接受 int / *big.Int / float64：整数走 int（飞书 19 位 message_id/chat_id 仍在 int64 内），
// 超 int64 走 *big.Int，小数走 float64。
// 不能像旧实现那样 json.Marshal+Unmarshal（无 UseNumber）round-trip——那会把所有数字降级成
// float64，19 位大整数被截断（如 7030776512726958083 → 7030776512726958080）。
func toJQInput(input any) (any, error) {
	return convertJSONNumbers(input), nil
}

// convertJSONNumbers 递归把 json.Number 转成 gojq 友好的精确数字类型。
func convertJSONNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = convertJSONNumbers(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = convertJSONNumbers(val)
		}
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		if bi, ok := new(big.Int).SetString(t.String(), 10); ok {
			return bi
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	default:
		return v
	}
}

// formatJSONResults 把结果列表编码为缩进 JSON。
// 单结果直接编码；多结果（jq 产生多个输出）逐个编码并以换行分隔，与 jq CLI 行为一致。
func formatJSONResults(results []any) (string, error) {
	var parts []string
	for _, r := range results {
		s, err := encodeJSONIndent(r)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n") + "\n", nil
}

func encodeJSONIndent(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("JSON 编码失败: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func encodeJSONCompact(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("JSON 编码失败: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// formatNDJSON 每行输出一个紧凑 JSON。
// 若某结果本身是数组，则按元素逐行展开（便于 jq '.items' 后直接管道）。
func formatNDJSON(results []any) (string, error) {
	var lines []string
	for _, r := range results {
		if arr, ok := r.([]any); ok {
			for _, e := range arr {
				s, err := encodeJSONCompact(e)
				if err != nil {
					return "", err
				}
				lines = append(lines, s)
			}
			continue
		}
		s, err := encodeJSONCompact(r)
		if err != nil {
			return "", err
		}
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// rowsFromResults 把结果列表抽取成扁平对象行集合（用于 table/csv）。
// 支持三种形态：单个对象数组、多个对象（jq 多输出）、单个对象。
func rowsFromResults(results []any) ([]map[string]any, error) {
	var rows []map[string]any
	add := func(v any) error {
		if v == nil {
			// 顶层/元素 nil（如 API 返回 data:null）视作空，与空数组一致优雅处理，不报错
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("table/csv 需要对象或对象数组，遇到 %T", v)
		}
		rows = append(rows, m)
		return nil
	}
	for _, r := range results {
		switch t := r.(type) {
		case []any:
			for _, e := range t {
				if err := add(e); err != nil {
					return nil, err
				}
			}
		default:
			if err := add(r); err != nil {
				return nil, err
			}
		}
	}
	return rows, nil
}

// collectColumns 返回所有行键的并集，保持稳定顺序（首次出现序优先，其余按字典序补齐）。
func collectColumns(rows []map[string]any) []string {
	seen := map[string]bool{}
	var cols []string
	for _, row := range rows {
		var keys []string
		for k := range row {
			if !seen[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			seen[k] = true
			cols = append(cols, k)
		}
	}
	return cols
}

// cellString 把单元格值转成字符串：标量直出，复合类型回退紧凑 JSON。
func cellString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		s, err := encodeJSONCompact(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return s
	}
}

func formatTable(results []any) (string, error) {
	rows, err := rowsFromResults(results)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "(空结果)\n", nil
	}
	cols := collectColumns(rows)
	// 表头也做单行化，避免列名含换行/制表符破坏对齐
	headers := make([]string, len(cols))
	widths := make([]int, len(cols))
	for i, c := range cols {
		headers[i] = sanitizeTableCell(c)
		widths[i] = displayWidth(headers[i])
	}
	cells := make([][]string, len(rows))
	for ri, row := range rows {
		cells[ri] = make([]string, len(cols))
		for ci, c := range cols {
			// 单元格 \n/\r/\t 替换为空格，否则表格错行（CSV 路径不需要，csv 包自带引号转义）
			s := sanitizeTableCell(cellString(row[c]))
			cells[ri][ci] = s
			if w := displayWidth(s); w > widths[ci] {
				widths[ci] = w
			}
		}
	}
	var b strings.Builder
	writeRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(v)
			b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(v)))
		}
		b.WriteString("\n")
	}
	writeRow(headers)
	sep := make([]string, len(cols))
	for i := range cols {
		sep[i] = strings.Repeat("-", widths[i])
	}
	writeRow(sep)
	for _, row := range cells {
		writeRow(row)
	}
	return b.String(), nil
}

// sanitizeTableCell 把换行/回车/制表符替换为空格，使表格单元格保持单行不错位。
func sanitizeTableCell(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
}

// displayWidth 估算字符串显示宽度：CJK/全角字符按 2 列计，其余按 1 列。
// 仅用于表格对齐的近似，不追求完全精确。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0xA4CF) || // CJK 部首 .. Yi
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul 音节
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 兼容
		(r >= 0xFE30 && r <= 0xFE4F) || // CJK 兼容形式
		(r >= 0xFF00 && r <= 0xFF60) || // 全角 ASCII
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1FAFF) // emoji 区段（近似）
}

func formatCSV(results []any) (string, error) {
	rows, err := rowsFromResults(results)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if len(rows) == 0 {
		w.Flush()
		return buf.String(), nil
	}
	cols := collectColumns(rows)
	if err := w.Write(cols); err != nil {
		return "", fmt.Errorf("CSV 写表头失败: %w", err)
	}
	for _, row := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = cellString(row[c])
		}
		if err := w.Write(rec); err != nil {
			return "", fmt.Errorf("CSV 写行失败: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("CSV 刷新失败: %w", err)
	}
	return buf.String(), nil
}

// writeOut 写到文件或 stdout。
func writeOut(path, text string) error {
	if path == "" {
		fmt.Print(text)
		return nil
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "已写入 %s\n", path)
	return nil
}
