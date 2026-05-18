package converter

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
)

const defaultMaxEmbeddedRows = 100
const defaultMaxEmbeddedCols = 50

var baseV3Call = client.BaseV3Call

type feishuEmbeddedTableFetcher struct{}

func (feishuEmbeddedTableFetcher) FetchSheet(token, sheetID string, maxRows, maxCols int, userAccessToken string) (*EmbeddedTableData, error) {
	if token == "" || sheetID == "" {
		return nil, fmt.Errorf("sheet token 或 sheet id 为空")
	}
	rowLimit := normalizedMaxEmbeddedRows(maxRows)
	colLimit := normalizedMaxEmbeddedCols(maxCols)
	rangeStr := fmt.Sprintf("%s!A1:%s%d", sheetID, columnName(colLimit), rowLimit+1)
	cellRange, err := client.ReadCells(context.Background(), token, rangeStr, "ToString", "FormattedString", userAccessToken)
	if err != nil {
		return nil, err
	}
	if cellRange == nil {
		return nil, fmt.Errorf("sheet 返回空数据")
	}
	rows := anyRowsToStringRows(cellRange.Values)
	if len(rows) == 0 {
		return &EmbeddedTableData{}, nil
	}
	return tableDataFromRows(rows, rowLimit, colLimit), nil
}

func (feishuEmbeddedTableFetcher) FetchBitable(baseToken, tableID string, maxRows, maxCols int, userAccessToken string) (*EmbeddedTableData, error) {
	if baseToken == "" || tableID == "" {
		return nil, fmt.Errorf("bitable token 或 table id 为空")
	}
	rowLimit := normalizedMaxEmbeddedRows(maxRows)
	colLimit := normalizedMaxEmbeddedCols(maxCols)
	headers, err := fetchBitableHeaders(baseToken, tableID, colLimit, userAccessToken)
	if err != nil {
		return nil, err
	}
	records, hasMore, err := fetchBitableRecords(baseToken, tableID, rowLimit+1, userAccessToken)
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		headers = inferBitableHeaders(records)
	}
	if len(headers) > colLimit {
		headers = headers[:colLimit]
	}

	rows := make([][]string, 0, len(records))
	for _, record := range records {
		row := make([]string, len(headers))
		for i, header := range headers {
			row[i] = embeddedCellString(record[header])
		}
		rows = append(rows, row)
	}
	data := &EmbeddedTableData{Headers: headers, Rows: rows}
	if len(data.Rows) > rowLimit {
		data.TruncatedRows = len(data.Rows) - rowLimit
		data.Rows = data.Rows[:rowLimit]
	} else if hasMore {
		data.TruncatedRows = 1
	}
	return data, nil
}

func normalizedMaxEmbeddedRows(maxRows int) int {
	if maxRows <= 0 {
		return defaultMaxEmbeddedRows
	}
	return maxRows
}

func normalizedMaxEmbeddedCols(maxCols int) int {
	if maxCols <= 0 {
		return defaultMaxEmbeddedCols
	}
	return maxCols
}

func tableDataFromRows(rows [][]string, maxRows, maxCols int) *EmbeddedTableData {
	data := &EmbeddedTableData{}
	rows = trimEmptyTableEdges(rows)
	if len(rows) == 0 {
		return data
	}
	data.Headers = rows[0]
	if len(data.Headers) > maxCols {
		data.Headers = data.Headers[:maxCols]
	}
	if len(data.Headers) == 0 {
		return data
	}
	data.Rows = rows[1:]
	if len(data.Rows) > maxRows {
		data.TruncatedRows = len(data.Rows) - maxRows
		data.Rows = data.Rows[:maxRows]
	}
	return data
}

func trimEmptyTableEdges(rows [][]string) [][]string {
	lastRow := -1
	lastCol := -1
	for i, row := range rows {
		for j, cell := range row {
			if strings.TrimSpace(cell) != "" {
				lastRow = i
				if j > lastCol {
					lastCol = j
				}
			}
		}
	}
	if lastRow < 0 || lastCol < 0 {
		return nil
	}
	trimmed := make([][]string, lastRow+1)
	for i := 0; i <= lastRow; i++ {
		trimmed[i] = make([]string, lastCol+1)
		copy(trimmed[i], rows[i])
	}
	return trimmed
}

func columnName(n int) string {
	if n <= 0 {
		return "A"
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('A' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}

func anyRowsToStringRows(values [][]any) [][]string {
	rows := make([][]string, len(values))
	for i, row := range values {
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			rows[i][j] = embeddedCellString(cell)
		}
	}
	return rows
}

func fetchBitableHeaders(baseToken, tableID string, maxCols int, userAccessToken string) ([]string, error) {
	headers := make([]string, 0, maxCols)
	pageToken := ""
	for {
		params := map[string]any{"page_size": 100}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := baseV3Call("GET", client.BaseV3Path("bases", baseToken, "tables", tableID, "fields"), params, nil, userAccessToken)
		if err != nil {
			return nil, err
		}
		items, _ := data["items"].([]any)
		if fields, ok := data["fields"].([]any); ok {
			items = fields
		}
		for _, item := range items {
			m, _ := item.(map[string]any)
			name, _ := m["field_name"].(string)
			if name == "" {
				name, _ = m["name"].(string)
			}
			if name != "" {
				headers = append(headers, name)
				if len(headers) >= maxCols {
					return headers, nil
				}
			}
		}
		if !boolValue(data["has_more"]) {
			break
		}
		pageToken = stringValue(data["page_token"])
		if pageToken == "" {
			break
		}
	}
	return headers, nil
}

func fetchBitableRecords(baseToken, tableID string, limit int, userAccessToken string) ([]map[string]any, bool, error) {
	data, err := baseV3Call("GET", client.BaseV3Path("bases", baseToken, "tables", tableID, "records"), map[string]any{"page_size": limit}, nil, userAccessToken)
	if err != nil {
		return nil, false, err
	}
	if matrix, ok := data["data"].([]any); ok {
		return bitableMatrixRecords(data, matrix), boolValue(data["has_more"]), nil
	}
	items, _ := data["items"].([]any)
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		fields, _ := m["fields"].(map[string]any)
		if fields != nil {
			records = append(records, fields)
		}
	}
	hasMore, _ := data["has_more"].(bool)
	return records, hasMore, nil
}

func bitableMatrixRecords(data map[string]any, matrix []any) []map[string]any {
	fields := stringSliceFromAny(data["fields"])
	records := make([]map[string]any, 0, len(matrix))
	for _, rawRow := range matrix {
		row, _ := rawRow.([]any)
		record := make(map[string]any, len(fields))
		for i, field := range fields {
			if i < len(row) {
				record[field] = row[i]
			}
		}
		records = append(records, record)
	}
	return records
}

func stringSliceFromAny(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func inferBitableHeaders(records []map[string]any) []string {
	seen := map[string]bool{}
	for _, record := range records {
		for key := range record {
			seen[key] = true
		}
	}
	headers := make([]string, 0, len(seen))
	for key := range seen {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

func embeddedCellString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			text := embeddedCellString(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, key := range []string{"text", "name", "title", "link", "url", "email", "en_name", "id"} {
			if s, ok := x[key].(string); ok && s != "" {
				return strings.TrimSpace(s)
			}
		}
		if data, err := json.Marshal(x); err == nil {
			return string(data)
		}
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

func sanitizeMarkdownTableCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "<br>")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}
