package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	textpkg "github.com/yuin/goldmark/text"
)

var sheetImportMDCmd = &cobra.Command{
	Use:   "import-md <file.md>",
	Short: "从 Markdown 表格创建电子表格",
	Long: `从 Markdown 文件中提取 GFM 表格，并以表格内容创建一个新的飞书电子表格。

工作流程:
  1. 读取本地 .md 文件，提取第 N 张 GFM 表格（默认第 0 张）
  2. POST /sheets/v3/spreadsheets         创建空电子表格
  3. GET  /sheets/v3/spreadsheets/{tok}/sheets/query   拿默认 sheet_id
  4. PUT  /sheets/v2/spreadsheets/{tok}/values         把表格数据写到 A1
  5. 打印新建电子表格的 URL

适用场景:
  把 Markdown 报告/笔记里的数据表格一键变成可在线编辑、筛选、排序的飞书电子表格。

支持的 Markdown 表格语法（GFM）:
  | 列1 | 列2 | 列3 |
  | --- | :---: | ---: |    ← 对齐符号会被忽略，只用作分隔行识别
  | 值A | 值B | 值C |
  | 值D | 值E | 值F |

  也兼容无前导/尾部竖线的写法：
  列1 | 列2
  --- | ---
  值A | 值B

特性:
  - 默认提取文件里第一张 GFM 表格；多表场景用 --table-index 选第几张
  - 单元格内 \| 会被正确识别为字面竖线
  - 不规则行长会按最长行 pad 空字符串

示例:
  # 用文件名作标题，导入第一张表
  feishu-cli sheet import-md report.md

  # 自定义标题 + 指定文件夹
  feishu-cli sheet import-md report.md --title "Q1 销售数据" -f fldcnxxxxxx

  # 文件里有多张表，导第二张（0-based）
  feishu-cli sheet import-md report.md --table-index 1

  # 用 user token 导入到个人空间
  feishu-cli sheet import-md report.md --user-access-token <token>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetImportMD(cmd, args, defaultSheetImportMDDeps())
	},
}

type sheetImportMDDeps struct {
	validate          func() error
	resolveUserToken  func(*cobra.Command) string
	createSpreadsheet func(context.Context, string, string, string) (*client.SpreadsheetInfo, error)
	querySheets       func(context.Context, string, string) ([]*client.SheetInfo, error)
	writeCells        func(context.Context, string, string, [][]any, string) (*client.CellRange, error)
}

func defaultSheetImportMDDeps() sheetImportMDDeps {
	return sheetImportMDDeps{
		validate:         config.Validate,
		resolveUserToken: resolveOptionalUserTokenWithFallback,
		createSpreadsheet: func(ctx context.Context, title, folderToken, userAccessToken string) (*client.SpreadsheetInfo, error) {
			return client.CreateSpreadsheet(ctx, title, folderToken, userAccessToken)
		},
		querySheets: func(ctx context.Context, spreadsheetToken, userAccessToken string) ([]*client.SheetInfo, error) {
			return client.QuerySheets(ctx, spreadsheetToken, userAccessToken)
		},
		writeCells: func(ctx context.Context, spreadsheetToken, rangeStr string, values [][]any, userAccessToken string) (*client.CellRange, error) {
			return client.WriteCells(ctx, spreadsheetToken, rangeStr, values, userAccessToken)
		},
	}
}

func runSheetImportMD(cmd *cobra.Command, args []string, deps sheetImportMDDeps) error {
	if err := deps.validate(); err != nil {
		return err
	}

	mdPath := args[0]
	if !strings.HasSuffix(strings.ToLower(mdPath), ".md") && !strings.HasSuffix(strings.ToLower(mdPath), ".markdown") {
		fmt.Fprintf(cmd.ErrOrStderr(), "提示: 文件扩展名不是 .md/.markdown，仍按 Markdown 解析\n")
	}

	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	tableIndex, _ := cmd.Flags().GetInt("table-index")
	title, _ := cmd.Flags().GetString("title")
	folderToken, _ := cmd.Flags().GetString("folder")
	if cmd.Flags().Changed("folder-token") {
		folderToken, _ = cmd.Flags().GetString("folder-token")
	}
	output, _ := cmd.Flags().GetString("output")

	if title == "" {
		base := filepath.Base(mdPath)
		ext := filepath.Ext(base)
		title = strings.TrimSuffix(base, ext)
		if title == "" {
			title = "Markdown 导入"
		}
	}

	// 1. 解析 markdown，挑出第 N 张 GFM 表格
	tables := extractGFMTables(string(raw))
	if len(tables) == 0 {
		return fmt.Errorf("文件里找不到任何 GFM 表格: %s", mdPath)
	}
	if tableIndex < 0 || tableIndex >= len(tables) {
		return fmt.Errorf("--table-index %d 超出范围（文件里共 %d 张表，0-based）", tableIndex, len(tables))
	}
	rows := tables[tableIndex]
	if len(rows) == 0 {
		return fmt.Errorf("第 %d 张表是空的", tableIndex)
	}
	if err := validateSheetImportMDSize(rows); err != nil {
		return err
	}

	sheetImportMDProgress(cmd, output, "解析到表格 #%d：%d 行 × %d 列\n", tableIndex, len(rows), len(rows[0]))

	userAccessToken := deps.resolveUserToken(cmd)
	ctx := client.Context()

	// 2. 创建空电子表格
	sheetImportMDProgress(cmd, output, "正在创建电子表格 %q ...\n", title)
	info, err := deps.createSpreadsheet(ctx, title, folderToken, userAccessToken)
	if err != nil {
		return err
	}

	// 3. 拿默认 sheet_id
	sheets, err := deps.querySheets(ctx, info.SpreadsheetToken, userAccessToken)
	if err != nil {
		return fmt.Errorf("查询工作表列表失败: %w", err)
	}
	if len(sheets) == 0 {
		return fmt.Errorf("新建电子表格没有默认工作表（不应该发生）")
	}
	sheetID := sheets[0].SheetID

	// 4. 写入数据到 A1
	colCount := len(rows[0])
	rowCount := len(rows)
	rangeStr := fmt.Sprintf("%s!A1:%s%d", sheetID, colIndexToLetter(colCount), rowCount)

	values := make([][]any, rowCount)
	for i, row := range rows {
		values[i] = make([]any, colCount)
		for j := 0; j < colCount; j++ {
			if j < len(row) {
				values[i][j] = row[j]
			} else {
				values[i][j] = ""
			}
		}
	}

	sheetImportMDProgress(cmd, output, "正在写入 %s ...\n", rangeStr)
	if _, err := deps.writeCells(ctx, info.SpreadsheetToken, rangeStr, values, userAccessToken); err != nil {
		return err
	}

	// 5. 输出
	if output == "json" {
		return printJSON(sheetImportMDResult{
			SpreadsheetToken: info.SpreadsheetToken,
			Title:            info.Title,
			URL:              info.URL,
			Rows:             rowCount,
			Cols:             colCount,
			TableIndex:       tableIndex,
			SourceFile:       mdPath,
		})
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== 导入完成 ===")
	fmt.Fprintf(out, "  Token: %s\n", info.SpreadsheetToken)
	fmt.Fprintf(out, "  标题: %s\n", info.Title)
	fmt.Fprintf(out, "  URL: %s\n", info.URL)
	fmt.Fprintf(out, "  数据: %d 行 × %d 列（来自 %s 第 %d 张表）\n", rowCount, colCount, mdPath, tableIndex)
	return nil
}

func sheetImportMDProgress(cmd *cobra.Command, output, format string, args ...any) {
	out := cmd.OutOrStdout()
	if output == "json" {
		out = cmd.ErrOrStderr()
	}
	fmt.Fprintf(out, format, args...)
}

// sheetImportMDResult 是 --output json 模式下的稳定输出 schema。
type sheetImportMDResult struct {
	SpreadsheetToken string `json:"spreadsheet_token"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	Rows             int    `json:"rows"`
	Cols             int    `json:"cols"`
	TableIndex       int    `json:"table_index"`
	SourceFile       string `json:"source_file"`
}

const maxSheetImportMDCells = 5000

func validateSheetImportMDSize(rows [][]string) error {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil
	}
	cells := len(rows) * len(rows[0])
	if cells > maxSheetImportMDCells {
		return fmt.Errorf("表格单次写入单元格数 %d（%d 行 × %d 列）超过飞书 API 上限 %d，请拆分", cells, len(rows), len(rows[0]), maxSheetImportMDCells)
	}
	return nil
}

// extractGFMTables 从 Markdown 文本中解析所有 GFM 表格，按出现顺序返回。
func extractGFMTables(text string) [][][]string {
	var tables [][][]string

	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	source := []byte(text)
	doc := md.Parser().Parse(textpkg.NewReader(source))

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		table, ok := n.(*east.Table)
		if !ok {
			return ast.WalkContinue, nil
		}

		rows := tableToRows(table, source)
		if len(rows) > 0 {
			tables = append(tables, rows)
		}
		return ast.WalkSkipChildren, nil
	})

	return tables
}

func tableToRows(table *east.Table, source []byte) [][]string {
	var rows [][]string
	colCount := len(table.Alignments)
	if colCount == 0 {
		return nil
	}

	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		switch r := row.(type) {
		case *east.TableHeader:
			if r.ChildCount() != colCount {
				return nil
			}
			rows = append(rows, tableRowToCells(r, source, colCount))
		case *east.TableRow:
			rows = append(rows, tableRowToCells(r, source, colCount))
		}
	}
	return rows
}

func tableRowToCells(row ast.Node, source []byte, colCount int) []string {
	cells := make([]string, 0, colCount)
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if tc, ok := cell.(*east.TableCell); ok {
			cells = append(cells, strings.TrimSpace(tableCellText(tc, source)))
		}
	}
	return padRow(cells, colCount)
}

func tableCellText(node ast.Node, source []byte) string {
	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			buf.WriteString(unescapeMDTableCellText(string(n.Segment.Value(source))))
		case *ast.String:
			buf.Write(n.Value)
		case *ast.RawHTML:
			raw := strings.TrimSpace(strings.ToLower(rawHTMLText(n, source)))
			if raw == "<br>" || raw == "<br/>" || raw == "<br />" {
				buf.WriteByte('\n')
			}
		default:
			buf.WriteString(tableCellText(child, source))
		}
	}
	return buf.String()
}

func rawHTMLText(node *ast.RawHTML, source []byte) string {
	var buf bytes.Buffer
	for i := 0; i < node.Segments.Len(); i++ {
		segment := node.Segments.At(i)
		buf.Write(segment.Value(source))
	}
	return buf.String()
}

func unescapeMDTableCellText(s string) string {
	replacer := strings.NewReplacer(`\|`, "|", `\\`, `\`)
	return replacer.Replace(s)
}

// looksLikeTableLine 行里至少一个 "|"（已 unescape \|）且非空。
func looksLikeTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// 至少有一个非转义的 |
	for k := 0; k < len(trimmed); k++ {
		if trimmed[k] == '|' && (k == 0 || trimmed[k-1] != '\\') {
			return true
		}
	}
	return false
}

// isSeparatorLine GFM 分隔线：每个 cell 是 :?-+:?（允许对齐冒号），
// 至少一个 "-"，cell 之间用 | 分隔。
func isSeparatorLine(line string) bool {
	cells := splitTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if !isSeparatorCell(c) {
			return false
		}
	}
	return true
}

func isSeparatorCell(c string) bool {
	if c == "" {
		return false
	}
	// 形如 ---  :---  ---:  :---: 都允许
	start := 0
	end := len(c)
	if c[start] == ':' {
		start++
	}
	if end > start && c[end-1] == ':' {
		end--
	}
	if end-start < 1 {
		return false
	}
	for k := start; k < end; k++ {
		if c[k] != '-' {
			return false
		}
	}
	return true
}

// splitTableRow 把一行 GFM 表格按 "|" 切成 cell 数组，处理：
//   - 前导/尾部竖线可有可无
//   - 单元格内 \| 是字面竖线
//   - 单元格内容首尾空白会被 trim
func splitTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	// 去前导/尾部 |（尾部要排除 \|，那是字面竖线不是分隔符）
	trimmed = strings.TrimPrefix(trimmed, "|")
	if strings.HasSuffix(trimmed, "|") && !strings.HasSuffix(trimmed, "\\|") {
		trimmed = trimmed[:len(trimmed)-1]
	}

	var cells []string
	var buf strings.Builder
	for k := 0; k < len(trimmed); k++ {
		ch := trimmed[k]
		if ch == '\\' && k+1 < len(trimmed) && trimmed[k+1] == '|' {
			buf.WriteByte('|')
			k++
			continue
		}
		if ch == '|' {
			cells = append(cells, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	cells = append(cells, strings.TrimSpace(buf.String()))
	return cells
}

// padRow 把行扩展到 colCount 长（短的补 ""，长的截断）。
func padRow(row []string, colCount int) []string {
	out := make([]string, colCount)
	for i := 0; i < colCount; i++ {
		if i < len(row) {
			out[i] = row[i]
		}
	}
	return out
}

func init() {
	sheetCmd.AddCommand(sheetImportMDCmd)
	sheetImportMDCmd.Flags().StringP("title", "t", "", "电子表格标题（默认用文件名去后缀）")
	sheetImportMDCmd.Flags().StringP("folder", "f", "", "目标文件夹 Token（可选）")
	sheetImportMDCmd.Flags().String("folder-token", "", "目标文件夹 Token（兼容旧参数，建议使用 --folder）")
	sheetImportMDCmd.Flags().Int("table-index", 0, "选第几张 GFM 表格（0-based，默认第 0 张）")
	sheetImportMDCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImportMDCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
	_ = sheetImportMDCmd.Flags().MarkDeprecated("folder-token", "请改用 --folder")
}
