package cmd

import (
	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "评论操作命令",
	Long: `文档评论操作命令，包括列出评论、添加评论、获取评论详情、解决/取消解决评论、回复管理等。

子命令:
  list        列出文档评论（含正文与回复；--solved-status/--comment-scope 过滤；--page-all 翻页）
  add         添加全文评论（纯文本）
  get         获取单条评论详情（含正文与回复）
  batch-get   按评论 ID 批量获取评论（单次最多 100 个）
  delete      删除评论（飞书不支持整条删除，改用 reply delete / resolve）
  resolve     标记评论为已解决
  unresolve   标记评论为未解决
  reply       评论回复管理（list/add/update/react/delete）

  富文本 / 局部 / 单元格 / 幻灯片 / 多维表格记录评论用 feishu-cli drive add-comment。

文件类型（--type）:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  file      云盘文件
  slides    幻灯片

身份说明:
  list/get/batch-get/add/resolve/unresolve/reply list 默认优先 auth login 的 User Token，
  不可用时告警并回退 App Token；reply add/update/react/delete 默认 App/Bot 身份
  （回复只能由作者身份修改/删除），仅显式 --user-access-token（或 FEISHU_USER_ACCESS_TOKEN）时切换为用户。
  若文档归个人所有且 App 未被加为协作者，App 身份会得到 1069303 forbidden。

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

  # 只看未解决的评论，拉全量
  feishu-cli comment list <file_token> --type docx --solved-status false --page-all

  # 获取单条评论
  feishu-cli comment get <file_token> <comment_id> --type docx

  # 列出评论回复
  feishu-cli comment reply list <file_token> <comment_id> --type docx

  # 修改回复 / 表情回应
  feishu-cli comment reply update <file_token> <comment_id> <reply_id> --text "已修改"
  feishu-cli comment reply react <file_token> <reply_id> --emoji THUMBSUP

  # 添加评论回复（默认同一 App 的 Bot 身份；用户身份需显式传 User Token）
  feishu-cli comment reply add <file_token> <comment_id> --text "回复内容"

  # 删除评论回复（身份必须匹配作者：Bot 回复默认同一 App，用户回复显式传 User Token）
  feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx`,
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.PersistentFlags().String("user-access-token", "", "User Access Token（可选，传入后所有 comment 子命令以用户身份发起请求；个人文档场景必填）")
}
