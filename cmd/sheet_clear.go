package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetClearCmd = &cobra.Command{
	Use:   "clear <spreadsheet_token> <sheet_id> <range1> [range2...]",
	Short: "清除单元格内容（V3 API）",
	Long: `使用 V3 API 清除单元格内容，保留原有样式。

范围格式:
  SheetID!A1:B2    - 指定工作表的范围

使用限制:
  - 单次传入的 range 数量不得超过 10 个

示例:
  # 清除单个范围
  feishu-cli sheet clear shtcnxxxxxx 0b12 "0b12!A1:B3"

  # 清除多个范围
  feishu-cli sheet clear shtcnxxxxxx 0b12 "0b12!A1:A10" "0b12!C1:C10"`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		ranges := args[2:]

		// 处理 shell 转义
		for i := range ranges {
			ranges[i] = unescapeSheetRange(ranges[i])
		}

		// 检查范围数量限制
		if len(ranges) > 10 {
			return fmt.Errorf("单次最多只能清除 10 个范围，当前传入 %d 个", len(ranges))
		}

		err := client.ClearCellsV3(client.Context(), spreadsheetToken, sheetID, ranges)
		if err != nil {
			return err
		}

		fmt.Printf("清除成功！\n")
		fmt.Printf("  工作表: %s\n", sheetID)
		fmt.Printf("  清除范围: %v\n", ranges)

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetClearCmd)
}
