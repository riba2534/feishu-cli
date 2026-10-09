package cmd

import (
	"github.com/spf13/cobra"
)

// docTableCmd 是 doc table 子命令组
var docTableCmd = &cobra.Command{
	Use:   "table",
	Short: "文档表格操作",
	Long: `文档内嵌表格操作命令组，支持插入/删除行列、合并/取消合并单元格。

所有操作需要指定文档 ID（或 /docx/、/wiki/ URL）和表格块 ID（Block 类型 31）。

子命令:
  insert-row      插入行
  insert-column   插入列
  delete-rows     删除行
  delete-columns  删除列
  merge-cells     合并单元格
  unmerge-cells   取消合并

示例:
  # 在表格末尾插入一行
  feishu-cli doc table insert-row DOC_ID TABLE_BLOCK_ID --index -1

  # 删除第 2-3 行（左闭右开）
  feishu-cli doc table delete-rows DOC_ID TABLE_BLOCK_ID --start 1 --end 3

  # 合并 A1:C2 区域的单元格
  feishu-cli doc table merge-cells DOC_ID TABLE_BLOCK_ID \
    --row-start 0 --row-end 2 --col-start 0 --col-end 3`,
}

func init() {
	docCmd.AddCommand(docTableCmd)
}
