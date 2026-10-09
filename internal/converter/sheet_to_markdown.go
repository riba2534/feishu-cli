package converter

import (
	"context"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
)

// SheetData 单个工作表的数据
type SheetData struct {
	Title  string  // 工作表标题
	Values [][]any // 二维数组，V2 API 返回
}

const (
	sheetMarkdownReadChunkCells = 5000
	sheetMarkdownMaxReadCells   = 100000
)

// SheetToMarkdown 将电子表格数据转换为 Markdown
func SheetToMarkdown(sheets []*SheetData) string {
	var sb strings.Builder

	for i, sheet := range sheets {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}

		if sheet.Title != "" {
			sb.WriteString("## ")
			sb.WriteString(sheet.Title)
			sb.WriteString("\n\n")
		}

		rows := trimEmptyRows(sheet.Values)
		if len(rows) == 0 {
			sb.WriteString("（空工作表）\n")
			continue
		}

		// 确定最大列数
		maxCols := 0
		for _, row := range rows {
			if len(row) > maxCols {
				maxCols = len(row)
			}
		}

		// 裁剪尾部全空列
		maxCols = trimEmptyCols(rows, maxCols)
		if maxCols == 0 {
			sb.WriteString("（空工作表）\n")
			continue
		}

		// 第一行作为表头
		writeRow(&sb, rows[0], maxCols)
		// 分隔行
		sb.WriteString("|")
		for c := 0; c < maxCols; c++ {
			sb.WriteString(" --- |")
		}
		sb.WriteString("\n")

		// 数据行
		for _, row := range rows[1:] {
			writeRow(&sb, row, maxCols)
		}

		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FetchSheetDataForMarkdown 读取电子表格数据，供 docx 内嵌 Sheet 自动展开使用。
// sheetID 为空时导出所有可见工作表；非空时只导出对应工作表。
func FetchSheetDataForMarkdown(spreadsheetToken, sheetID, userAccessToken string) ([]*SheetData, error) {
	ctx := client.Context()
	sheets, err := client.QuerySheets(ctx, spreadsheetToken, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("获取工作表列表失败: %w", err)
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("电子表格中没有工作表")
	}

	var targets []*client.SheetInfo
	for _, sheet := range sheets {
		if sheetID != "" && sheet.SheetID != sheetID {
			continue
		}
		if sheetID == "" && sheet.Hidden {
			continue
		}
		targets = append(targets, sheet)
	}
	if sheetID != "" && len(targets) == 0 {
		return nil, fmt.Errorf("未找到工作表: %s", sheetID)
	}

	result := make([]*SheetData, 0, len(targets))
	for _, sheet := range targets {
		values, err := readSheetValuesForMarkdown(ctx, spreadsheetToken, sheet, userAccessToken)
		if err != nil {
			return nil, fmt.Errorf("读取工作表 %q 失败: %w", sheet.Title, err)
		}
		result = append(result, &SheetData{
			Title:  sheet.Title,
			Values: values,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可导出的工作表数据")
	}

	return result, nil
}

func readSheetValuesForMarkdown(ctx context.Context, spreadsheetToken string, sheet *client.SheetInfo, userAccessToken string) ([][]any, error) {
	if sheet.RowCount <= 0 || sheet.ColCount <= 0 {
		return nil, nil
	}

	chunks, err := buildSheetReadChunks(sheet.SheetID, sheet.RowCount, sheet.ColCount)
	if err != nil {
		return nil, err
	}

	values := make([][]any, sheet.RowCount)
	for _, chunk := range chunks {
		cellRange, err := client.ReadCells(ctx, spreadsheetToken, chunk.Range, "", "", userAccessToken)
		if err != nil {
			return nil, err
		}
		if cellRange == nil {
			continue
		}
		mergeSheetChunkValues(values, chunk.StartRow, chunk.StartCol, cellRange.Values)
	}
	return values, nil
}

type sheetReadChunk struct {
	Range    string
	StartRow int
	StartCol int
}

func buildSheetReadChunks(sheetID string, rowCount, colCount int) ([]sheetReadChunk, error) {
	if rowCount <= 0 || colCount <= 0 {
		return nil, nil
	}
	if rowCount*colCount > sheetMarkdownMaxReadCells {
		return nil, fmt.Errorf("网格大小 %d 行 × %d 列超过 Markdown 导出上限 %d 个单元格，请缩小工作表或按范围读取", rowCount, colCount, sheetMarkdownMaxReadCells)
	}

	var chunks []sheetReadChunk
	firstColSpan := minInt(colCount, sheetMarkdownReadChunkCells)
	rowSpan := maxInt(1, sheetMarkdownReadChunkCells/firstColSpan)
	for rowStart := 1; rowStart <= rowCount; {
		rowEnd := minInt(rowCount, rowStart+rowSpan-1)
		for colStart := 1; colStart <= colCount; {
			colSpan := minInt(colCount-colStart+1, sheetMarkdownReadChunkCells)
			colEnd := colStart + colSpan - 1
			chunks = append(chunks, sheetReadChunk{
				Range:    fmt.Sprintf("%s!%s%d:%s%d", sheetID, sheetColIndexToLetter(colStart), rowStart, sheetColIndexToLetter(colEnd), rowEnd),
				StartRow: rowStart,
				StartCol: colStart,
			})
			colStart = colEnd + 1
		}

		rowStart = rowEnd + 1
	}
	return chunks, nil
}

func mergeSheetChunkValues(dest [][]any, startRow, startCol int, values [][]any) {
	for r, row := range values {
		rowIdx := startRow - 1 + r
		if rowIdx < 0 || rowIdx >= len(dest) {
			break
		}
		needLen := startCol - 1 + len(row)
		if len(dest[rowIdx]) < needLen {
			next := make([]any, needLen)
			copy(next, dest[rowIdx])
			dest[rowIdx] = next
		}
		copy(dest[rowIdx][startCol-1:], row)
	}
}

func sheetColIndexToLetter(col int) string {
	var result string
	for col > 0 {
		col--
		result = string(rune('A'+col%26)) + result
		col /= 26
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// writeRow 写入一行 Markdown 表格
func writeRow(sb *strings.Builder, row []any, maxCols int) {
	sb.WriteString("|")
	for c := 0; c < maxCols; c++ {
		sb.WriteString(" ")
		if c < len(row) {
			sb.WriteString(cellToMarkdown(row[c]))
		}
		sb.WriteString(" |")
	}
	sb.WriteString("\n")
}

// cellToMarkdown 将单元格值转为 Markdown 文本
func cellToMarkdown(cell any) string {
	if cell == nil {
		return ""
	}

	switch v := cell.(type) {
	case string:
		return escapeMDTableCell(v)
	case float64:
		// 整数不带小数点
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case []any:
		// 富文本数组（mention / attachment / text 混合）
		return escapeMDTableCell(richTextArrayToMarkdown(v))
	case map[string]any:
		// 单个富文本元素
		return escapeMDTableCell(richTextElementToMarkdown(v))
	default:
		return escapeMDTableCell(fmt.Sprintf("%v", v))
	}
}

// richTextArrayToMarkdown 将富文本数组转为 Markdown
func richTextArrayToMarkdown(elements []any) string {
	var parts []string
	for _, elem := range elements {
		m, ok := elem.(map[string]any)
		if !ok {
			parts = append(parts, fmt.Sprintf("%v", elem))
			continue
		}
		text := richTextElementToMarkdown(m)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

// richTextElementToMarkdown 将单个富文本元素转为 Markdown
func richTextElementToMarkdown(m map[string]any) string {
	elemType, _ := m["type"].(string)

	switch elemType {
	case "text":
		text, _ := m["text"].(string)
		return text
	case "mention":
		// @文档/幻灯片等引用
		text, _ := m["text"].(string)
		link, _ := m["link"].(string)
		if link != "" && text != "" {
			return fmt.Sprintf("[%s](%s)", text, link)
		}
		return text
	case "attachment":
		text, _ := m["text"].(string)
		if text == "" {
			text = "附件"
		}
		return fmt.Sprintf("📎 %s", text)
	default:
		// 尝试通用处理：有 link+text 就做链接，否则取 text
		text, _ := m["text"].(string)
		link, _ := m["link"].(string)
		if link != "" && text != "" {
			return fmt.Sprintf("[%s](%s)", text, link)
		}
		if text != "" {
			return text
		}
		return ""
	}
}

// escapeMDTableCell 转义 Markdown 表格中的特殊字符
func escapeMDTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return strings.TrimSpace(s)
}

// trimEmptyRows 去除尾部的全空行
func trimEmptyRows(rows [][]any) [][]any {
	last := len(rows)
	for last > 0 {
		if !isRowEmpty(rows[last-1]) {
			break
		}
		last--
	}
	return rows[:last]
}

// trimEmptyCols 返回裁剪尾部全空列后的列数
func trimEmptyCols(rows [][]any, maxCols int) int {
	for maxCols > 0 {
		allEmpty := true
		for _, row := range rows {
			if maxCols-1 < len(row) && row[maxCols-1] != nil {
				if s, ok := row[maxCols-1].(string); ok && s == "" {
					continue
				}
				allEmpty = false
				break
			}
		}
		if !allEmpty {
			break
		}
		maxCols--
	}
	return maxCols
}

// isRowEmpty 判断行是否全空
func isRowEmpty(row []any) bool {
	for _, cell := range row {
		if cell == nil {
			continue
		}
		if s, ok := cell.(string); ok && s == "" {
			continue
		}
		return false
	}
	return true
}
