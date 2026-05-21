package cmd

import (
	"github.com/spf13/cobra"
)

// sheetCmd represents the sheet command group
var sheetCmd = &cobra.Command{
	Use:   "sheet",
	Short: "电子表格操作",
	Long:  "电子表格操作命令组，包括创建、读写、工作表管理、筛选视图管理（filter-view）和下拉菜单设置（dropdown）等功能",
}

func init() {
	rootCmd.AddCommand(sheetCmd)
}
