package cmd

import (
	"github.com/spf13/cobra"
)

var sheetDeleteRowsCmd = &cobra.Command{
	Use:   "delete-rows <spreadsheet_token|url> <sheet_id>",
	Short: "删除行（不可撤销）",
	Long: `删除指定范围的整行，下方行整体上移。删除不可撤销，建议先用 --dry-run 确认实际删除范围。

范围写法（二选一）:
  --range "3:5"            第 3 到 5 行（1 起始、两端包含，与表格行号一致）
  --start 2 --end 5        0 起始、--end 不包含：删除第 3 到 5 行；省略 --end 时只删 --start 这一行

CLI 会换算成接口口径（删除接口为 1 起始、两端包含），输出与 --dry-run 均显示实际删除的行号。

示例:
  feishu-cli sheet delete-rows shtcnxxxxxx 0b12 --range "3:5"
  feishu-cli sheet delete-rows shtcnxxxxxx 0b12 --range "3:5" --dry-run
  feishu-cli sheet delete-rows shtcnxxxxxx 0b12 --start 0          # 删除第 1 行`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetDeleteDimension(cmd, args, "ROWS")
	},
}

func init() {
	sheetCmd.AddCommand(sheetDeleteRowsCmd)

	sheetDeleteRowsCmd.Flags().String("range", "", "行区间，如 \"3:5\" 或 \"3\"（1 起始、两端包含）")
	sheetDeleteRowsCmd.Flags().Int("start", 0, "起始行号（从 0 开始）")
	sheetDeleteRowsCmd.Flags().Int("end", 0, "结束行号（不包含；缺省只删除 1 行）")
	sheetDeleteRowsCmd.Flags().Bool("dry-run", false, "只打印将要删除的实际行号与请求，不执行")
	sheetDeleteRowsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
