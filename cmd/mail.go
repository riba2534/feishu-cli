package cmd

import (
	"github.com/spf13/cobra"
)

// mailCmd 邮件命令组
var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "飞书邮箱（Mail）操作命令",
	Long: `飞书邮箱（Mail）操作，通过 OAuth User Access Token 访问。

⚠️ 首期限制：send/draft/reply/forward 暂不支持普通附件；CID 内联图片走 send --inline-images-auto-scan 自动扫描上传。

子命令:
  message       获取单封邮件内容（含 HTML/纯文本 body）
  messages      批量获取多封邮件
  thread        获取线程（按时间排序）
  triage        列出邮件（支持 folder/label/query/unread-only 过滤）
  send          发送邮件（默认保存草稿，加 --confirm-send 直接发送）
  draft-create  创建草稿（不发送）
  draft-edit    编辑已有草稿
  reply         回复邮件（自动带 Re: 前缀和引用块）
  reply-all     全部回复（包含 To 和 CC）
  forward       转发邮件
  template      邮件模板 CRUD（create/list；字节租户 scope 暂未开放）

权限要求（User Access Token）:
  - mail:user_mailbox:readonly
  - mail:user_mailbox.message:readonly
  - mail:user_mailbox.message.body:read
  - mail:user_mailbox.message.address:read
  - mail:user_mailbox.message.subject:read
  - mail:user_mailbox.message:send
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail triage --folder INBOX --unread-only
  feishu-cli mail message --message-id xxx
  feishu-cli mail send --to user@example.com --subject "测试" --body "hi" --confirm-send
  feishu-cli mail reply --message-id xxx --body "收到"`,
}

func init() {
	rootCmd.AddCommand(mailCmd)
}
