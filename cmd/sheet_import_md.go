package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
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
	resolveUserToken  func(*cobra.Command) (string, error)
	createSpreadsheet func(context.Context, string, string, string) (*client.SpreadsheetInfo, error)
	querySheets       func(context.Context, string, string) ([]*client.SheetInfo, error)
	writeCells        func(context.Context, string, string, [][]any, string) (*client.CellRange, error)
}

func defaultSheetImportMDDeps() sheetImportMDDeps {
	return sheetImportMDDeps{
		validate:         config.Validate,
		resolveUserToken: resolveSheetUserToken,
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

	raw, err := readLocalInputFile(mdPath)
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

	userAccessToken, err := deps.resolveUserToken(cmd)
	if err != nil {
		return err
	}
	ctx := client.Context()

	// 2. 创建空电子表格
	sheetImportMDProgress(cmd, output, "正在创建电子表格 %q ...\n", title)
	info, err := deps.createSpreadsheet(ctx, title, folderToken, userAccessToken)
	if err != nil {
		return err
	}
	// Bot 身份创建时自动给当前登录用户授 full_access（User 身份创建时跳过）
	grant := autoGrantCurrentUser(userAccessToken, info.SpreadsheetToken, client.ResourceTypeSheet)

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
			PermissionGrant:  grant,
		})
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== 导入完成 ===")
	fmt.Fprintf(out, "  Token: %s\n", info.SpreadsheetToken)
	fmt.Fprintf(out, "  标题: %s\n", info.Title)
	fmt.Fprintf(out, "  URL: %s\n", info.URL)
	fmt.Fprintf(out, "  数据: %d 行 × %d 列（来自 %s 第 %d 张表）\n", rowCount, colCount, mdPath, tableIndex)
	printPermissionGrantText(out, grant)
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
	// PermissionGrant Bot 身份创建时为当前用户自动授权的结果（User 身份创建时省略）
	PermissionGrant *client.PermissionGrantResult `json:"permission_grant,omitempty"`
}

const maxSheetImportMDCells = 5000
const maxSheetImportMDCellChars = 50000

func validateSheetImportMDSize(rows [][]string) error {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil
	}
	cells := len(rows) * len(rows[0])
	if cells > maxSheetImportMDCells {
		return fmt.Errorf("表格单次写入单元格数 %d（%d 行 × %d 列）超过飞书 API 上限 %d，请拆分", cells, len(rows), len(rows[0]), maxSheetImportMDCells)
	}
	for r, row := range rows {
		for c, cell := range row {
			if len([]rune(cell)) > maxSheetImportMDCellChars {
				return fmt.Errorf("单元格 R%dC%d 字符数 %d 超过飞书 API 上限 %d，请拆分或精简内容", r+1, c+1, len([]rune(cell)), maxSheetImportMDCellChars)
			}
		}
	}
	return nil
}

// extractGFMTables 从 Markdown 文本中解析所有 GFM 表格，按出现顺序返回。
func extractGFMTables(text string) [][][]string {
	var tables [][][]string
	lines := strings.Split(text, "\n")
	inFence := false
	var fenceChar byte
	fenceLen := 0

	for i := 0; i < len(lines)-1; i++ {
		line := lines[i]

		if inFence {
			if isClosingMarkdownFence(line, fenceChar, fenceLen) {
				inFence = false
				fenceChar = 0
				fenceLen = 0
			}
			continue
		}

		if ch, n, ok := openingMarkdownFence(line); ok {
			inFence = true
			fenceChar = ch
			fenceLen = n
			continue
		}

		if isIndentedCodeLine(line) || isIndentedCodeLine(lines[i+1]) ||
			!looksLikeTableLine(line) || !isSeparatorLine(lines[i+1]) {
			continue
		}

		header := splitTableRow(line)
		separator := splitTableRow(lines[i+1])
		if len(header) == 0 || len(header) != len(separator) {
			continue
		}

		rows := [][]string{header}
		i += 2
		for i < len(lines) && !isIndentedCodeLine(lines[i]) && looksLikeTableLine(lines[i]) && !isSeparatorLine(lines[i]) {
			rows = append(rows, splitTableRow(lines[i]))
			i++
		}
		i--

		if len(rows) > 0 {
			tables = append(tables, normalizeTableRows(rows))
		}
	}
	return tables
}

func openingMarkdownFence(line string) (byte, int, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return 0, 0, false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	n := countLeadingByte(trimmed, ch)
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

func isClosingMarkdownFence(line string, fenceChar byte, fenceLen int) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < fenceLen {
		return false
	}
	if trimmed[0] != fenceChar {
		return false
	}
	n := countLeadingByte(trimmed, fenceChar)
	return n >= fenceLen && strings.TrimSpace(trimmed[n:]) == ""
}

func countLeadingByte(s string, ch byte) int {
	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	return n
}

func isIndentedCodeLine(line string) bool {
	if strings.HasPrefix(line, "\t") {
		return true
	}
	return len(line)-len(strings.TrimLeft(line, " ")) >= 4
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

func normalizeTableRows(rows [][]string) [][]string {
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i, row := range rows {
		rows[i] = padRow(row, maxCols)
	}
	return rows
}

// padRow 把行扩展到 colCount 长；长行保持原样，避免静默截断数据。
func padRow(row []string, colCount int) []string {
	if colCount <= 0 {
		return []string{}
	}
	if len(row) > colCount {
		return append([]string(nil), row...)
	}
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
