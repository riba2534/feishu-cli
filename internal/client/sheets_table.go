package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 本文件实现 sheet table-put 的类型保真核心：把 pandas DataFrame 形状的 JSON
// （string columns + dtypes/formats 映射）按列 dtype 映射为 V3 typed cell。
//
// V3 /values/batch_update 的单元格元素契约（经真表 round-trip 实测）：
//   - 文本/布尔走 {"type":"text","text":{"text":...}}；字符串塞进 value 元素会被
//     服务端拒收（code=1310251 value invalid）。
//   - 数字/日期走 {"type":"value","value":{"value":"<数字串>"}}；日期写 Excel 序列号。
//   - 空单元格必须写空文本元素 {"type":"text","text":{"text":""}}；写空元素数组 []
//     会让整批写入 500（code=1315201）。
//   - 单元格元素本身【不支持】cell_styles/number_format（V3 模型无此字段，静默丢弃）。
//     日期列要成「真日期」（可排序/可透视），需写完序列号后再用 V2 style 接口给该
//     区域设 formatter（如 yyyy/MM/dd）—— 序列号 45306 + 日期 formatter 实测渲染为
//     2024/01/15。formatter 由命令层根据各列 Format 统一施加。
// 纯函数化便于单测（无需网络）。

// TableColType 是 table-put 列的内部类型。
type TableColType string

const (
	TableColTypeString TableColType = "string"
	TableColTypeNumber TableColType = "number"
	TableColTypeBool   TableColType = "bool"
	TableColTypeDate   TableColType = "date"
)

// TableColSpec normalize 后的列规格。
type TableColSpec struct {
	Name   string
	Type   TableColType
	Format string // number_format（如 yyyy-mm-dd / @），命令层写完值后转成 V2 formatter 施加；空则不设
	// FormatExplicit 表示 Format 来自用户的 formats 映射（而非 dtype 默认值），用户显式格式优先。
	FormatExplicit bool
	// HasTime 表示 date 列中至少一个值带非零时分秒；未显式指定格式时据此改用日期时间 formatter，
	// 否则写入的时分秒虽保留在序列号里，却会被 yyyy/MM/dd 格式隐藏，table-get 也读不回来。
	HasTime bool
}

// TableSheetSpec normalize 后的 sheet 规格。
type TableSheetSpec struct {
	Name    string
	Columns []TableColSpec
	Rows    [][]json.RawMessage // 每行每列的原始 JSON 值（保精度，按列 type 解析）
}

// TablePayload normalize 后的 table-put 输入。
type TablePayload struct {
	Sheets []TableSheetSpec
}

// TableSheetIn 用户输入（对齐 pandas to_json(orient="split") 形状）。
type TableSheetIn struct {
	Name    string              `json:"name,omitempty"`
	Columns []string            `json:"columns"`
	Data    [][]json.RawMessage `json:"data"`
	Dtypes  map[string]string   `json:"dtypes,omitempty"`
	Formats map[string]string   `json:"formats,omitempty"`
}

// TablePayloadIn table-put 输入。
type TablePayloadIn struct {
	Sheets []TableSheetIn `json:"sheets"`
}

// ParseTablePutPayload 解析 + normalize + 校验 table-put JSON。
func ParseTablePutPayload(raw []byte) (*TablePayload, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var in TablePayloadIn
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("解析 table-put payload 失败: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("table-put payload 含多余的 JSON 顶层内容")
	}
	if len(in.Sheets) == 0 {
		return nil, fmt.Errorf("table-put payload 至少需要一个 sheet（sheets 数组为空）")
	}
	out := &TablePayload{Sheets: make([]TableSheetSpec, 0, len(in.Sheets))}
	for i, s := range in.Sheets {
		spec, err := normalizeTableSheet(s, i)
		if err != nil {
			return nil, err
		}
		out.Sheets = append(out.Sheets, spec)
	}
	return out, nil
}

func normalizeTableSheet(in TableSheetIn, idx int) (TableSheetSpec, error) {
	if len(in.Columns) == 0 {
		return TableSheetSpec{}, fmt.Errorf("sheets[%d]: columns 为空", idx)
	}
	name := strings.TrimSpace(in.Name)
	cols := make([]TableColSpec, len(in.Columns))
	colIdx := make(map[string]int, len(in.Columns))
	for i, c := range in.Columns {
		if _, dup := colIdx[c]; dup {
			return TableSheetSpec{}, fmt.Errorf("sheets[%d]: 重复列名 %q", idx, c)
		}
		colIdx[c] = i
		typ, format := dtypeToTypeFormat(in.Dtypes[c])
		explicit := false
		if f, ok := in.Formats[c]; ok && strings.TrimSpace(f) != "" {
			format = strings.TrimSpace(f)
			explicit = true
		}
		cols[i] = TableColSpec{Name: c, Type: typ, Format: format, FormatExplicit: explicit}
	}
	for k := range in.Dtypes {
		if _, ok := colIdx[k]; !ok {
			return TableSheetSpec{}, fmt.Errorf("sheets[%d]: dtypes 引用了不存在的列 %q", idx, k)
		}
	}
	for k := range in.Formats {
		if _, ok := colIdx[k]; !ok {
			return TableSheetSpec{}, fmt.Errorf("sheets[%d]: formats 引用了不存在的列 %q", idx, k)
		}
	}
	for i, row := range in.Data {
		if len(row) != len(in.Columns) {
			return TableSheetSpec{}, fmt.Errorf("sheets[%d]: data[%d] 列数 %d 与 columns %d 不一致", idx, i, len(row), len(in.Columns))
		}
	}
	for c := range cols {
		if cols[c].Type != TableColTypeDate {
			continue
		}
		for _, row := range in.Data {
			var s string
			if json.Unmarshal(row[c], &s) == nil && isoHasTimeOfDay(s) {
				cols[c].HasTime = true
				break
			}
		}
	}
	return TableSheetSpec{Name: name, Columns: cols, Rows: in.Data}, nil
}

// dtypeToTypeFormat 映射 pandas dtype → (type, format)。
// 缺失/未知 → string + "@"（文本格式，防数字字符串被识别为数字）。
func dtypeToTypeFormat(dtype string) (TableColType, string) {
	d := strings.TrimSpace(dtype)
	if d == "" {
		return TableColTypeString, "@"
	}
	lower := strings.ToLower(d)
	switch {
	case strings.HasPrefix(lower, "datetime"):
		return TableColTypeDate, "yyyy-mm-dd"
	case lower == "bool" || lower == "boolean":
		return TableColTypeBool, ""
	case isNumericDtype(lower):
		return TableColTypeNumber, ""
	default:
		return TableColTypeString, "@"
	}
}

func isNumericDtype(lower string) bool {
	// interval 以 "int" 开头但不是数值列（pandas IntervalDtype，如
	// "interval[int64, right]"），其值是区间字符串，须按文本处理，显式排除。
	if strings.HasPrefix(lower, "interval") {
		return false
	}
	for _, p := range []string{"int", "uint", "float", "complex"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// excelEpoch Excel/飞书表格序列日期起点（1899-12-30 = 0）。
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// isoDateToSerial 把 ISO 日期 / 日期时间转成 Excel 序列号（对齐官方 lark_sheet_table_io.go）。
//
// 时分秒作为小数部分保留（如 2024-01-15T08:30:00 → 45306.354166…），保证 table-get 输出的
// 日期时间经 table-put 写回不丢时间（实测该序列号配合 yyyy/MM/dd HH:mm:ss 渲染为 2024/01/15 08:30:00）。
// 带时区后缀时保留调用方给出的「墙上时间」：序列号本身不携带时区，按表格本地时间解释。
// 日期与时间之间接受 "T" 或单个空格。
func isoDateToSerial(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("date 列的值为空")
	}
	sep := strings.IndexAny(s, "T ")
	if sep <= 0 {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return 0, fmt.Errorf("date %q 必须是 ISO yyyy-mm-dd 格式: %w", s, err)
		}
		return float64(int(math.Round(t.Sub(excelEpoch).Hours() / 24))), nil
	}
	base := s[:sep]
	date, err := time.Parse("2006-01-02", base)
	if err != nil {
		return 0, fmt.Errorf("date %q 必须是 ISO yyyy-mm-dd 格式: %w", base, err)
	}
	norm := base + "T" + strings.TrimSpace(s[sep+1:])
	clock := norm[len(base):]
	var parsed time.Time
	if strings.ContainsAny(clock, "Zz+-") {
		parsed, err = time.Parse(time.RFC3339Nano, norm)
	} else {
		parsed, err = time.Parse("2006-01-02T15:04:05.999999999", norm)
		if err != nil {
			parsed, err = time.Parse("2006-01-02T15:04", norm)
		}
	}
	if err != nil {
		return 0, fmt.Errorf("datetime %q 必须是 ISO 格式（如 2024-01-15T08:30:00）: %w", s, err)
	}
	days := math.Round(date.Sub(excelEpoch).Hours() / 24)
	secs := float64(parsed.Hour()*3600+parsed.Minute()*60+parsed.Second()) + float64(parsed.Nanosecond())/1e9
	return days + secs/86400, nil
}

// isoHasTimeOfDay 判断 ISO 日期时间串是否带非零时分秒。
func isoHasTimeOfDay(s string) bool {
	serial, err := isoDateToSerial(s)
	if err != nil {
		return false
	}
	return serial != math.Trunc(serial)
}

// formatSerial 把序列号渲染为写入 value 元素的十进制串（无指数、最短往返表示）。
func formatSerial(serial float64) string {
	return strconv.FormatFloat(serial, 'f', -1, 64)
}

// emptyTextCell 返回一个空文本单元格元素。V3 写入里空单元格必须用空文本元素占位，
// 写空元素数组 [] 会触发整批 500（code=1315201）。
func emptyTextCell() *CellElement {
	return &CellElement{Type: "text", Text: &TextElement{Text: ""}}
}

// BuildTypedCell 把一个原始 JSON 值按列规格构造为 V3 CellElement。
//   - string/bool 用 text 元素（字符串塞进 value 元素会被服务端拒收）。
//   - number/date 用 value 元素（date 写 Excel 序列号，真日期格式由命令层后置 formatter 施加）。
//   - 空值（null/缺失）返回空文本元素（不能返回 nil/空数组，否则整批写入 500）。
//
// 注意：不在此处设置任何 cell_styles/number_format —— V3 单元格元素不支持该字段，
// 会被静默丢弃；列的 Format 由命令层在写值后用 V2 style 接口统一施加。
func BuildTypedCell(col TableColSpec, raw json.RawMessage) (*CellElement, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return emptyTextCell(), nil
	}
	switch col.Type {
	case TableColTypeString:
		s, err := stringifyRawScalar(raw)
		if err != nil {
			return nil, fmt.Errorf("string 列解析失败: %w", err)
		}
		return &CellElement{Type: "text", Text: &TextElement{Text: s}}, nil
	case TableColTypeNumber:
		n, blank, err := numberCellLiteral(raw)
		if err != nil {
			return nil, err
		}
		if blank {
			// 数字列里的空串等同 null（官方同样处理：合计行/占位空格不应让整批失败）
			return emptyTextCell(), nil
		}
		return &CellElement{Type: "value", Value: &ValueElement{Value: n.String()}}, nil
	case TableColTypeBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("bool 列期望 true/false，得到 %s", describeJSONType(raw))
		}
		return &CellElement{Type: "text", Text: &TextElement{Text: boolLabel(b)}}, nil
	case TableColTypeDate:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("date 列期望 ISO 日期字符串，得到 %s", describeJSONType(raw))
		}
		if strings.TrimSpace(s) == "" {
			// 日期列里的空串等同 null，写空单元格
			return emptyTextCell(), nil
		}
		serial, err := isoDateToSerial(s)
		if err != nil {
			return nil, err
		}
		return &CellElement{Type: "value", Value: &ValueElement{Value: formatSerial(serial)}}, nil
	default:
		return nil, fmt.Errorf("不支持的列类型 %q", col.Type)
	}
}

// numberCellLiteral 解析数字列的单元格：JSON 数字原样保留；内容恰为 JSON 数字字面量的字符串
// （如 "100.5"）按数字处理；空白字符串返回 blank=true（写空单元格）；其他值报错。
// 对齐官方 coerceNumericCellValue：不接受千分位、货币符号、NaN/Inf 等需要猜测语义的写法。
func numberCellLiteral(raw json.RawMessage) (n json.Number, blank bool, err error) {
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, false, nil
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false, fmt.Errorf("number 列期望数值，得到 %s", describeJSONType(raw))
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", true, nil
	}
	var probe json.Number
	// 字面量 "null" 能无错解码进任何目标且保持零值，需显式排除
	if json.Unmarshal([]byte(trimmed), &probe) != nil || probe == "" {
		return "", false, fmt.Errorf("number 列期望数值，得到非数字字符串 %q；空值请写 null，或把该列从 dtypes 中去掉按文本写入", s)
	}
	return probe, false, nil
}

// boolLabel 把布尔渲染为飞书表格的 TRUE/FALSE 文本字面量。
func boolLabel(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// stringifyRawScalar 把一个原始 JSON 标量渲染为字符串列应写入的文本。
// 用 UseNumber 解析以保住大整数/高精度数字（不经过 float64，避免科学计数法与精度丢失）。
// object/array 这类非标量按原始 JSON 字面量原样返回，不退化成 Go 语法的 map[...]/[...]。
func stringifyRawScalar(raw json.RawMessage) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return boolLabel(t), nil
	case nil:
		return "", nil
	default:
		// object/array：保留原始 JSON 文本，而非 fmt 的 Go 语法表示。
		return strings.TrimSpace(string(raw)), nil
	}
}

func describeJSONType(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return "无法解析的 JSON"
	}
	switch v.(type) {
	case string:
		return "字符串"
	case json.Number:
		return "数字"
	case bool:
		return "布尔"
	case nil:
		return "null"
	case []interface{}:
		return "数组"
	case map[string]interface{}:
		return "对象"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// FormatterForType 把列的 number_format（如 yyyy-mm-dd / @）转成 V2 style 接口接受的
// formatter 串。返回空串表示该列无需后置 formatter。
//   - date 列：飞书 V2 formatter 不接受 yyyy-mm-dd（小写月），统一用 yyyy/MM/dd，
//     使序列号渲染为真日期（实测 45306 → 2024/01/15）。Format 已是其它日期样式时原样透传。
//   - 其它列：原样透传用户在 formats 指定的 number_format（如 #,##0.00）；
//     文本列的 "@" 也透传（V2 接受，强制按文本显示）。
//   - date 列带时分秒（HasTime）且未显式指定格式：用 yyyy/MM/dd HH:mm:ss，否则时间被隐藏。
//     V2 只接受这一种日期时间写法（实测 yyyy-MM-dd HH:mm:ss、yyyy/MM/dd HH:mm 均报 90204 invalid formatter）。
func FormatterForType(col TableColSpec) string {
	f := strings.TrimSpace(col.Format)
	if col.Type == TableColTypeDate {
		if isISODateTimeFormat(f) {
			return v2DateTimeFormatter
		}
		// 默认 / 等价的 ISO 日期格式统一成 V2 接受的 yyyy/MM/dd。
		if f == "" || strings.EqualFold(f, "yyyy-mm-dd") || strings.EqualFold(f, "yyyy/mm/dd") {
			if col.HasTime && !col.FormatExplicit {
				return v2DateTimeFormatter
			}
			return "yyyy/MM/dd"
		}
	}
	return f
}

// v2DateTimeFormatter 是 V2 style 接口接受的日期时间 formatter。
const v2DateTimeFormatter = "yyyy/MM/dd HH:mm:ss"

// TableDateTimeFormat 是 table-get 为带时分秒的日期列输出的 formats 值（table-put 映射回 v2DateTimeFormatter）。
const TableDateTimeFormat = "yyyy-mm-dd hh:mm:ss"

func isISODateTimeFormat(f string) bool {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "yyyy-mm-dd hh:mm:ss", "yyyy/mm/dd hh:mm:ss", "yyyy-mm-dd hh:mm", "yyyy/mm/dd hh:mm":
		return true
	}
	return false
}
