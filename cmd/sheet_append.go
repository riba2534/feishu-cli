package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetAppendCmd = &cobra.Command{
	Use:   "append <spreadsheet_token|url> <range>",
	Short: "追加数据到表格",
	Long: `在指定范围的最后一行之后追加数据（从范围起始单元格向下找第一个空白行写入）。

数据格式（JSON 二维数组）:
  [["A值", "B值"], ["C值", "D值"]]

数字按原始字面量写入（不经 float64）；布尔值写为 "TRUE"/"FALSE"。

` + sheetRangeFormatHelp + `

自动分批:
  接口单次最多追加 5000 行、100 列。超过 5000 行时自动分批，每一批紧接上一批实际写入位置之后；
  超过 100 列无法分批追加（会错位），请改用 sheet write 指定起始单元格写入。

示例:
  feishu-cli sheet append shtcnxxxxxx "0b12!A:B" --data '[["新行1", "数据1"], ["新行2", "数据2"]]'
  feishu-cli sheet append shtcnxxxxxx "Sheet1!A:B" --data-file rows.json --insert-option INSERT_ROWS`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		insertOption, _ := cmd.Flags().GetString("insert-option")
		output, _ := cmd.Flags().GetString("output")

		sheetID, sheetName, err := sheetSelectorFlags(cmd)
		if err != nil {
			return err
		}
		raw, err := readSheetDataInput(cmd)
		if err != nil {
			return err
		}
		values, err := client.DecodeSheetValues(raw)
		if err != nil {
			return clierr.Usagef("解析数据失败（需要 JSON 二维数组）: %v", err)
		}
		width := client.MaxRowWidth(values)
		if width > client.SheetV2MaxColsPerWrite {
			return clierr.Usagef("追加数据有 %d 列，超过接口单次 %d 列上限且无法分批追加；请改用 sheet write 指定起始单元格写入",
				width, client.SheetV2MaxColsPerWrite)
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		rangeStr, err := target.qualifyRange(args[1], sheetID, sheetName)
		if err != nil {
			return err
		}

		batches := (len(values) + client.SheetV2MaxRowsPerWrite - 1) / client.SheetV2MaxRowsPerWrite
		if batches < 1 {
			batches = 1
		}
		if batches > 1 {
			fmt.Fprintf(os.Stderr, "数据 %d 行超过单次追加上限 %d 行，分 %d 批追加\n", len(values), client.SheetV2MaxRowsPerWrite, batches)
		}
		var written []string
		chunkRange := rangeStr
		for i := 0; i < batches; i++ {
			start := i * client.SheetV2MaxRowsPerWrite
			end := start + client.SheetV2MaxRowsPerWrite
			if end > len(values) {
				end = len(values)
			}
			chunk := values[start:end]
			if i > 0 {
				chunkRange = client.AppendChunkRange(rangeStr, written[len(written)-1], width, len(chunk))
			}
			res, err := client.AppendCells(client.Context(), target.Token, chunkRange, chunk, insertOption, target.UAT)
			if err != nil {
				if batches > 1 {
					return fmt.Errorf("第 %d/%d 批追加失败，之前的 %d 批已写入: %w", i+1, batches, i, err)
				}
				return err
			}
			written = append(written, res.Range)
		}
		result := &client.CellRange{Range: client.SheetRangesSpan(written), Values: values}

		if output == "json" {
			if batches > 1 {
				return printJSON(map[string]any{"range": result.Range, "values": values, "batches": batches})
			}
			return printJSON(result)
		}
		fmt.Printf("追加成功！\n")
		fmt.Printf("  更新范围: %s\n", result.Range)
		fmt.Printf("  追加行数: %d\n", len(values))
		if batches > 1 {
			fmt.Printf("  分批次数: %d\n", batches)
		}
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetAppendCmd)

	sheetAppendCmd.Flags().String("sheet-id", "", "工作表 ID")
	addSheetNameFlag(sheetAppendCmd)
	sheetAppendCmd.Flags().StringP("data", "d", "", "要追加的数据（JSON 二维数组）")
	sheetAppendCmd.Flags().String("data-file", "", "数据文件路径")
	sheetAppendCmd.Flags().String("insert-option", "", "插入选项: OVERWRITE, INSERT_ROWS")
	sheetAppendCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetAppendCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
