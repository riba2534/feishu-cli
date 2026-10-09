package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// 邮件模板 get / update / delete（client 层已有封装，此前未接命令）。

func formatMailTemplateAddrs(list []client.MailTemplateAddr) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, formatMailAddress(a.Name, a.MailAddress))
	}
	return strings.Join(parts, ", ")
}

var mailTemplateGetCmd = &cobra.Command{
	Use:   "get",
	Short: "查看邮件模板详情",
	Long: `查看个人邮件模板详情（主题、正文、默认收件人、附件）。

示例:
  feishu-cli mail template get --template-id 7642268717035851484
  feishu-cli mail template get --template-id 7642268717035851484 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		templateID, _ := cmd.Flags().GetString("template-id")
		output, _ := cmd.Flags().GetString("output")
		if strings.TrimSpace(templateID) == "" {
			return clierr.Usagef("--template-id 必填")
		}
		token, err := requireUserToken(cmd, "mail template get")
		if err != nil {
			return err
		}
		tpl, err := client.GetMailTemplate(mailbox, strings.TrimSpace(templateID), token)
		if err != nil {
			return fmt.Errorf("获取邮件模板失败: %w", err)
		}
		if output == "json" {
			return printJSON(tpl)
		}
		fmt.Printf("模板 ID: %s\n名称: %s\n", tpl.TemplateID, sanitizeMailSingleLine(tpl.Name))
		if tpl.Subject != "" {
			fmt.Printf("主题: %s\n", sanitizeMailSingleLine(tpl.Subject))
		}
		if len(tpl.Tos) > 0 {
			fmt.Printf("收件人: %s\n", formatMailTemplateAddrs(tpl.Tos))
		}
		if len(tpl.Ccs) > 0 {
			fmt.Printf("抄送: %s\n", formatMailTemplateAddrs(tpl.Ccs))
		}
		if len(tpl.Bccs) > 0 {
			fmt.Printf("密送: %s\n", formatMailTemplateAddrs(tpl.Bccs))
		}
		if len(tpl.Attachments) > 0 {
			names := make([]string, 0, len(tpl.Attachments))
			for _, a := range tpl.Attachments {
				names = append(names, a.Filename)
			}
			fmt.Printf("附件(%d): %s\n", len(names), strings.Join(names, ", "))
		}
		mode := "HTML"
		if tpl.IsPlainTextMode {
			mode = "纯文本"
		}
		fmt.Printf("正文（%s）:\n", mode)
		body := tpl.TemplateContent
		if !tpl.IsPlainTextMode && strings.Contains(body, "<") {
			body = mailHTMLToText(body)
		}
		fmt.Println(sanitizeMailText(body))
		return nil
	},
}

var mailTemplateUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新邮件模板（未指定的字段保持不变）",
	Long: `更新个人邮件模板：先读取当前模板，只覆盖显式传入的字段，再整体写回（服务端为 PUT 全量语义，无乐观锁）。
附件保持不变。

至少指定一项修改:
  --name / --subject / --body / --plain-text / --to / --cc / --bcc（--to "" 表示清空）

示例:
  feishu-cli mail template update --template-id 764xxx --subject "新主题"
  feishu-cli mail template update --template-id 764xxx --body "<p>新正文</p>" --cc ""`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		templateID, _ := cmd.Flags().GetString("template-id")
		output, _ := cmd.Flags().GetString("output")
		templateID = strings.TrimSpace(templateID)
		if templateID == "" {
			return clierr.Usagef("--template-id 必填")
		}
		var changed []string
		for _, f := range []string{"name", "subject", "body", "plain-text", "to", "cc", "bcc"} {
			if cmd.Flags().Changed(f) {
				changed = append(changed, f)
			}
		}
		if len(changed) == 0 {
			return clierr.Usagef("至少指定一项修改：--name / --subject / --body / --plain-text / --to / --cc / --bcc")
		}
		addrFlag := func(name string) ([]client.MailTemplateAddr, error) {
			raw, _ := cmd.Flags().GetString(name)
			list, err := parseEmailList(raw)
			if err != nil {
				return nil, clierr.Usage(err)
			}
			return toMailTemplateAddrs(list), nil
		}
		// 先离线校验，再联网
		if cmd.Flags().Changed("name") {
			name, _ := cmd.Flags().GetString("name")
			if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 {
				return clierr.Usagef("--name 不能为空且不超过 100 个字符")
			}
		}
		for _, f := range []string{"to", "cc", "bcc"} {
			if cmd.Flags().Changed(f) {
				if _, err := addrFlag(f); err != nil {
					return err
				}
			}
		}

		token, err := requireUserToken(cmd, "mail template update")
		if err != nil {
			return err
		}
		tpl, err := client.GetMailTemplate(mailbox, templateID, token)
		if err != nil {
			return fmt.Errorf("读取邮件模板失败: %w", err)
		}
		if cmd.Flags().Changed("name") {
			tpl.Name, _ = cmd.Flags().GetString("name")
		}
		if cmd.Flags().Changed("subject") {
			tpl.Subject, _ = cmd.Flags().GetString("subject")
		}
		if cmd.Flags().Changed("body") {
			tpl.TemplateContent, _ = cmd.Flags().GetString("body")
		}
		if cmd.Flags().Changed("plain-text") {
			tpl.IsPlainTextMode, _ = cmd.Flags().GetBool("plain-text")
		}
		if cmd.Flags().Changed("to") {
			tpl.Tos, _ = addrFlag("to")
		}
		if cmd.Flags().Changed("cc") {
			tpl.Ccs, _ = addrFlag("cc")
		}
		if cmd.Flags().Changed("bcc") {
			tpl.Bccs, _ = addrFlag("bcc")
		}
		update := *tpl
		update.TemplateID = ""
		update.CreateTime = ""
		updated, err := client.UpdateMailTemplate(mailbox, templateID, &update, token)
		if err != nil {
			return fmt.Errorf("更新邮件模板失败: %w", err)
		}
		if output == "json" {
			return printJSON(map[string]any{"template_id": templateID, "updated": true, "changed": changed, "template": updated})
		}
		fmt.Printf("邮件模板已更新: %s（修改: %s）\n", templateID, strings.Join(changed, ", "))
		return nil
	},
}

var mailTemplateDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除邮件模板（需确认）",
	Long: `删除个人邮件模板。非交互环境需加 --yes；--dry-run 只打印请求（优先于确认）。

示例:
  feishu-cli mail template delete --template-id 764xxx --dry-run
  feishu-cli mail template delete --template-id 764xxx --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		templateID, _ := cmd.Flags().GetString("template-id")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		templateID = strings.TrimSpace(templateID)
		if templateID == "" {
			return clierr.Usagef("--template-id 必填")
		}
		if mailbox == "" {
			mailbox = "me"
		}
		if dryRun {
			return printJSON(map[string]any{"dry_run": true, "method": "DELETE",
				"path": "/open-apis/mail/v1/user_mailboxes/" + mailbox + "/templates/" + templateID})
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("将删除邮件模板 %s，确认?", templateID)); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail template delete")
		if err != nil {
			return err
		}
		if err := client.DeleteMailTemplate(mailbox, templateID, token); err != nil {
			return fmt.Errorf("删除邮件模板失败: %w", err)
		}
		if output == "json" {
			return printJSON(map[string]any{"template_id": templateID, "deleted": true})
		}
		fmt.Printf("邮件模板已删除: %s\n", templateID)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{mailTemplateGetCmd, mailTemplateUpdateCmd, mailTemplateDeleteCmd} {
		mailTemplateCmd.AddCommand(c)
		c.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
		c.Flags().String("template-id", "", "模板 ID（必填）")
		c.Flags().StringP("output", "o", "", "输出格式（json）")
		c.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
		mustMarkFlagRequired(c, "template-id")
	}
	mailTemplateUpdateCmd.Flags().String("name", "", "新模板名（≤100 字符）")
	mailTemplateUpdateCmd.Flags().String("subject", "", "新默认主题")
	mailTemplateUpdateCmd.Flags().String("body", "", "新默认正文")
	mailTemplateUpdateCmd.Flags().Bool("plain-text", false, "纯文本模式（is_plain_text_mode）")
	mailTemplateUpdateCmd.Flags().String("to", "", "默认收件人（逗号分隔，空字符串清空）")
	mailTemplateUpdateCmd.Flags().String("cc", "", "默认抄送（逗号分隔，空字符串清空）")
	mailTemplateUpdateCmd.Flags().String("bcc", "", "默认密送（逗号分隔，空字符串清空）")
	mailTemplateDeleteCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
}
