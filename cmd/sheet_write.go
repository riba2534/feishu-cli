package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetWriteCmd = &cobra.Command{
	Use:   "write <spreadsheet_token|url> <range>",
	Short: "写入单元格数据",
	Long: `写入数据到电子表格的指定范围。

数据格式（JSON 二维数组）:
  [["A1值", "B1值"], ["A2值", "B2值"]]

数字按原始字面量写入（不经 float64，1000000 不会变成 1e+06）；布尔值写为 "TRUE"/"FALSE"。

` + sheetRangeFormatHelp + `

自动分批:
  接口单次最多写 5000 行、100 列。数据超限时以范围左上角为锚点自动拆成多个请求依次写入；
  范围显式声明的行/列数小于数据时报错（只写左上角单元格如 "0b12!A1" 即可让 CLI 自动计算）。

示例:
  # 通过命令行参数传入数据
  feishu-cli sheet write shtcnxxxxxx "0b12!A1:B2" --data '[["姓名", "年龄"], ["张三", 25]]'

  # 子表名作前缀
  feishu-cli sheet write shtcnxxxxxx "Sheet1!A1:B2" --data '[["姓名", "年龄"], ["张三", 25]]'

  # 从文件读取数据（超过 5000 行自动分批）
  feishu-cli sheet write shtcnxxxxxx "0b12!A1" --data-file data.json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		rangeStr, err := target.qualifyRange(args[1], sheetID, sheetName)
		if err != nil {
			return err
		}

		chunks, err := client.PlanSheetWriteChunks(rangeStr, values, 0, 0)
		if err != nil {
			return clierr.Usage(err)
		}
		if len(chunks) > 1 {
			fmt.Fprintf(os.Stderr, "数据 %d 行 × %d 列超过单次写入上限（%d 行 / %d 列），分 %d 批写入\n",
				len(values), client.MaxRowWidth(values), client.SheetV2MaxRowsPerWrite, client.SheetV2MaxColsPerWrite, len(chunks))
		}
		written := make([]string, 0, len(chunks))
		for i, chunk := range chunks {
			res, err := client.WriteCells(client.Context(), target.Token, chunk.Range, chunk.Values, target.UAT)
			if err != nil {
				if len(chunks) > 1 {
					return fmt.Errorf("第 %d/%d 批（%s）写入失败，之前的 %d 批已写入: %w", i+1, len(chunks), chunk.Range, i, err)
				}
				return err
			}
			written = append(written, res.Range)
		}
		result := &client.CellRange{Range: client.SheetRangesSpan(written), Values: values}

		if output == "json" {
			if len(chunks) > 1 {
				return printJSON(map[string]any{"range": result.Range, "values": values, "batches": len(chunks)})
			}
			return printJSON(result)
		}
		fmt.Printf("写入成功！\n")
		fmt.Printf("  更新范围: %s\n", result.Range)
		fmt.Printf("  写入行数: %d\n", len(values))
		if len(chunks) > 1 {
			fmt.Printf("  分批次数: %d\n", len(chunks))
		}
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetWriteCmd)

	sheetWriteCmd.Flags().String("sheet-id", "", "工作表 ID（如果范围中未指定）")
	addSheetNameFlag(sheetWriteCmd)
	sheetWriteCmd.Flags().StringP("data", "d", "", "要写入的数据（JSON 二维数组）")
	sheetWriteCmd.Flags().String("data-file", "", "数据文件路径")
	sheetWriteCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetWriteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
