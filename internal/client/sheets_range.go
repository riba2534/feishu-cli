package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// sheets_range.go —— 电子表格范围（A1 写法）相关的纯函数：子表前缀拆分、A1 解析、
// 行列区间（"3:5" / "B:D"）解析，以及 v2 写入的分批规划。全部离线可测，不发网络请求。

// SplitSheetRangePrefix 把 "Sheet1!A1:D20" 拆成 ("Sheet1", "A1:D20", true)。
//
// 规则对齐官方（shortcuts/sheets/range_sheet_prefix.go）与前端公式词法：
//   - 分隔符接受 "!"、全角 "！"，以及前面带反斜杠的转义写法（`Sheet1\!A1`，规避 zsh 历史展开）；
//   - 带引号的子表名（'My Sheet'!A1）去掉引号，” 还原为单个 '，引号内可以包含 "!"；
//   - 不带引号时按第一个分隔符切分。
//
// 没有分隔符、任一侧为空或引号未闭合时 ok=false（视为没有前缀）。
func SplitSheetRangePrefix(rng string) (sheet, rest string, ok bool) {
	rng = strings.TrimSpace(rng)
	sheet, end, ok := scanSheetQualifier(rng)
	if !ok {
		return "", "", false
	}
	rest = strings.TrimSpace(rng[end:])
	if sheet == "" || rest == "" {
		return "", "", false
	}
	return sheet, rest, true
}

// scanSheetQualifier 返回限定符中的子表名与分隔符之后的偏移量。
func scanSheetQualifier(rng string) (sheet string, end int, ok bool) {
	if strings.HasPrefix(rng, "'") {
		return scanQuotedSheetQualifier(rng)
	}
	for i, r := range rng {
		if r != '!' && r != '！' {
			continue
		}
		nameEnd := i
		// 紧挨着的反斜杠属于分隔符的转义写法，不属于子表名
		if i > 0 && rng[i-1] == '\\' {
			nameEnd = i - 1
		}
		return strings.TrimSpace(rng[:nameEnd]), i + len(string(r)), true
	}
	return "", 0, false
}

func scanQuotedSheetQualifier(rng string) (sheet string, end int, ok bool) {
	var name strings.Builder
	for i := 1; i < len(rng); i++ {
		if rng[i] != '\'' {
			name.WriteByte(rng[i])
			continue
		}
		if i+1 < len(rng) && rng[i+1] == '\'' {
			// '' 表示一个字面量单引号
			name.WriteByte('\'')
			i++
			continue
		}
		sepStart := i + 1
		tail := rng[sepStart:]
		ws := len(tail) - len(strings.TrimLeft(tail, " \t\r\n"))
		tail = tail[ws:]
		sepLen := ws
		if strings.HasPrefix(tail, `\`) {
			tail, sepLen = tail[1:], sepLen+1
		}
		switch {
		case strings.HasPrefix(tail, "!"):
			sepLen++
		case strings.HasPrefix(tail, "！"):
			sepLen += len("！")
		default:
			return "", 0, false
		}
		return strings.TrimSpace(name.String()), sepStart + sepLen, true
	}
	return "", 0, false
}

// a1Ref 是 A1 写法中的一个端点（列字母 + 行号，两者都可缺省）。
// col/row 为 0 起始；缺省时为 -1。
type a1Ref struct {
	col, row int
}

// parseA1Ref 解析单个端点：A1 / A / 1 / $A$1。空串返回 ok=false。
func parseA1Ref(s string) (a1Ref, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "$", "")
	if s == "" {
		return a1Ref{}, false
	}
	i := 0
	for i < len(s) && ((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
		i++
	}
	letters, digits := s[:i], s[i:]
	if len(letters) > 3 {
		return a1Ref{}, false
	}
	ref := a1Ref{col: -1, row: -1}
	if letters != "" {
		ref.col = ColumnToIndex(letters)
	}
	if digits != "" {
		for _, c := range digits {
			if c < '0' || c > '9' {
				return a1Ref{}, false
			}
		}
		n, err := strconv.Atoi(digits)
		if err != nil || n <= 0 {
			return a1Ref{}, false
		}
		ref.row = n - 1
	}
	if letters == "" && digits == "" {
		return a1Ref{}, false
	}
	return ref, true
}

// A1Bounds 是去掉子表前缀后的 A1 区域。缺省的维度用 -1 表示（如整列 "A:C" 的行）。
type A1Bounds struct {
	StartCol, StartRow int
	EndCol, EndRow     int
}

// ParseA1Bounds 解析 "A1:C10" / "A1" / "A:C" / "1:3"。不接受带子表前缀的输入。
func ParseA1Bounds(s string) (A1Bounds, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "!！") {
		return A1Bounds{}, false
	}
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return A1Bounds{}, false
	}
	start, ok := parseA1Ref(parts[0])
	if !ok {
		return A1Bounds{}, false
	}
	end := start
	if len(parts) == 2 {
		if end, ok = parseA1Ref(parts[1]); !ok {
			return A1Bounds{}, false
		}
		// 两端的维度形状必须一致（不能 "A:3"）
		if (start.col < 0) != (end.col < 0) || (start.row < 0) != (end.row < 0) {
			return A1Bounds{}, false
		}
	}
	return A1Bounds{StartCol: start.col, StartRow: start.row, EndCol: end.col, EndRow: end.row}, true
}

// LooksLikeA1Range 判断不带 "!" 的范围参数是否为 A1 写法（否则视为子表 ID/名称本身）。
func LooksLikeA1Range(s string) bool {
	_, ok := ParseA1Bounds(s)
	return ok
}

// DimRange 是一段整行或整列区间，索引为飞书 v2 dimension_range 删除/更新接口的口径：
// 从 1 开始、两端包含（实测：DELETE startIndex=2,endIndex=3 删除第 2、3 行，delCount=2；
// startIndex=0 报 90202 "StartIndex or EndIndex must greater than 0"）。
type DimRange struct {
	Major string // ROWS / COLUMNS
	Start int    // 1 起始，包含
	End   int    // 1 起始，包含
}

// Count 返回区间覆盖的行/列数。
func (d DimRange) Count() int { return d.End - d.Start + 1 }

// A1 返回区间的 A1 写法（行 "3:5"，列 "B:D"）。
func (d DimRange) A1() string {
	if d.Major == "COLUMNS" {
		return IndexToColumn(d.Start-1) + ":" + IndexToColumn(d.End-1)
	}
	return fmt.Sprintf("%d:%d", d.Start, d.End)
}

// ParseDimRange 解析 A1 行列区间（与官方 +dim-* 一致：1 起始、两端包含）：
//
//	"3:5" / "3"   → ROWS 3..5 / 3..3
//	"B:D" / "B"   → COLUMNS 2..4 / 2..2
//
// 不允许混用行号与列字母，结束不得早于开始。
func ParseDimRange(s string) (DimRange, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "$", "")
	if s == "" {
		return DimRange{}, fmt.Errorf("范围为空")
	}
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return DimRange{}, fmt.Errorf("范围 %q 格式错误，应为 \"3:5\"（行）或 \"B:D\"（列）", s)
	}
	major1, idx1, err := parseDimPosition(parts[0])
	if err != nil {
		return DimRange{}, err
	}
	major2, idx2 := major1, idx1
	if len(parts) == 2 {
		if major2, idx2, err = parseDimPosition(parts[1]); err != nil {
			return DimRange{}, err
		}
	}
	if major1 != major2 {
		return DimRange{}, fmt.Errorf("范围 %q 不能混用行号与列字母", s)
	}
	if idx2 < idx1 {
		return DimRange{}, fmt.Errorf("范围 %q 的结束位置早于开始位置", s)
	}
	return DimRange{Major: major1, Start: idx1, End: idx2}, nil
}

// parseDimPosition 解析单个位置：纯数字为行号（1 起始），纯字母为列字母（A=1）。
func parseDimPosition(s string) (major string, idx int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("位置为空")
	}
	isDigits, isLetters := true, true
	for _, r := range s {
		if r < '0' || r > '9' {
			isDigits = false
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			isLetters = false
		}
	}
	switch {
	case isDigits:
		n, convErr := strconv.Atoi(s)
		if convErr != nil || n <= 0 {
			return "", 0, fmt.Errorf("行号必须 ≥ 1，得到 %q", s)
		}
		return "ROWS", n, nil
	case isLetters && len(s) <= 3:
		return "COLUMNS", ColumnToIndex(s) + 1, nil
	default:
		return "", 0, fmt.Errorf("位置 %q 应为纯数字行号（如 3）或纯字母列号（如 C）", s)
	}
}

// DecodeSheetValues 解析 JSON 二维数组，数字保留为 json.Number（不经过 float64），
// 避免 1000000 → 1e+06、19 位整数丢精度。
func DecodeSheetValues(raw []byte) ([][]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var values [][]any
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("JSON 二维数组之后存在多余内容")
	}
	return values, nil
}

// v2 单范围写入 / 追加接口的上限（官方文档：单次写入不得超过 5000 行、100 列）。
const (
	SheetV2MaxRowsPerWrite = 5000
	SheetV2MaxColsPerWrite = 100
)

// SheetWriteChunk 是一次 v2 写入请求：Range 为带子表前缀的完整范围，Values 为对应数据块。
type SheetWriteChunk struct {
	Range  string
	Values [][]any
}

// MaxRowWidth 返回二维数组中最长一行的列数。
func MaxRowWidth(values [][]any) int {
	w := 0
	for _, row := range values {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

// PlanSheetWriteChunks 把一次 v2 写入按「≤maxRows 行 × ≤maxCols 列」拆成多个请求。
//
// 未超限时原样返回单个分块（range 不做任何改写，保持旧行为）。超限时以范围左上角为锚点
// 计算每块的精确范围；范围显式声明的结束行/列小于数据尺寸时报错（与接口「range 需大于等于
// 写入数据范围」的约束一致，避免写到声明范围之外）。
func PlanSheetWriteChunks(fullRange string, values [][]any, maxRows, maxCols int) ([]SheetWriteChunk, error) {
	if maxRows <= 0 {
		maxRows = SheetV2MaxRowsPerWrite
	}
	if maxCols <= 0 {
		maxCols = SheetV2MaxColsPerWrite
	}
	width := MaxRowWidth(values)
	if len(values) <= maxRows && width <= maxCols {
		return []SheetWriteChunk{{Range: fullRange, Values: values}}, nil
	}
	sheetID, rest, ok := SplitSheetRangePrefix(fullRange)
	if !ok {
		// 只有子表 ID（整表）时以 A1 为锚点
		if strings.TrimSpace(fullRange) == "" || strings.ContainsAny(fullRange, "!！") {
			return nil, fmt.Errorf("范围 %q 无法解析，无法自动分批写入", fullRange)
		}
		sheetID, rest = strings.TrimSpace(fullRange), "A1"
	}
	b, ok := ParseA1Bounds(rest)
	if !ok {
		return nil, fmt.Errorf("范围 %q 无法解析，无法自动分批写入；请使用 <sheetId>!A1:C10 形式", fullRange)
	}
	startCol, startRow := b.StartCol, b.StartRow
	if startCol < 0 {
		startCol = 0
	}
	if startRow < 0 {
		startRow = 0
	}
	if !strings.Contains(rest, ":") {
		// 单个单元格（如 "A1"）只作为左上角锚点，不限制行列数
		b.EndRow, b.EndCol = -1, -1
	}
	if b.EndRow >= 0 && b.EndRow-startRow+1 < len(values) {
		return nil, fmt.Errorf("范围 %q 只有 %d 行，小于数据的 %d 行；请扩大范围或只写左上角单元格（如 %s!%s%d）",
			fullRange, b.EndRow-startRow+1, len(values), sheetID, IndexToColumn(startCol), startRow+1)
	}
	if b.EndCol >= 0 && b.EndCol-startCol+1 < width {
		return nil, fmt.Errorf("范围 %q 只有 %d 列，小于数据的 %d 列；请扩大范围或只写左上角单元格（如 %s!%s%d）",
			fullRange, b.EndCol-startCol+1, width, sheetID, IndexToColumn(startCol), startRow+1)
	}

	var chunks []SheetWriteChunk
	for r0 := 0; r0 < len(values); r0 += maxRows {
		r1 := r0 + maxRows
		if r1 > len(values) {
			r1 = len(values)
		}
		block := values[r0:r1]
		blockWidth := MaxRowWidth(block)
		if blockWidth == 0 {
			blockWidth = 1
		}
		for c0 := 0; c0 < blockWidth; c0 += maxCols {
			c1 := c0 + maxCols
			if c1 > blockWidth {
				c1 = blockWidth
			}
			sub := make([][]any, len(block))
			for i, row := range block {
				if c0 >= len(row) {
					sub[i] = []any{}
					continue
				}
				end := c1
				if end > len(row) {
					end = len(row)
				}
				sub[i] = row[c0:end]
			}
			rng := fmt.Sprintf("%s!%s%d:%s%d", sheetID,
				IndexToColumn(startCol+c0), startRow+r0+1,
				IndexToColumn(startCol+c1-1), startRow+r1)
			chunks = append(chunks, SheetWriteChunk{Range: rng, Values: sub})
		}
	}
	return chunks, nil
}

// unmarshalUseNumber 与 json.Unmarshal 相同，但数字解析为 json.Number。
func unmarshalUseNumber(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// SheetRangesSpan 返回多个「<sheetId>!A1:C10」范围的外接矩形（子表取第一个范围的）。
// 任一范围无法解析时返回第一个范围原文。
func SheetRangesSpan(ranges []string) string {
	if len(ranges) == 0 {
		return ""
	}
	if len(ranges) == 1 {
		return ranges[0]
	}
	sheetID := ""
	minCol, minRow, maxCol, maxRow := -1, -1, -1, -1
	for i, r := range ranges {
		sid, rest, ok := SplitSheetRangePrefix(r)
		if !ok {
			return ranges[0]
		}
		if i == 0 {
			sheetID = sid
		}
		b, ok := ParseA1Bounds(rest)
		if !ok || b.StartCol < 0 || b.StartRow < 0 || b.EndCol < 0 || b.EndRow < 0 {
			return ranges[0]
		}
		if minCol < 0 || b.StartCol < minCol {
			minCol = b.StartCol
		}
		if minRow < 0 || b.StartRow < minRow {
			minRow = b.StartRow
		}
		if b.EndCol > maxCol {
			maxCol = b.EndCol
		}
		if b.EndRow > maxRow {
			maxRow = b.EndRow
		}
	}
	return fmt.Sprintf("%s!%s%d:%s%d", sheetID, IndexToColumn(minCol), minRow+1, IndexToColumn(maxCol), maxRow+1)
}

// AppendChunkRange 计算分批追加时下一批的搜索范围：以上一批实际写入范围（updatedRange）的下一行为起点，
// 列与原范围起始列对齐、宽度为数据列数。values_append 会从该范围起始向下找第一个空白位置写入，
// 锚定在上一批之后可避免数据中的整行空白被下一批覆盖。无法解析时返回 fallback。
func AppendChunkRange(fallback, prevUpdatedRange string, width, rows int) string {
	sheetID, rest, ok := SplitSheetRangePrefix(prevUpdatedRange)
	if !ok {
		return fallback
	}
	prev, ok := ParseA1Bounds(rest)
	if !ok || prev.EndRow < 0 {
		return fallback
	}
	startCol := 0
	if _, frest, ok := SplitSheetRangePrefix(fallback); ok {
		if fb, ok := ParseA1Bounds(frest); ok && fb.StartCol >= 0 {
			startCol = fb.StartCol
		}
	} else if prev.StartCol >= 0 {
		startCol = prev.StartCol
	}
	if width < 1 {
		width = 1
	}
	if rows < 1 {
		rows = 1
	}
	start := prev.EndRow + 2 // EndRow 为 0 起始，下一行的 1 起始行号 = EndRow+2
	return fmt.Sprintf("%s!%s%d:%s%d", sheetID, IndexToColumn(startCol), start, IndexToColumn(startCol+width-1), start+rows-1)
}
