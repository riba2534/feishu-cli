package cmd

import (
	"github.com/spf13/cobra"
)

// sheetCmd represents the sheet command group
var sheetCmd = &cobra.Command{
	Use:   "sheet",
	Short: "电子表格操作",
	Long: `电子表格操作命令组，包括创建、读写、工作表与行列管理、筛选视图管理（filter-view）和下拉菜单设置（dropdown）等功能。

通用约定:
  <spreadsheet_token>  可传裸 token，也可直接粘贴表格 URL（/sheets/、/spreadsheets/）或知识库 URL（/wiki/，自动换出底层表格）
  范围前缀             "<sheetId>!A1:C10" 与 "<子表名>!A1:C10"（如 "Sheet1!A1:C10"）均可，子表名自动换算为 sheetId
  --as                 bot | user | auto，强制身份；不传时 User 优先、User Token 不可用时告警后回退 Bot`,
}

func init() {
	rootCmd.AddCommand(sheetCmd)
	sheetCmd.PersistentFlags().String("as", "", sheetAsFlagHelp)
}
