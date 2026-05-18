package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "发送邮件（默认保存草稿，加 --confirm-send 才真的发送）",
	Long: `发送邮件或保存草稿。

工作流:
  1. 默认: 构造 EML 并保存为草稿，返回 draft_id
  2. --confirm-send: 保存草稿后立即发送，返回 message_id

⚠️ 首期限制: 暂不支持普通附件；CID 内联图片走 --inline-images-auto-scan 自动扫描上传。

必填:
  --to        收件人（逗号分隔多个地址，支持 "Name <email>" 或 "email"）
  --subject   主题
  --body      正文（自动检测纯文本/HTML）

可选:
  --cc / --bcc                   抄送/密送
  --from                         发件人地址（默认使用登录账号的 primary_email_address）
  --from-name                    发件人显示名
  --mailbox                      邮箱 ID（默认 me）
  --confirm-send                 保存草稿后立即发送
  --html                         强制视为 HTML（即使不含 HTML 标签）
  --plain-text                   强制视为纯文本
  --inline-images-auto-scan      扫描 HTML 中 <img src="本地路径"> → 上传到飞书云盘 → 改写为 cid: 引用

权限:
  - User Access Token
  - mail:user_mailbox.message:send / mail:user_mailbox.message:modify

示例:
  feishu-cli mail send --to user@example.com --subject "test" --body "hi"                   # 默认草稿
  feishu-cli mail send --to user@example.com --subject "test" --body "hi" --confirm-send    # 立即发送
  feishu-cli mail send --to a@x.com,b@x.com --cc c@x.com --subject "会议" --body "<b>议程</b>"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail send")
		if err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		toRaw, _ := cmd.Flags().GetString("to")
		ccRaw, _ := cmd.Flags().GetString("cc")
		bccRaw, _ := cmd.Flags().GetString("bcc")
		subject, _ := cmd.Flags().GetString("subject")
		body, _ := cmd.Flags().GetString("body")
		from, _ := cmd.Flags().GetString("from")
		fromName, _ := cmd.Flags().GetString("from-name")
		confirmSend, _ := cmd.Flags().GetBool("confirm-send")
		forceHTML, _ := cmd.Flags().GetBool("html")
		plainText, _ := cmd.Flags().GetBool("plain-text")
		autoScanInline, _ := cmd.Flags().GetBool("inline-images-auto-scan")
		output, _ := cmd.Flags().GetString("output")

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

		// 如果没传 --from，从 mailbox profile 获取
		if from == "" {
			profile, perr := client.GetMailboxProfile(mailbox, token)
			if perr == nil && profile != nil {
				from = profile.PrimaryEmailAddress
				if fromName == "" {
					fromName = profile.Name
				}
			}
		}

		// 决定 body 类型
		isHTML := forceHTML
		if !forceHTML && !plainText {
			isHTML = detectHTMLBody(body)
		}

		input := mailMessageInput{
			From:     from,
			FromName: fromName,
			To:       to,
			CC:       cc,
			BCC:      bcc,
			Subject:  subject,
		}
		if isHTML {
			input.BodyHTML = body
		} else {
			input.BodyText = body
		}

		// --inline-images-auto-scan: 扫 <img src="local-path"> → drive 上传 → 改写为 cid:xxx
		// 仅在 HTML body 下生效；纯文本下跳过
		if autoScanInline && input.BodyHTML != "" {
			rewritten, parts, scanErr := scanAndUploadInlineImages(input.BodyHTML, mailbox, token)
			if scanErr != nil {
				return scanErr
			}
			input.BodyHTML = rewritten
			input.InlineImages = parts
		}

		rawB64, err := buildEMLBase64URL(input)
		if err != nil {
			return err
		}

		// 创建草稿
		draftID, err := client.CreateMailDraft(mailbox, rawB64, token)
		if err != nil {
			return fmt.Errorf("创建草稿失败: %w", err)
		}

		if !confirmSend {
			result := map[string]any{
				"draft_id":  draftID,
				"confirmed": false,
				"tip":       "加 --confirm-send 可立即发送。或用 `feishu-cli mail draft-edit` 修改后再发送。",
			}
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("草稿已保存: %s\n", draftID)
			fmt.Printf("提示: 加 --confirm-send 可立即发送\n")
			return nil
		}

		// 发送草稿
		data, err := client.SendMailDraft(mailbox, draftID, token)
		if err != nil {
			return fmt.Errorf("发送草稿失败（草稿已创建 %s）: %w", draftID, err)
		}

		var parsed struct {
			MessageID string `json:"message_id"`
			ThreadID  string `json:"thread_id"`
		}
		_ = json.Unmarshal(data, &parsed)

		result := map[string]any{
			"draft_id":   draftID,
			"message_id": parsed.MessageID,
			"thread_id":  parsed.ThreadID,
			"confirmed":  true,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("邮件发送成功!\n")
		fmt.Printf("  草稿 ID:  %s\n", draftID)
		if parsed.MessageID != "" {
			fmt.Printf("  邮件 ID:  %s\n", parsed.MessageID)
		}
		if parsed.ThreadID != "" {
			fmt.Printf("  线程 ID:  %s\n", parsed.ThreadID)
		}
		return nil
	},
}

func init() {
	mailCmd.AddCommand(mailSendCmd)
	mailSendCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailSendCmd.Flags().String("to", "", "收件人，逗号分隔（必填）")
	mailSendCmd.Flags().String("cc", "", "抄送")
	mailSendCmd.Flags().String("bcc", "", "密送")
	mailSendCmd.Flags().String("subject", "", "主题（必填）")
	mailSendCmd.Flags().String("body", "", "正文（必填）")
	mailSendCmd.Flags().String("from", "", "发件人地址（默认从登录 profile 获取）")
	mailSendCmd.Flags().String("from-name", "", "发件人显示名")
	mailSendCmd.Flags().Bool("confirm-send", false, "保存草稿后立即发送")
	mailSendCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailSendCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailSendCmd.Flags().Bool("inline-images-auto-scan", false, "扫描 HTML body 中 <img src=\"local-path\"> 自动上传飞书云盘并改写为 cid: 引用")
	mailSendCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailSendCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailSendCmd, "to", "subject", "body")
}
