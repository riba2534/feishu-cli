package cmd

import (
	"github.com/spf13/cobra"
)

// mailCmd 邮件命令组
var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "飞书邮箱（Mail）操作命令",
	Long: `飞书邮箱（Mail）操作，通过 OAuth User Access Token 访问（读类命令支持 --as bot 读取指定邮箱）。

子命令:
  message        获取单封邮件（正文自动解码为明文）
  messages       批量获取多封邮件
  thread         获取线程（按时间排序）
  triage         列出/搜索邮件摘要（folder/label/query/unread-only，--max 自动翻页）
  send           发送邮件（默认保存草稿，加 --confirm-send 直接发送；支持 --attach）
  draft-create   创建草稿（不发送）
  draft-edit     编辑已有草稿（只改传入字段，保留回复头、附件与引用块）
  reply          回复邮件（Re: 前缀、引用块、In-Reply-To；优先 Reply-To）
  reply-all      全部回复（包含 To 和 CC，排除自己）
  forward        转发邮件（携带原附件）
  draft-send     发送已有草稿（--confirm-send 保护）
  message-modify 批量加/删 label、移动文件夹（超过 20 封自动分批）
  message-trash  批量软删进废纸篓（--yes 跳过确认，可移回）
  thread-modify  批量整理线程（加/删 label、移动文件夹）
  thread-trash   批量软删线程（需确认）
  rule-*         收信规则：rule-list/rule-get/rule-create/rule-update/rule-enable/rule-disable/rule-delete/rule-reorder
  signature      查看邮箱签名
  template       邮件模板 create/list/get/update/delete

权限要求（User Access Token）:
  - mail:user_mailbox:readonly
  - mail:user_mailbox.message:readonly
  - mail:user_mailbox.message.body:read
  - mail:user_mailbox.message.address:read
  - mail:user_mailbox.message.subject:read
  - mail:user_mailbox.message:send
  - mail:user_mailbox.message:modify
  - mail:user_mailbox.rule:read / mail:user_mailbox.rule:write（收信规则）

示例:
  feishu-cli mail triage --folder inbox --unread-only
  feishu-cli mail message --message-id xxx
  feishu-cli mail send --to user@example.com --subject "测试" --body "hi" --confirm-send
  feishu-cli mail reply --message-id xxx --body "收到"`,
}

func init() {
	rootCmd.AddCommand(mailCmd)
}
