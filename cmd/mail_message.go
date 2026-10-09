package cmd

import (
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var mailMessageCmd = &cobra.Command{
	Use:   "message",
	Short: "获取单封邮件",
	Long: `获取单封邮件的完整内容（含 HTML body 或纯文本 body）。

正文字段 body_plain_text / body_html / body_preview 在 API 中是 base64url 编码，
本命令默认解码为明文输出（body_plain_text 额外清除 ANSI/控制字符，防终端注入）；
需要 API 原始编码值时加 --raw-body。文本模式输出可读的邮件头 + 纯文本正文。

必填:
  --message-id

可选:
  --mailbox    邮箱地址（默认 me，即当前登录用户）
  --format     full / plain_text_full / metadata（默认 full；metadata 只返回元信息不含正文）
  --raw-body   保留 API 原始 base64url 正文（不解码；服务端没有 raw 格式）
  -o json      JSON 格式输出

示例:
  feishu-cli mail message --message-id msg_xxx
  feishu-cli mail message --message-id msg_xxx --format plain_text_full`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID, _ := cmd.Flags().GetString("message-id")
		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")
		rawBody, _ := cmd.Flags().GetBool("raw-body")

		// 本地参数校验前置（用法错误 exit 2），再解析身份
		if messageID == "" {
			return clierr.Usagef("--message-id 必填")
		}
		format, err := normalizeMailReadFormat(format)
		if err != nil {
			return err
		}

		token, mailbox, err := resolveMailReadIdentity(cmd)
		if err != nil {
			return err
		}

		data, err := client.GetMailMessage(mailbox, messageID, format, token)
		if err != nil {
			return err
		}

		payload, err := decodeMailPayloadBodies(data, rawBody)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(payload)
		}
		renderMailPayloadText(os.Stdout, payload)
		return nil
	},
}

func init() {
	mailCmd.AddCommand(mailMessageCmd)
	mailMessageCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailMessageCmd.Flags().String("message-id", "", "邮件 message_id（必填）")
	mailMessageCmd.Flags().String("format", "full", mailReadFormatHelp)
	mailMessageCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	mailMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailMessageCmd.Flags().Bool("raw-body", false, "保留 API 原始 base64url 编码的正文字段（默认解码为明文）")
	mailMessageCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailMessageCmd, "message-id")
}
