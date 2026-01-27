package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetReadPlainCmd = &cobra.Command{
	Use:   "read-plain <spreadsheet_token> <sheet_id> <range1> [range2...]",
	Short: "获取纯文本内容（V3 API）",
	Long: `使用 V3 API 批量获取工作表的纯文本内容。

范围格式:
  SheetID!A1:B2    - 指定工作表的范围
  A1:B2            - 当前工作表的范围

特点:
  - 支持批量获取多个范围
  - 返回纯文本内容，@提及等会被转换为文本

示例:
  # 获取单个范围
  feishu-cli sheet read-plain shtcnxxxxxx 0b12 "0b12!A1:C10"

  # 获取多个范围
  feishu-cli sheet read-plain shtcnxxxxxx 0b12 "0b12!A1:A1" "0b12!G2:G2"`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		ranges := args[2:]
		outputFormat, _ := cmd.Flags().GetString("output")

		// 处理 shell 转义
		for i := range ranges {
			ranges[i] = unescapeSheetRange(ranges[i])
		}

		result, err := client.ReadCellsPlainV3(client.Context(), spreadsheetToken, sheetID, ranges)
		if err != nil {
			return err
		}

		if outputFormat == "json" {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
		} else {
			for _, cellRange := range result {
				fmt.Printf("范围: %s\n", cellRange.Range)
				if len(cellRange.Values) == 0 {
					fmt.Println("  （空数据）")
					continue
				}
				for i, row := range cellRange.Values {
					rowStrs := make([]string, len(row))
					for j, cell := range row {
						rowStrs[j] = fmt.Sprintf("%v", cell)
					}
					fmt.Printf("  [%d] %s\n", i+1, strings.Join(rowStrs, " | "))
				}
				fmt.Println()
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetReadPlainCmd)

	sheetReadPlainCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
}
