package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailDraftCreateCmd = &cobra.Command{
	Use:   "draft-create",
	Short: "创建邮件草稿（不发送）",
	Long: `创建邮件草稿，不发送。与 mail send 的区别：
- mail send      默认行为就是创建草稿，加 --confirm-send 才发送
- mail draft-create  仅创建，不会发送

参数同 mail send（含 --attach 附件；不支持 --inline-images-auto-scan 与 --confirm-send）。

示例:
  feishu-cli mail draft-create --to user@example.com --subject "草稿" --body "内容"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail draft-create")
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
		forceHTML, _ := cmd.Flags().GetBool("html")
		plainText, _ := cmd.Flags().GetBool("plain-text")
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
		attachments, err := loadMailAttachmentsFromFlags(cmd)
		if err != nil {
			return err
		}

		if from == "" {
			if profile, perr := client.GetMailboxProfile(mailbox, token); perr == nil && profile != nil {
				from = profile.PrimaryEmailAddress
				if fromName == "" {
					fromName = profile.Name
				}
			}
		}

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
		input.Attachments = attachments
		if isHTML {
			input.BodyHTML = body
		} else {
			input.BodyText = body
		}

		rawB64, err := buildEMLBase64URL(input)
		if err != nil {
			return err
		}

		draftID, err := client.CreateMailDraft(mailbox, rawB64, token)
		if err != nil {
			return err
		}

		result := map[string]any{"draft_id": draftID}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("草稿创建成功: %s\n", draftID)
		return nil
	},
}

func init() {
	mailCmd.AddCommand(mailDraftCreateCmd)
	mailDraftCreateCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailDraftCreateCmd.Flags().String("to", "", "收件人，逗号分隔（必填）")
	mailDraftCreateCmd.Flags().String("cc", "", "抄送")
	mailDraftCreateCmd.Flags().String("bcc", "", "密送")
	mailDraftCreateCmd.Flags().String("subject", "", "主题（必填）")
	mailDraftCreateCmd.Flags().String("body", "", "正文（必填）")
	mailDraftCreateCmd.Flags().String("from", "", "发件人地址（默认从 profile 获取）")
	mailDraftCreateCmd.Flags().String("from-name", "", "发件人显示名")
	mailDraftCreateCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailDraftCreateCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailDraftCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailDraftCreateCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addMailAttachFlag(mailDraftCreateCmd)
	mustMarkFlagRequired(mailDraftCreateCmd, "to", "subject", "body")
}
