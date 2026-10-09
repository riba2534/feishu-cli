package cmd

import (
	"github.com/spf13/cobra"
)

var sheetDeleteColsCmd = &cobra.Command{
	Use:   "delete-cols <spreadsheet_token|url> <sheet_id>",
	Short: "删除列（不可撤销）",
	Long: `删除指定范围的整列，右侧列整体左移。删除不可撤销，建议先用 --dry-run 确认实际删除范围。

范围写法（二选一）:
  --range "B:D"            B 到 D 列（列字母，两端包含）
  --start 1 --end 4        0 起始（A=0）、--end 不包含：删除 B 到 D 列；省略 --end 时只删 --start 这一列

CLI 会换算成接口口径（删除接口为 1 起始、两端包含），输出与 --dry-run 均显示实际删除的列。

示例:
  feishu-cli sheet delete-cols shtcnxxxxxx 0b12 --range "B:D"
  feishu-cli sheet delete-cols shtcnxxxxxx 0b12 --range "C" --dry-run
  feishu-cli sheet delete-cols shtcnxxxxxx 0b12 --start 0          # 删除 A 列`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetDeleteDimension(cmd, args, "COLUMNS")
	},
}

func init() {
	sheetCmd.AddCommand(sheetDeleteColsCmd)

	sheetDeleteColsCmd.Flags().String("range", "", "列区间，如 \"B:D\" 或 \"C\"（两端包含）")
	sheetDeleteColsCmd.Flags().Int("start", 0, "起始列号（从 0 开始，A=0）")
	sheetDeleteColsCmd.Flags().Int("end", 0, "结束列号（不包含；缺省只删除 1 列）")
	sheetDeleteColsCmd.Flags().Bool("dry-run", false, "只打印将要删除的实际列与请求，不执行")
	sheetDeleteColsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
