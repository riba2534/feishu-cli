package cmd

import (
	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "评论操作命令",
	Long: `文档评论操作命令，包括列出评论、添加评论、获取评论详情、解决/取消解决评论、回复管理等。

子命令:
  list        列出文档评论
  add         添加评论
  get         获取评论详情
  delete      删除评论
  resolve     标记评论为已解决
  unresolve   标记评论为未解决
  reply       评论回复管理（list/add/delete）

文件类型（--type）:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

身份说明:
  默认以 App Token（租户身份）请求；若文档归个人所有且 App 未被加为协作者，
  会得到 1069303 forbidden。此时通过 --user-access-token 或先 feishu-cli auth login，
  让请求以用户身份发出。

示例:
  # 列出文档评论
  feishu-cli comment list <file_token> --type docx

  # 列出个人文档的评论（用户身份；--user-access-token 也可省略，让 FEISHU_USER_ACCESS_TOKEN 接管）
  feishu-cli auth login
  feishu-cli comment list <file_token> --type docx --user-access-token "u-xxxxx"

  # 添加评论
  feishu-cli comment add <file_token> --type docx --text "这是一条评论"

  # 解决评论
  feishu-cli comment resolve <file_token> <comment_id> --type docx

  # 取消解决评论
  feishu-cli comment unresolve <file_token> <comment_id> --type docx

  # 列出评论回复
  feishu-cli comment reply list <file_token> <comment_id> --type docx

  # 添加评论回复（推荐登录后以用户身份发布）
  feishu-cli comment reply add <file_token> <comment_id> --text "回复内容"

  # 删除评论回复（飞书只允许回复作者删除，需 User Token）
  feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx`,
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.PersistentFlags().String("user-access-token", "", "User Access Token（可选，传入后所有 comment 子命令以用户身份发起请求；个人文档场景必填）")
}
