package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var mailForwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "转发邮件（带 Fwd: 前缀 + 原文 + 原附件）",
	Long: `转发一封邮件到新的收件人。自动：
  1. subject 加 "Fwd: " 前缀（已有 Fwd:/Fw:/转发： 则不重复）
  2. body 带原邮件的发件人/时间/主题/收件人和原文（正文 base64url 自动解码；HTML 模式下全部转义）
  3. 默认携带原邮件的普通附件（超大附件为云文档卡片，不随 EML 携带，会在 stderr 提示）
  4. In-Reply-To 写原邮件 <smtp_message_id>，X-LMS-Reply-To-Message-Id 写原邮件 message_id

必填:
  --message-id   要转发的邮件 ID
  --to           新收件人

可选:
  --cc / --bcc / --body（前置附言）
  --html / --plain-text          强制正文类型（默认按 --body 内容自动检测，纯文本附言走纯文本）
  --attach                       追加本地附件（可重复或逗号分隔）
  --no-original-attachments      不携带原邮件附件
  --confirm-send                 保存草稿后立即发送

示例:
  feishu-cli mail forward --message-id msg_xxx --to user@example.com
  feishu-cli mail forward --message-id msg_xxx --to team@example.com --body "请关注此邮件"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		messageID, _ := cmd.Flags().GetString("message-id")
		toRaw, _ := cmd.Flags().GetString("to")
		ccRaw, _ := cmd.Flags().GetString("cc")
		bccRaw, _ := cmd.Flags().GetString("bcc")
		comment, _ := cmd.Flags().GetString("body")
		confirmSend, _ := cmd.Flags().GetBool("confirm-send")
		forceHTML, _ := cmd.Flags().GetBool("html")
		plainText, _ := cmd.Flags().GetBool("plain-text")
		noOrigAtts, _ := cmd.Flags().GetBool("no-original-attachments")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(messageID) == "" {
			return clierr.Usagef("--message-id 必填")
		}
		if forceHTML && plainText {
			return clierr.Usagef("--html 与 --plain-text 互斥，只能指定其中一个")
		}
		to, err := parseEmailList(toRaw)
		if err != nil {
			return clierr.Usage(err)
		}
		if len(to) == 0 {
			return clierr.Usagef("--to 至少一个收件人")
		}
		cc, err := parseEmailList(ccRaw)
		if err != nil {
			return clierr.Usage(err)
		}
		bcc, err := parseEmailList(bccRaw)
		if err != nil {
			return clierr.Usage(err)
		}
		extraAtts, err := loadMailAttachmentsFromFlags(cmd)
		if err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "mail forward")
		if err != nil {
			return err
		}

		// 获取原邮件（正文已解码）
		src, err := fetchMailComposeSource(mailbox, messageID, token)
		if err != nil {
			return err
		}

		// 原邮件附件：下载后随转发携带（超大附件/无下载链接的给出提示）
		var attachments []mailAttachmentPart
		if !noOrigAtts {
			origAtts, skipped, aerr := fetchMailForwardAttachments(mailbox, firstNonEmptyString(src.MessageID, messageID), src.Attachments, token)
			if aerr != nil {
				return aerr
			}
			for _, sk := range skipped {
				fmt.Fprintf(os.Stderr, "提示: 原附件 %s\n", sk)
			}
			attachments = append(attachments, origAtts...)
		}
		attachments = append(attachments, extraAtts...)
		var total int64
		for _, a := range attachments {
			total += estimateBase64Size(int64(len(a.Bytes)))
		}
		if total > mailMaxEMLBytes {
			return clierr.Usagef("原附件与追加附件编码后总大小超过 25MB 上限；可加 --no-original-attachments 或减少 --attach")
		}

		// 发件人
		var from, fromName string
		if profile, perr := client.GetMailboxProfile(mailbox, token); perr == nil && profile != nil {
			from = profile.PrimaryEmailAddress
			fromName = profile.Name
		}

		isHTML := forceHTML
		if !forceHTML && !plainText {
			isHTML = detectHTMLBody(comment)
		}
		input := mailMessageInput{
			From:                from,
			FromName:            fromName,
			To:                  to,
			CC:                  cc,
			BCC:                 bcc,
			Subject:             ensureForwardSubject(src.Subject),
			InReplyTo:           src.SMTPMessageID,
			LMSReplyToMessageID: firstNonEmptyString(src.MessageID, messageID),
			Attachments:         attachments,
		}
		if isHTML {
			input.BodyHTML = comment + buildMailHTMLForwardQuote(src)
		} else {
			input.BodyText = buildMailPlainForward(src, comment)
		}

		rawB64, err := buildEMLBase64URL(input)
		if err != nil {
			return err
		}

		draftID, err := client.CreateMailDraft(mailbox, rawB64, token)
		if err != nil {
			return err
		}

		attNames := make([]string, 0, len(attachments))
		for _, a := range attachments {
			attNames = append(attNames, a.Filename)
		}

		if !confirmSend {
			result := map[string]any{"draft_id": draftID, "confirmed": false, "attachments": attNames}
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("转发草稿已保存: %s\n", draftID)
			if len(attNames) > 0 {
				fmt.Printf("  附件(%d): %s\n", len(attNames), strings.Join(attNames, ", "))
			}
			return nil
		}

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
			"draft_id":    draftID,
			"message_id":  sent.MessageID,
			"thread_id":   sent.ThreadID,
			"confirmed":   true,
			"attachments": attNames,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("转发发送成功 (message_id=%s)\n", sent.MessageID)
		return nil
	},
}

func init() {
	mailCmd.AddCommand(mailForwardCmd)
	mailForwardCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailForwardCmd.Flags().String("message-id", "", "要转发的邮件 ID（必填）")
	mailForwardCmd.Flags().String("to", "", "新收件人（必填）")
	mailForwardCmd.Flags().String("cc", "", "抄送")
	mailForwardCmd.Flags().String("bcc", "", "密送")
	mailForwardCmd.Flags().String("body", "", "前置评论（可选）")
	mailForwardCmd.Flags().Bool("confirm-send", false, "保存草稿后立即发送")
	mailForwardCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailForwardCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mailForwardCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailForwardCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailForwardCmd.Flags().Bool("no-original-attachments", false, "不携带原邮件的附件")
	addMailAttachFlag(mailForwardCmd)
	mustMarkFlagRequired(mailForwardCmd, "message-id", "to")
}
