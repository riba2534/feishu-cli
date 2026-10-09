package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetClearCmd = &cobra.Command{
	Use:   "clear <spreadsheet_token|url> <sheet_id> <range1> [range2...]",
	Short: "清除单元格内容（V3 API）",
	Long: `使用 V3 API 清除单元格内容，保留原有样式。

范围格式:
  <sheetId>!A1:B2  - 指定工作表的范围（也可用子表名作前缀）
  A1:B2            - 不带前缀时自动补上 <sheet_id>

使用限制:
  - 单次传入的 range 数量不得超过 10 个

示例:
  # 清除单个范围
  feishu-cli sheet clear shtcnxxxxxx 0b12 "0b12!A1:B3"

  # 清除多个范围
  feishu-cli sheet clear shtcnxxxxxx 0b12 "0b12!A1:A10" "0b12!C1:C10"`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]

		// 检查范围数量限制
		if len(args[2:]) > 10 {
			return clierr.Usagef("单次最多只能清除 10 个范围，当前传入 %d 个", len(args[2:]))
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		ranges, err := target.qualifyRanges(args[2:], sheetID)
		if err != nil {
			return err
		}

		if err := client.ClearCellsV3(client.Context(), target.Token, sheetID, ranges, target.UAT); err != nil {
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

	sheetClearCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
