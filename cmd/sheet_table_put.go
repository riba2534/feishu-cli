package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var sheetTablePutCmd = &cobra.Command{
	Use:   "table-put <spreadsheet_token|url> <sheet_id>",
	Short: "按列 dtype 类型保真写入整表（V3 typed cell）",
	Long: `按列 dtype 把 DataFrame 形状的 JSON 写入电子表格，保证数字/日期列不被当文本。

核心价值：日期列写 Excel 序列号 + 写后给该列设日期 formatter（yyyy/MM/dd），
飞书识别为「真日期」（可排序/可透视/ISNUMBER=TRUE），而非被当文本。
数字列保持数值类型，文本列用 formatter "@" 防止 ID/邮编等数字串被识别为数字。

输入格式（--sheets 或 --sheets-file，对齐 pandas to_json(orient="split") 形状）:
  {
    "sheets": [
      {
        "name": "可选 sheet 名（写入时以 <sheet_id> 为准）",
        "columns": ["id", "name", "amount", "date"],
        "data": [
          ["A001", "张三", 1200.50, "2024-01-15"],
          ["A002", "李四", 980,    "2024-02-20"]
        ],
        "dtypes":  {"amount": "float64", "date": "datetime64[ns]"},
        "formats": {"amount": "#,##0.00"}
      }
    ]
  }

dtype 映射（缺省/未知 → string）:
  - int*/uint*/float*/complex*   → number（interval* 除外，按文本处理）
  - bool/boolean                 → bool
  - datetime*                    → date（写 Excel 序列号 + 日期 formatter）
  - 其他（object/string 等）      → string（文本格式 @）

写入模式:
  - overwrite（默认）：从 --start-cell（默认 A1）起覆盖写入矩形区域，不清除该区域之外的旧数据
  - append：写到目标列范围内最后一个有数据的行之后（--start-cell 只取列）；默认不写表头，
    空子表首次追加自动写表头

日期时间:
  datetime 列的时分秒保留为序列号小数（2024-01-15T08:30:00 → 45306.354…），带时间的列自动用
  yyyy/MM/dd HH:mm:ss 格式；table-get 读出的日期时间写回后不丢时间。
  number / date 列中的空字符串按空单元格写入；number 列中恰为数字字面量的字符串（"100.5"）按数字写入。

使用限制:
  - 当前仅支持单 sheet 一次写入（payload 含多 sheet 会报错，请逐个写入）
  - 子表行列不足时自动追加行/列
  - 单批 ≤ 5000 单元格，超出自动按行分批

示例:
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets-file table.json
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets '{"sheets":[{"columns":["x"],"data":[[1]],"dtypes":{"x":"int64"}}]}'
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets-file table.json --header=false
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets-file more.json --mode append
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets-file table.json --start-cell C3 -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		sheetID := args[1]
		sheetsJSON, _ := cmd.Flags().GetString("sheets")
		sheetsFile, _ := cmd.Flags().GetString("sheets-file")
		header, _ := cmd.Flags().GetBool("header")
		headerExplicit := cmd.Flags().Changed("header")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		mode, _ := cmd.Flags().GetString("mode")
		startCell, _ := cmd.Flags().GetString("start-cell")
		output, _ := cmd.Flags().GetString("output")

		mode = strings.ToLower(strings.TrimSpace(mode))
		if mode != "overwrite" && mode != "append" {
			return clierr.Usagef("--mode 仅支持 overwrite / append，得到 %q", mode)
		}
		col0, row0, err := parseTablePutStartCell(startCell)
		if err != nil {
			return err
		}
		if mode == "append" && !headerExplicit {
			// 追加模式默认不重复写表头（与官方一致）；目标子表为空时下面会补写表头
			header = false
		}

		raw, err := loadJSONInput(sheetsJSON, sheetsFile, "sheets", "sheets-file", "表格数据")
		if err != nil {
			return err
		}
		payload, err := client.ParseTablePutPayload([]byte(raw))
		if err != nil {
			return clierr.Usage(err)
		}
		if len(payload.Sheets) > 1 {
			return clierr.Usagef("table-put 当前仅支持单 sheet（payload 含 %d 个，请逐个写入）", len(payload.Sheets))
		}
		spec := payload.Sheets[0]
		numCols := len(spec.Columns)
		if numCols < 1 {
			return clierr.Usagef("columns 为空")
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT
		gridRows, gridCols := tablePutGridSize(target, sheetID)

		// 追加模式：从目标列范围内最后一个有数据的行之后开始写（--start-cell 的行号被忽略，列号保留）
		baseRow := row0
		if mode == "append" {
			lastRow, err := tablePutLastDataRow(target, sheetID, col0, numCols, gridRows)
			if err != nil {
				return fmt.Errorf("定位追加位置失败: %w", err)
			}
			baseRow = lastRow
			if lastRow == 0 && !headerExplicit {
				// 空子表首次追加：写表头，避免 table-get 把第一行数据当成列名
				header = true
			}
		}

		// 构造元素矩阵（行 → 列 → 元素数组）。BuildTypedCell 对空值返回空文本元素，
		// 每格恒有一个元素（V3 不接受空元素数组，否则整批写入 500）。
		var rows [][][]*client.CellElement
		if header {
			hdr := make([][]*client.CellElement, numCols)
			for c, col := range spec.Columns {
				hdr[c] = []*client.CellElement{{Type: "text", Text: &client.TextElement{Text: col.Name}}}
			}
			rows = append(rows, hdr)
		}
		for ri, row := range spec.Rows {
			cells := make([][]*client.CellElement, numCols)
			for c := 0; c < numCols; c++ {
				cell, err := client.BuildTypedCell(spec.Columns[c], row[c])
				if err != nil {
					return clierr.Usagef("构造单元格失败（数据行 %d 列 %q）: %v", ri+1, spec.Columns[c].Name, err)
				}
				cells[c] = []*client.CellElement{cell}
			}
			rows = append(rows, cells)
		}
		if len(rows) == 0 {
			return clierr.Usagef("无数据可写入（data 为空且未写表头）")
		}

		// 子表网格不够大时自动扩容（写入超出网格会被接口拒绝）
		if err := tablePutEnsureGrid(spreadsheetToken, sheetID, userAccessToken, gridRows, gridCols, baseRow+len(rows), col0+numCols); err != nil {
			return err
		}

		// 前置 formatter：V3 单元格元素不支持 cell_styles，须先用 V2 style 接口给各列数据区
		// 设 formatter，再写值。顺序很关键——若先写值，飞书后端会对 text 元素做类型推断
		// （如 "007" 被存成数字 7、前导零丢失），formatter 后置只改显示、无法挽回；
		// 先设 @ / 日期 formatter 再写值，数字串才按文本保真、序列号才渲染为真日期（已实测）。
		dataStartRow := baseRow + 1
		if header {
			dataStartRow = baseRow + 2 // 表头占一行
		}
		dataEndRow := baseRow + len(rows)
		if dataEndRow >= dataStartRow {
			for c, col := range spec.Columns {
				formatter := client.FormatterForType(col)
				if formatter == "" {
					continue
				}
				colL := client.IndexToColumn(col0 + c)
				styleRange := fmt.Sprintf("%s!%s%d:%s%d", sheetID, colL, dataStartRow, colL, dataEndRow)
				style := &client.CellStyle{Formatter: formatter}
				if err := client.SetCellStyle(client.Context(), spreadsheetToken, styleRange, style, userAccessToken); err != nil {
					return fmt.Errorf("设置列 %q 格式（%s）失败: %w", col.Name, formatter, err)
				}
			}
		}

		// 分批写入（V3 单批 ≤ 5000 cell）
		const maxCellsPerWrite = 5000
		batchRows := maxCellsPerWrite / numCols
		if batchRows < 1 {
			batchRows = 1
		}
		startColLetter := client.IndexToColumn(col0)
		lastColLetter := client.IndexToColumn(col0 + numCols - 1)
		batches := 0
		for start := 0; start < len(rows); start += batchRows {
			end := start + batchRows
			if end > len(rows) {
				end = len(rows)
			}
			rng := fmt.Sprintf("%s!%s%d:%s%d", sheetID, startColLetter, baseRow+start+1, lastColLetter, baseRow+end)
			vr := &client.ValueRangeV3{Range: rng, Values: rows[start:end]}
			if err := client.WriteCellsV3(client.Context(), spreadsheetToken, sheetID, []*client.ValueRangeV3{vr}, userIDType, userAccessToken); err != nil {
				return fmt.Errorf("写入第 %d-%d 行失败: %w", baseRow+start+1, baseRow+end, err)
			}
			batches++
		}

		fullRange := fmt.Sprintf("%s!%s%d:%s%d", sheetID, startColLetter, baseRow+1, lastColLetter, baseRow+len(rows))
		if output == "json" {
			return printJSON(map[string]any{
				"spreadsheet_token": spreadsheetToken,
				"sheet_id":          sheetID,
				"range":             fullRange,
				"mode":              mode,
				"header":            header,
				"columns":           numCols,
				"data_rows":         len(spec.Rows),
				"writes":            batches,
			})
		}
		fmt.Printf("table-put 完成：sheet=%s，%d 列 × %d 行（含表头 %v）已写入 %s（模式 %s）\n", sheetID, numCols, len(rows), header, fullRange, mode)
		return nil
	},
}

// parseTablePutStartCell 解析 --start-cell（单个单元格，如 A1、C5），返回 0 起始的列与行。
func parseTablePutStartCell(cell string) (col0, row0 int, err error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return 0, 0, nil
	}
	b, ok := client.ParseA1Bounds(cell)
	if !ok || b.StartCol < 0 || b.StartRow < 0 || b.EndCol != b.StartCol || b.EndRow != b.StartRow {
		return 0, 0, clierr.Usagef("--start-cell 必须是单个单元格（如 A1、C5），得到 %q", cell)
	}
	return b.StartCol, b.StartRow, nil
}

// tablePutGridSize 返回子表当前网格行列数；获取失败时返回 0（跳过自动扩容，保持旧行为）。
func tablePutGridSize(target *sheetTarget, sheetID string) (rows, cols int) {
	sheets, err := target.listSheets()
	if err != nil {
		return 0, 0
	}
	for _, s := range sheets {
		if s.SheetID == sheetID {
			return s.RowCount, s.ColCount
		}
	}
	return 0, 0
}

// tablePutLastDataRow 返回目标列范围内最后一个有数据的行号（1 起始；空表返回 0）。
// 按 5000 行一段读取整个网格，避免中间有空行时把追加位置算到空行处、覆盖下方数据。
func tablePutLastDataRow(target *sheetTarget, sheetID string, col0, ncols, gridRows int) (int, error) {
	startCol := client.IndexToColumn(col0)
	endCol := client.IndexToColumn(col0 + ncols - 1)
	if gridRows <= 0 {
		// 网格未知：读整列
		rng := fmt.Sprintf("%s!%s:%s", sheetID, startCol, endCol)
		cr, err := client.ReadCells(client.Context(), target.Token, rng, "", "", target.UAT)
		if err != nil {
			return 0, err
		}
		return lastNonEmptyRow(cr, 0), nil
	}
	last := 0
	const chunk = 5000
	for start := 1; start <= gridRows; start += chunk {
		end := start + chunk - 1
		if end > gridRows {
			end = gridRows
		}
		rng := fmt.Sprintf("%s!%s%d:%s%d", sheetID, startCol, start, endCol, end)
		cr, err := client.ReadCells(client.Context(), target.Token, rng, "", "", target.UAT)
		if err != nil {
			return 0, err
		}
		if r := lastNonEmptyRow(cr, start-1); r > 0 {
			last = r
		}
	}
	return last, nil
}

// lastNonEmptyRow 返回 cr 中最后一个含非空单元格的行（1 起始，加上 offset）；全空返回 0。
func lastNonEmptyRow(cr *client.CellRange, offset int) int {
	if cr == nil {
		return 0
	}
	for i := len(cr.Values) - 1; i >= 0; i-- {
		for _, v := range cr.Values[i] {
			if v == nil {
				continue
			}
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			return offset + i + 1
		}
	}
	return 0
}

// tablePutEnsureGrid 在写入范围超出子表网格时追加行/列（单次最多 5000）。网格未知（0）时跳过。
func tablePutEnsureGrid(token, sheetID, uat string, gridRows, gridCols, needRows, needCols int) error {
	grow := func(major string, have, need int) error {
		if have <= 0 || need <= have {
			return nil
		}
		missing := need - have
		fmt.Fprintf(os.Stderr, "子表网格 %d %s不足，自动追加 %d %s\n", have, dimLabel(major), missing, dimLabel(major))
		for missing > 0 {
			n := missing
			if n > 5000 {
				n = 5000
			}
			if err := client.AddDimension(client.Context(), token, sheetID, major, n, uat); err != nil {
				return fmt.Errorf("自动扩容子表失败: %w", err)
			}
			missing -= n
		}
		return nil
	}
	if err := grow("ROWS", gridRows, needRows); err != nil {
		return err
	}
	return grow("COLUMNS", gridCols, needCols)
}

func init() {
	sheetCmd.AddCommand(sheetTablePutCmd)
	sheetTablePutCmd.Flags().String("sheets", "", "表格数据 JSON（与 --sheets-file 二选一）")
	sheetTablePutCmd.Flags().String("sheets-file", "", "表格数据 JSON 文件路径")
	sheetTablePutCmd.Flags().Bool("header", true, "首行写入列名（overwrite 默认开启；append 默认关闭，空子表首次追加自动写表头）")
	sheetTablePutCmd.Flags().String("mode", "overwrite", "写入模式: overwrite（从 --start-cell 起覆盖）, append（写到已有数据最后一行之后）")
	sheetTablePutCmd.Flags().String("start-cell", "A1", "写入起点单元格（append 模式只取其列）")
	sheetTablePutCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetTablePutCmd.Flags().String("user-id-type", "", "用户 ID 类型: open_id, union_id, user_id")
	sheetTablePutCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
