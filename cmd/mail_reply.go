package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var mailReplyCmd = &cobra.Command{
	Use:   "reply",
	Short: "回复邮件（自动 Re: 前缀 + 引用块）",
	Long: `回复指定邮件。自动：
  1. 获取原邮件的 subject/from/reply_to/body/smtp_message_id（正文 base64url 自动解码）
  2. subject 加 "Re: " 前缀（已有 Re:/回复： 则不重复）
  3. In-Reply-To 写原邮件 <smtp_message_id>，References 继承原链并追加，
     X-LMS-Reply-To-Message-Id 写原邮件 message_id（飞书据此关联会话）
  4. body 自动带原文引用块（发件人/时间/主题/收件人 + 原文）；HTML 模式下引用块全部 HTML 转义
  5. 收件人：原邮件有 Reply-To 时回复到 Reply-To，否则回复原发件人；
     回复自己发出的邮件时改为回复原收件人

必填:
  --message-id   要回复的邮件 ID
  --body         回复正文

可选:
  --confirm-send 保存草稿后立即发送
  --mailbox      默认 me
  --html / --plain-text  强制正文类型（默认按 --body 内容自动检测）
  --attach       附件路径（可重复或逗号分隔；单封邮件附件总计 ≤25MB）

示例:
  feishu-cli mail reply --message-id msg_xxx --body "收到，周三开会"
  feishu-cli mail reply --message-id msg_xxx --body "同意" --confirm-send`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMailReply(cmd, false)
	},
}

var mailReplyAllCmd = &cobra.Command{
	Use:   "reply-all",
	Short: "全部回复（包含 To 和 CC 所有收件人）",
	Long: `全部回复指定邮件，自动包含原邮件的所有 To 和 CC 收件人（排除自己，按邮箱去重）。
回复自己发出的邮件时，保持原邮件的 To / CC 区分。

参数同 mail reply。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMailReply(cmd, true)
	},
}

func runMailReply(cmd *cobra.Command, replyAll bool) error {
	if err := config.Validate(); err != nil {
		return err
	}

	mailbox, _ := cmd.Flags().GetString("mailbox")
	messageID, _ := cmd.Flags().GetString("message-id")
	body, _ := cmd.Flags().GetString("body")
	confirmSend, _ := cmd.Flags().GetBool("confirm-send")
	forceHTML, _ := cmd.Flags().GetBool("html")
	plainText, _ := cmd.Flags().GetBool("plain-text")
	output, _ := cmd.Flags().GetString("output")

	if strings.TrimSpace(messageID) == "" {
		return clierr.Usagef("--message-id 必填")
	}
	if forceHTML && plainText {
		return clierr.Usagef("--html 与 --plain-text 互斥，只能指定其中一个")
	}
	attachments, err := loadMailAttachmentsFromFlags(cmd)
	if err != nil {
		return err
	}

	token, err := requireUserToken(cmd, cmd.CommandPath())
	if err != nil {
		return err
	}

	// Step 1: 获取原邮件（正文已解码）
	src, err := fetchMailComposeSource(mailbox, messageID, token)
	if err != nil {
		return err
	}

	// Step 2: 发件人 profile（selfEmail 用于识别"自己发出的邮件"与 reply-all 排除自己）
	var from, fromName string
	if profile, perr := client.GetMailboxProfile(mailbox, token); perr == nil && profile != nil {
		from = profile.PrimaryEmailAddress
		fromName = profile.Name
	}
	self := mailSelfAddressSet(from, mailbox)

	// Step 3: 收件人（Reply-To 优先；自己发出的邮件回复原收件人）
	to, cc, err := buildMailReplyRecipients(src, self, replyAll)
	if err != nil {
		return err
	}

	// Step 4: 正文 + 引用块（HTML 模式下引用块全部转义）
	isHTML := forceHTML
	if !forceHTML && !plainText {
		isHTML = detectHTMLBody(body)
	}
	input := mailMessageInput{
		From:                from,
		FromName:            fromName,
		To:                  mailAddrStrings(to),
		CC:                  mailAddrStrings(cc),
		Subject:             ensureReplySubject(src.Subject),
		InReplyTo:           src.SMTPMessageID,
		References:          buildMailReplyReferences(src),
		LMSReplyToMessageID: firstNonEmptyString(src.MessageID, messageID),
		Attachments:         attachments,
	}
	if isHTML {
		input.BodyHTML = body + buildMailHTMLReplyQuote(src)
	} else {
		input.BodyText = body + buildMailPlainReplyQuote(src)
	}

	rawB64, err := buildEMLBase64URL(input)
	if err != nil {
		return err
	}

	// Step 5: 保存草稿
	draftID, err := client.CreateMailDraft(mailbox, rawB64, token)
	if err != nil {
		return err
	}

	if !confirmSend {
		result := map[string]any{"draft_id": draftID, "confirmed": false, "to": input.To, "cc": input.CC}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("回复草稿已保存: %s\n", draftID)
		fmt.Printf("  收件人: %s\n", strings.Join(input.To, ", "))
		if len(input.CC) > 0 {
			fmt.Printf("  抄送: %s\n", strings.Join(input.CC, ", "))
		}
		return nil
	}

	// 发送
	sendData, err := client.SendMailDraft(mailbox, draftID, token)
	if err != nil {
		return fmt.Errorf("发送失败（草稿 %s 已创建）: %w", draftID, err)
	}

	var sent struct {
		MessageID string `json:"message_id"`
		ThreadID  string `json:"thread_id"`
	}
	_ = json.Unmarshal(sendData, &sent)

	result := map[string]any{
		"draft_id":   draftID,
		"message_id": sent.MessageID,
		"thread_id":  sent.ThreadID,
		"confirmed":  true,
		"to":         input.To,
		"cc":         input.CC,
	}
	if output == "json" {
		return printJSON(result)
	}
	fmt.Printf("回复发送成功 (message_id=%s)\n", sent.MessageID)
	return nil
}

// firstNonEmptyString 返回第一个非空（去空白后）字符串。
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func init() {
	mailCmd.AddCommand(mailReplyCmd)
	mailReplyCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailReplyCmd.Flags().String("message-id", "", "要回复的邮件 ID（必填）")
	mailReplyCmd.Flags().String("body", "", "回复正文（必填）")
	mailReplyCmd.Flags().Bool("confirm-send", false, "保存草稿后立即发送")
	mailReplyCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailReplyCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailReplyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailReplyCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addMailAttachFlag(mailReplyCmd)
	mustMarkFlagRequired(mailReplyCmd, "message-id", "body")

	mailCmd.AddCommand(mailReplyAllCmd)
	mailReplyAllCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailReplyAllCmd.Flags().String("message-id", "", "要回复的邮件 ID（必填）")
	mailReplyAllCmd.Flags().String("body", "", "回复正文（必填）")
	mailReplyAllCmd.Flags().Bool("confirm-send", false, "保存草稿后立即发送")
	mailReplyAllCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailReplyAllCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailReplyAllCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailReplyAllCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addMailAttachFlag(mailReplyAllCmd)
	mustMarkFlagRequired(mailReplyAllCmd, "message-id", "body")
}
