package cmd

import (
	"github.com/spf13/cobra"
)

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "云空间文件管理命令",
	Long: `云空间文件管理命令，包括列出文件、上传下载、创建文件夹、移动、复制、删除等操作。

子命令:
  list      列出文件夹中的文件
  download  下载文件
  upload    上传文件
  mkdir     创建文件夹
  move      移动文件或文件夹
  copy      复制文件
  delete    删除文件或文件夹
  shortcut  创建文件快捷方式
  version   文件版本管理（list/create/get/delete）
  meta      批量获取文件元数据
  stats     获取文件统计信息
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

  # 下载文件
  feishu-cli file download <file_token> -o output.pdf

  # 上传文件
  feishu-cli file upload /tmp/report.pdf --parent <folder_token>

  # 创建文件夹
  feishu-cli file mkdir "新文件夹" --parent <folder_token>

  # 移动文件
  feishu-cli file move <file_token> --target <folder_token> --type docx

  # 复制文件
  feishu-cli file copy <file_token> --target <folder_token> --type docx

  # 删除文件
  feishu-cli file delete <file_token> --type docx
  feishu-cli file delete <file_token> --type slides

  # 文件版本管理
  feishu-cli file version list <file_token> --obj-type docx

  # 获取文件元数据
  feishu-cli file meta <token> --doc-type docx

  # 获取文件统计信息
  feishu-cli file stats <file_token> --doc-type docx`,
}

func init() {
	rootCmd.AddCommand(fileCmd)
}
