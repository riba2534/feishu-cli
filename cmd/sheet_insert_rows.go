package cmd

import (
	"github.com/spf13/cobra"
)

var sheetInsertRowsCmd = &cobra.Command{
	Use:   "insert-rows <spreadsheet_token|url> <sheet_id>",
	Short: "插入行",
	Long: `在指定位置插入空白行，下方原有行整体下移。

位置写法（二选一）:
  --range "3:4"            插入后新行所在的行号（1 起始、两端包含）：在原第 3 行之前插入 2 行
  --start 2 --end 4        0 起始、--end 不包含：同样在原第 3 行之前插入 2 行；省略 --end 时插入 1 行

示例:
  feishu-cli sheet insert-rows shtcnxxxxxx 0b12 --range "3:4"
  feishu-cli sheet insert-rows shtcnxxxxxx 0b12 --start 0 --inherit-style AFTER   # 在最上方插入 1 行`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetInsertDimension(cmd, args, "ROWS")
	},
}

func init() {
	sheetCmd.AddCommand(sheetInsertRowsCmd)

	sheetInsertRowsCmd.Flags().String("range", "", "插入后新行所在位置（如 \"3:4\"，1 起始、两端包含）")
	sheetInsertRowsCmd.Flags().Int("start", 0, "起始行号（从 0 开始）")
	sheetInsertRowsCmd.Flags().Int("end", 0, "结束行号（不包含；缺省插入 1 行）")
	sheetInsertRowsCmd.Flags().String("inherit-style", "", "继承样式: BEFORE（上方行）, AFTER（下方行）")
	sheetInsertRowsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
