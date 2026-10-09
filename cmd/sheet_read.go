package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

// sheetRangeFormatHelp 是 v2 范围类命令共用的范围格式说明。
const sheetRangeFormatHelp = `范围格式:
  <sheetId>!A1:C10   - 指定子表的范围（sheetId 见 list-sheets 或 URL ?sheet=）
  <子表名>!A1:C10     - 也可以用子表名作前缀（如 Sheet1!A1:C10），自动换算为 sheetId
  A1:C10             - 不带前缀时依次取 --sheet-id / --sheet-name / URL ?sheet= / 唯一子表
  <sheetId>!A:C      - 整列
  <sheetId>!1:3      - 整行`

var sheetReadCmd = &cobra.Command{
	Use:   "read <spreadsheet_token|url> <range>",
	Short: "读取单元格数据",
	Long: `读取电子表格中指定范围的单元格数据。

` + sheetRangeFormatHelp + `

数字按原始精度输出（不经 float64，19 位整数、1000000 等不会变成科学计数法）。

示例:
  feishu-cli sheet read shtcnxxxxxx "0b12!A1:C10"
  feishu-cli sheet read shtcnxxxxxx "Sheet1!A1:C10"
  feishu-cli sheet read shtcnxxxxxx "A1:C10" --sheet-id 0b12
  feishu-cli sheet read "https://xxx.feishu.cn/sheets/shtcnxxxxxx?sheet=0b12" "A1:C10"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		valueRenderOption, _ := cmd.Flags().GetString("value-render")
		dateTimeRenderOption, _ := cmd.Flags().GetString("datetime-render")
		output, _ := cmd.Flags().GetString("output")

		sheetID, sheetName, err := sheetSelectorFlags(cmd)
		if err != nil {
			return err
		}
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		rangeStr, err := target.qualifyRange(args[1], sheetID, sheetName)
		if err != nil {
			return err
		}

		cellRange, err := client.ReadCells(client.Context(), target.Token, rangeStr, valueRenderOption, dateTimeRenderOption, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(cellRange); err != nil {
				return err
			}
		} else {
			fmt.Printf("范围: %s\n", cellRange.Range)
			if len(cellRange.Values) == 0 {
				fmt.Println("（空数据）")
				return nil
			}
			fmt.Println("数据:")
			for i, row := range cellRange.Values {
				rowStrs := make([]string, len(row))
				for j, cell := range row {
					rowStrs[j] = fmt.Sprintf("%v", cell)
				}
				fmt.Printf("  [%d] %s\n", i+1, strings.Join(rowStrs, " | "))
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetReadCmd)

	sheetReadCmd.Flags().String("sheet-id", "", "工作表 ID（如果范围中未指定）")
	addSheetNameFlag(sheetReadCmd)
	sheetReadCmd.Flags().String("value-render", "", "值渲染选项: ToString, FormattedValue, Formula, UnformattedValue")
	sheetReadCmd.Flags().String("datetime-render", "", "日期时间渲染选项: FormattedString")
	sheetReadCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetReadCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
