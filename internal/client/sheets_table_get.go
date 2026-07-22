package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// sheets_table_get.go —— sheet table-get 的读侧实现：把电子表格区域读回
// DataFrame 形状（columns + data + dtypes/formats），与 table-put 输入完全对称，
// 支持 table-get → 修改 → table-put 的类型保真 round-trip。
//
// dtype 推断依据 V3 batch_get 的类型化元素：
//   - value 元素     → 数字（json.Number 保精度）
//   - date_time 元素 → 日期（归一为 ISO yyyy-mm-dd）
//   - text 元素      → 文本；全列值 ∈ {TRUE,FALSE} 时升级为 bool
//   - formula 元素   → 取 formula_value，可解析为数字则按数字，否则文本
//   - 混合类型列     → object（逐格保留各自类型）

// TableGetSheet table-get 单 sheet 结果（形状对齐 table-put 的 TableSheetIn）。
type TableGetSheet struct {
	Name    string            `json:"name"`
	Range   string            `json:"range"`
	Columns []string          `json:"columns"`
	Data    [][]any           `json:"data"`
	Dtypes  map[string]string `json:"dtypes"`
	Formats map[string]string `json:"formats,omitempty"`
}

// TableGetResult table-get 输出（顶层形状对齐 table-put 输入）。
type TableGetResult struct {
	Sheets []TableGetSheet `json:"sheets"`
}

// cellScalar 单元格归一后的标量：kind ∈ empty/number/date/text。
type cellScalar struct {
	kind string
	num  json.Number
	text string
}

// ReadTable 读取指定区域并推断列类型。rangeStr 为空时读整张 sheet 的 used range
// （按 grid 行列数推算）。noHeader 为 true 时首行按数据行处理，列名生成 col_1..col_n。
func ReadTable(ctx context.Context, spreadsheetToken, sheetID, rangeStr string, noHeader bool, userAccessToken string) (*TableGetResult, error) {
	if rangeStr == "" {
		rows, cols, err := sheetGridSize(ctx, spreadsheetToken, sheetID, userAccessToken)
		if err != nil {
			return nil, fmt.Errorf("获取 sheet 元信息失败（也可显式传 --range）: %w", err)
		}
		if rows <= 0 || cols <= 0 {
			return nil, fmt.Errorf("sheet %s 无法推断行列数，请显式传 --range", sheetID)
		}
		rangeStr = fmt.Sprintf("A1:%s%d", ColumnIndexToLetter(cols-1), rows)
	}
	fullRange := rangeStr
	if !strings.Contains(fullRange, "!") {
		fullRange = sheetID + "!" + fullRange
	}

	ranges, err := ReadCellsRichV3(ctx, spreadsheetToken, sheetID, []string{fullRange},
		"formatted_string", "", "", userAccessToken)
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("range %s 无返回数据", fullRange)
	}
	grid := ranges[0]

	// 每行每列归一为标量
	scalars := make([][]cellScalar, len(grid.Values))
	maxCols := 0
	for i, row := range grid.Values {
		scalars[i] = make([]cellScalar, len(row))
		for j, elems := range row {
			scalars[i][j] = normalizeCellElements(elems)
		}
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	// 裁掉尾部整行/整列全空（used range 常大于实际数据）
	scalars = trimEmptyEdges(scalars, maxCols)
	if len(scalars) == 0 {
		return &TableGetResult{Sheets: []TableGetSheet{{
			Name: sheetID, Range: grid.Range, Columns: []string{}, Data: [][]any{}, Dtypes: map[string]string{},
		}}}, nil
	}
	nCols := len(scalars[0])

	// 列名（去重：dtypes/formats 以列名为键，且 table-put 硬拒重复列名——
	// 不去重会让 get→put round-trip 在含重复表头的表上必然失败）
	var columns []string
	dataRows := scalars
	if noHeader {
		for j := 0; j < nCols; j++ {
			columns = append(columns, fmt.Sprintf("col_%d", j+1))
		}
	} else {
		for j := 0; j < nCols; j++ {
			name := strings.TrimSpace(scalars[0][j].text)
			if scalars[0][j].kind == "number" {
				name = scalars[0][j].num.String()
			}
			if name == "" {
				name = fmt.Sprintf("col_%d", j+1)
			}
			columns = append(columns, name)
		}
		dataRows = scalars[1:]
	}
	columns = dedupeColumnNames(columns)

	// 逐列推断 dtype
	dtypes := make(map[string]string, nCols)
	formats := make(map[string]string)
	data := make([][]any, len(dataRows))
	for i := range data {
		data[i] = make([]any, nCols)
	}
	for j := 0; j < nCols; j++ {
		dtype := inferColumnDtype(dataRows, j)
		dtypes[columns[j]] = dtype
		if dtype == "datetime64[ns]" {
			formats[columns[j]] = "yyyy-mm-dd"
		}
		for i, row := range dataRows {
			var sc cellScalar
			if j < len(row) {
				sc = row[j]
			}
			data[i][j] = scalarToJSON(sc, dtype)
		}
	}

	return &TableGetResult{Sheets: []TableGetSheet{{
		Name:    sheetID,
		Range:   grid.Range,
		Columns: columns,
		Data:    data,
		Dtypes:  dtypes,
		Formats: formats,
	}}}, nil
}

// sheetGridSize 通过 V2 metainfo 拿指定子表的行列数（支持 User Token）。
func sheetGridSize(ctx context.Context, spreadsheetToken, sheetID, userAccessToken string) (int, int, error) {
	meta, err := GetSpreadsheetMeta(ctx, spreadsheetToken, "", userAccessToken)
	if err != nil {
		return 0, 0, err
	}
	sheets, _ := meta["sheets"].([]any)
	for _, s := range sheets {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["sheetId"].(string); id != sheetID {
			continue
		}
		rows := int(toFloat(m["rowCount"]))
		cols := int(toFloat(m["columnCount"]))
		return rows, cols, nil
	}
	return 0, 0, fmt.Errorf("未找到子表 %s", sheetID)
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

// normalizeCellElements 把 V3 单元格元素列表归一为标量。
func normalizeCellElements(elems []*CellElement) cellScalar {
	var texts []string
	var nums []json.Number
	var dates []string
	for _, e := range elems {
		if e == nil {
			continue
		}
		switch e.Type {
		case "value":
			if e.Value != nil {
				nums = append(nums, json.Number(e.Value.Value))
			}
		case "date_time":
			if e.DateTime != nil {
				dates = append(dates, e.DateTime.DateTime)
			}
		case "text":
			if e.Text != nil {
				texts = append(texts, e.Text.Text)
			}
		case "formula":
			if e.Formula != nil {
				v := e.Formula.FormulaValue
				if isNumericLiteral(v) {
					nums = append(nums, json.Number(v))
				} else if v != "" {
					texts = append(texts, v)
				}
			}
		case "link":
			if e.Link != nil {
				texts = append(texts, e.Link.Text)
			}
		case "mention_user":
			if e.MentionUser != nil {
				texts = append(texts, e.MentionUser.Name)
			}
		case "mention_document":
			if e.MentionDocument != nil {
				texts = append(texts, e.MentionDocument.Title)
			}
		case "image":
			if e.Image != nil {
				texts = append(texts, "[image:"+e.Image.ImageToken+"]")
			}
		case "file":
			if e.File != nil {
				texts = append(texts, e.File.Name)
			}
		}
	}
	switch {
	case len(dates) > 0 && len(nums) == 0 && len(texts) == 0:
		return cellScalar{kind: "date", text: dates[0]}
	case len(nums) == 1 && len(texts) == 0 && len(dates) == 0:
		return cellScalar{kind: "number", num: nums[0]}
	default:
		joined := strings.Join(texts, "")
		for _, n := range nums {
			joined += n.String()
		}
		for _, d := range dates {
			joined += d
		}
		if strings.TrimSpace(joined) == "" {
			return cellScalar{kind: "empty"}
		}
		return cellScalar{kind: "text", text: joined}
	}
}

// inferColumnDtype 按整列非空单元格推断 dtype（pandas 命名，与 table-put 输入对齐）。
func inferColumnDtype(rows [][]cellScalar, col int) string {
	counts := map[string]int{}
	boolCandidate := true
	nonEmpty := 0
	for _, row := range rows {
		if col >= len(row) {
			continue
		}
		sc := row[col]
		if sc.kind == "empty" {
			continue
		}
		nonEmpty++
		counts[sc.kind]++
		if sc.kind != "text" || !isBoolLabel(sc.text) {
			boolCandidate = false
		}
	}
	if nonEmpty == 0 {
		return "string"
	}
	if boolCandidate {
		return "bool"
	}
	if counts["number"] == nonEmpty {
		return "float64"
	}
	if counts["date"] == nonEmpty {
		return "datetime64[ns]"
	}
	if counts["text"] == nonEmpty {
		return "string"
	}
	return "object"
}

// scalarToJSON 按列 dtype 把标量转为输出 JSON 值。
func scalarToJSON(sc cellScalar, dtype string) any {
	if sc.kind == "empty" {
		return nil
	}
	switch dtype {
	case "bool":
		return strings.EqualFold(strings.TrimSpace(sc.text), "TRUE")
	case "float64":
		return sc.num
	case "datetime64[ns]":
		return normalizeDateString(sc.text)
	case "string":
		return sc.text
	default: // object：逐格保留类型
		switch sc.kind {
		case "number":
			return sc.num
		case "date":
			return normalizeDateString(sc.text)
		default:
			return sc.text
		}
	}
}

// normalizeDateString 尽力把飞书返回的日期串归一为 ISO yyyy-mm-dd（带时间保留时间部分）。
// 无法解析时原样返回。
func normalizeDateString(s string) string {
	s = strings.TrimSpace(s)
	layouts := []struct {
		in, out string
	}{
		{"2006-01-02 15:04:05", "2006-01-02T15:04:05"},
		{"2006/01/02 15:04:05", "2006-01-02T15:04:05"},
		{"2006-01-02 15:04", "2006-01-02T15:04"},
		{"2006/01/02 15:04", "2006-01-02T15:04"},
		{"2006-01-02", "2006-01-02"},
		{"2006/01/02", "2006-01-02"},
	}
	for _, l := range layouts {
		if t, err := time.Parse(l.in, s); err == nil {
			return t.Format(l.out)
		}
	}
	return s
}

func isBoolLabel(s string) bool {
	up := strings.ToUpper(strings.TrimSpace(s))
	return up == "TRUE" || up == "FALSE"
}

func isNumericLiteral(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	var n json.Number
	return json.Unmarshal([]byte(s), &n) == nil
}

// trimEmptyEdges 去掉尾部全空行与右侧全空列，并把所有行补齐到同一列数。
func trimEmptyEdges(rows [][]cellScalar, maxCols int) [][]cellScalar {
	// 尾部全空行
	lastRow := -1
	for i, row := range rows {
		for _, sc := range row {
			if sc.kind != "empty" {
				lastRow = i
				break
			}
		}
	}
	rows = rows[:lastRow+1]
	// 右侧全空列
	lastCol := -1
	for _, row := range rows {
		for j, sc := range row {
			if sc.kind != "empty" && j > lastCol {
				lastCol = j
			}
		}
	}
	if lastCol < 0 {
		return nil
	}
	out := make([][]cellScalar, len(rows))
	for i, row := range rows {
		padded := make([]cellScalar, lastCol+1)
		for j := 0; j <= lastCol && j < len(row); j++ {
			padded[j] = row[j]
		}
		for j := len(row); j <= lastCol; j++ {
			padded[j] = cellScalar{kind: "empty"}
		}
		out[i] = padded
	}
	return out
}

// dedupeColumnNames 给重复列名追加 _2/_3 后缀（首个保持原名）。
// 后缀本身撞名时继续递增，保证结果全局唯一。
func dedupeColumnNames(columns []string) []string {
	seen := make(map[string]int, len(columns))
	out := make([]string, len(columns))
	for i, name := range columns {
		if seen[name] == 0 {
			seen[name] = 1
			out[i] = name
			continue
		}
		for {
			seen[name]++
			candidate := fmt.Sprintf("%s_%d", name, seen[name])
			if seen[candidate] == 0 {
				seen[candidate] = 1
				out[i] = candidate
				break
			}
		}
	}
	return out
}

// ColumnIndexToLetter 0-based 列号转 Excel 列字母（0→A, 25→Z, 26→AA）。
// 唯一的 base-26 进位实现，1-based 调用方传 n-1（cmd 侧 colIndexToLetter 薄壳）。
func ColumnIndexToLetter(idx int) string {
	s := ""
	for idx >= 0 {
		s = string(rune('A'+idx%26)) + s
		idx = idx/26 - 1
	}
	return s
}
