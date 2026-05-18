package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailTemplateCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建邮件模板（personal mail template）",
	Long: `创建一个个人邮件模板。注意 mail send 当前没有 --template-id 引用 flag（飞书 API 暂未支持），模板用于以后查询/复用，不影响立即可读取的 template_id 输出。

必填:
  --name        模板名（≤ 100 字符）

可选:
  --subject     默认主题
  --body        默认正文（含 HTML/纯文本；不会扫内嵌图片 — 模板内嵌图请走专用流程）
  --plain-text  纯文本模式（is_plain_text_mode=true）
  --to          默认收件人列表（逗号分隔）
  --cc          默认抄送
  --bcc         默认密送
  --mailbox     邮箱 ID（默认 me）

权限:
  - User Access Token
  - mail:user_mailbox.message:modify / mail:user_mailbox:readonly
  - mail:user.email.template（字节租户暂未开放）

示例:
  feishu-cli mail template create --name "周报" --subject "本周进度" \
      --body "<p>这是模板</p>" --to lead@example.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail template create")
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("--name 必填")
		}
		if len([]rune(name)) > 100 {
			return fmt.Errorf("--name 长度不能超过 100 个字符")
		}
		subject, _ := cmd.Flags().GetString("subject")
		body, _ := cmd.Flags().GetString("body")
		plainText, _ := cmd.Flags().GetBool("plain-text")
		mailbox, _ := cmd.Flags().GetString("mailbox")
		toRaw, _ := cmd.Flags().GetString("to")
		ccRaw, _ := cmd.Flags().GetString("cc")
		bccRaw, _ := cmd.Flags().GetString("bcc")
		output, _ := cmd.Flags().GetString("output")

		toList, err := parseEmailList(toRaw)
		if err != nil {
			return err
		}
		ccList, err := parseEmailList(ccRaw)
		if err != nil {
			return err
		}
		bccList, err := parseEmailList(bccRaw)
		if err != nil {
			return err
		}

		tpl := &client.MailTemplate{
			Name:            name,
			Subject:         subject,
			TemplateContent: body,
			IsPlainTextMode: plainText,
			Tos:             toMailTemplateAddrs(toList),
			Ccs:             toMailTemplateAddrs(ccList),
			Bccs:            toMailTemplateAddrs(bccList),
		}

		created, err := client.CreateMailTemplate(mailbox, tpl, token)
		if err != nil {
			return fmt.Errorf("创建邮件模板失败: %w", err)
		}

		result := map[string]any{
			"template_id":        created.TemplateID,
			"name":               created.Name,
			"subject":            created.Subject,
			"is_plain_text_mode": created.IsPlainTextMode,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("邮件模板已创建:\n")
		fmt.Printf("  template_id: %s\n", created.TemplateID)
		fmt.Printf("  name:        %s\n", created.Name)
		if created.Subject != "" {
			fmt.Printf("  subject:     %s\n", created.Subject)
		}
		return nil
	},
}

// toMailTemplateAddrs 把 "Name <email>" / "email" 列表转为 MailTemplateAddr
func toMailTemplateAddrs(addrs []string) []client.MailTemplateAddr {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]client.MailTemplateAddr, 0, len(addrs))
	for _, raw := range addrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// "Name <email>" 拆 name / address
		if open := strings.Index(raw, "<"); open >= 0 {
			close := strings.LastIndex(raw, ">")
			if close > open {
				name := strings.TrimSpace(raw[:open])
				addr := strings.TrimSpace(raw[open+1 : close])
				out = append(out, client.MailTemplateAddr{MailAddress: addr, Name: name})
				continue
			}
		}
		out = append(out, client.MailTemplateAddr{MailAddress: raw})
	}
	return out
}

func init() {
	mailTemplateCmd.AddCommand(mailTemplateCreateCmd)
	mailTemplateCreateCmd.Flags().String("name", "", "模板名（必填，≤ 100 字符）")
	mailTemplateCreateCmd.Flags().String("subject", "", "默认主题")
	mailTemplateCreateCmd.Flags().String("body", "", "默认正文（HTML 或纯文本）")
	mailTemplateCreateCmd.Flags().Bool("plain-text", false, "纯文本模式")
	mailTemplateCreateCmd.Flags().String("to", "", "默认收件人，逗号分隔")
	mailTemplateCreateCmd.Flags().String("cc", "", "默认抄送")
	mailTemplateCreateCmd.Flags().String("bcc", "", "默认密送")
	mailTemplateCreateCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailTemplateCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailTemplateCreateCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailTemplateCreateCmd, "name")
}
