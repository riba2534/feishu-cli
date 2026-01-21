package cmd

import (
	"github.com/spf13/cobra"
)

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "云空间文件管理命令",
	Long: `云空间文件管理命令，包括列出文件、创建文件夹、移动、复制、删除等操作。

子命令:
  list      列出文件夹中的文件
  mkdir     创建文件夹
  move      移动文件或文件夹
  copy      复制文件
  delete    删除文件或文件夹
  shortcut  创建文件快捷方式
  quota     查询云空间容量

文件类型（type）:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件
  folder    文件夹
  slides    幻灯片

示例:
  # 列出根目录文件
  feishu-cli file list

  # 列出指定文件夹
  feishu-cli file list <folder_token>

  # 创建文件夹
  feishu-cli file mkdir "新文件夹" --parent <folder_token>

  # 移动文件
  feishu-cli file move <file_token> --target <folder_token> --type docx

  # 复制文件
  feishu-cli file copy <file_token> --target <folder_token> --type docx

  # 删除文件
  feishu-cli file delete <file_token> --type docx

  # 创建快捷方式
  feishu-cli file shortcut <file_token> --target <folder_token> --type docx

  # 查询云空间容量
  feishu-cli file quota`,
}

func init() {
	rootCmd.AddCommand(fileCmd)
}
