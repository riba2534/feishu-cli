package cmd

import (
	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "评论操作命令",
	Long: `文档评论操作命令，包括列出评论、添加评论、获取评论详情、删除评论等。

子命令:
  list      列出文档评论
  add       添加评论
  get       获取评论详情
  delete    删除评论

文件类型（--type）:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

示例:
  # 列出文档评论
  feishu-cli comment list <file_token> --type docx

  # 添加评论
  feishu-cli comment add <file_token> --type docx --text "这是一条评论"

  # 获取评论详情
  feishu-cli comment get <file_token> <comment_id> --type docx

  # 删除评论
  feishu-cli comment delete <file_token> <comment_id> --type docx`,
}

func init() {
	rootCmd.AddCommand(commentCmd)
}
