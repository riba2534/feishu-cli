package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetReadPlainCmd = &cobra.Command{
	Use:   "read-plain <spreadsheet_token|url> <sheet_id> <range1> [range2...]",
	Short: "获取纯文本内容（V3 API）",
	Long: `使用 V3 API 批量获取工作表的纯文本内容。

范围格式:
  <sheetId>!A1:B2  - 指定工作表的范围（也可用子表名作前缀）
  A1:B2            - 不带前缀时自动补上 <sheet_id>

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
		sheetID := args[1]
		output, _ := cmd.Flags().GetString("output")

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		ranges, err := target.qualifyRanges(args[2:], sheetID)
		if err != nil {
			return err
		}

		result, err := client.ReadCellsPlainV3(client.Context(), target.Token, sheetID, ranges, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
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
	sheetReadPlainCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
