package cmd

import (
	"github.com/spf13/cobra"
)

// mailTemplateCmd 邮件模板命令组（template create/list 等）
var mailTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "邮件模板（templates）操作",
	Long: `飞书邮箱个人邮件模板（template）管理。

子命令:
  create   创建邮件模板
  list     列出当前邮箱下的全部模板

权限要求（User Access Token）:
  - mail:user_mailbox:readonly
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail template create --name "周报" --subject "本周进度" --body "..."
  feishu-cli mail template list`,
}

func init() {
	mailCmd.AddCommand(mailTemplateCmd)
}
