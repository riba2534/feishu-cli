package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailForwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "转发邮件（带 Fwd: 前缀 + 原文）",
	Long: `转发一封邮件到新的收件人。自动：
  1. subject 加 "Fwd: " 前缀
  2. body 带原邮件的 from/subject/date 和原文

⚠️ 首期限制: 暂不支持转发原邮件的附件。

必填:
  --message-id   要转发的邮件 ID
  --to           新收件人

可选:
  --cc / --bcc / --body（前置评论）
  --confirm-send 保存草稿后立即发送

示例:
  feishu-cli mail forward --message-id msg_xxx --to user@example.com
  feishu-cli mail forward --message-id msg_xxx --to team@example.com --body "请关注此邮件"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail forward")
		if err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		messageID, _ := cmd.Flags().GetString("message-id")
		toRaw, _ := cmd.Flags().GetString("to")
		ccRaw, _ := cmd.Flags().GetString("cc")
		bccRaw, _ := cmd.Flags().GetString("bcc")
		comment, _ := cmd.Flags().GetString("body")
		confirmSend, _ := cmd.Flags().GetBool("confirm-send")
		output, _ := cmd.Flags().GetString("output")

		if messageID == "" {
			return fmt.Errorf("--message-id 必填")
		}
		to, err := parseEmailList(toRaw)
		if err != nil {
			return err
		}
		if len(to) == 0 {
			return fmt.Errorf("--to 至少一个收件人")
		}
		cc, err := parseEmailList(ccRaw)
		if err != nil {
			return err
		}
		bcc, err := parseEmailList(bccRaw)
		if err != nil {
			return err
		}

		// 获取原邮件
		origData, err := client.GetMailMessage(mailbox, messageID, "full", token)
		if err != nil {
			return fmt.Errorf("获取原邮件失败: %w", err)
		}
		var orig struct {
			Message struct {
				Subject  string `json:"subject"`
				HeadFrom struct {
					MailAddress string `json:"mail_address"`
					Name        string `json:"name"`
				} `json:"head_from"`
				Date          int64  `json:"date"`
				BodyPlainText string `json:"body_plain_text"`
			} `json:"message"`
		}
		if err := json.Unmarshal(origData, &orig); err != nil {
			return err
		}
		if orig.Message.Subject == "" {
			_ = json.Unmarshal(origData, &orig.Message)
		}
		origMsg := orig.Message

		subject := ensureForwardSubject(origMsg.Subject)

		// 构造转发 body
		fwdBody := comment
		fwdBody += "\n\n---------- 转发邮件 ----------\n"
		fwdBody += fmt.Sprintf("发件人: %s <%s>\n", origMsg.HeadFrom.Name, origMsg.HeadFrom.MailAddress)
		fwdBody += fmt.Sprintf("主题: %s\n\n", origMsg.Subject)
		fwdBody += origMsg.BodyPlainText

		// 发件人
		var from, fromName string
		if profile, perr := client.GetMailboxProfile(mailbox, token); perr == nil && profile != nil {
			from = profile.PrimaryEmailAddress
			fromName = profile.Name
		}

		input := mailMessageInput{
			From:     from,
			FromName: fromName,
			To:       to,
			CC:       cc,
			BCC:      bcc,
			Subject:  subject,
			BodyText: fwdBody,
		}

		rawB64, err := buildEMLBase64URL(input)
		if err != nil {
			return err
		}

		draftID, err := client.CreateMailDraft(mailbox, rawB64, token)
		if err != nil {
			return err
		}

		if !confirmSend {
			result := map[string]any{"draft_id": draftID, "confirmed": false}
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("转发草稿已保存: %s\n", draftID)
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
			"draft_id":   draftID,
			"message_id": sent.MessageID,
			"thread_id":  sent.ThreadID,
			"confirmed":  true,
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
	mustMarkFlagRequired(mailForwardCmd, "message-id", "to")
}
