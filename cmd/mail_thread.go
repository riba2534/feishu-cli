package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailThreadCmd = &cobra.Command{
	Use:   "thread",
	Short: "获取邮件线程",
	Long: `获取一个邮件线程（对话）中的所有邮件，按时间排序。

必填:
  --thread-id

正文字段默认从 base64url 解码为明文（--raw-body 保留原始编码）。

可选:
  --mailbox   默认 me
  --format    full / plain_text_full
  --raw-body  保留 API 原始 base64url 正文（不解码）
  -o json     JSON 格式

示例:
  feishu-cli mail thread --thread-id thread_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, mailbox, err := resolveMailReadIdentity(cmd)
		if err != nil {
			return err
		}

		threadID, _ := cmd.Flags().GetString("thread-id")
		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")
		rawBody, _ := cmd.Flags().GetBool("raw-body")

		if threadID == "" {
			return fmt.Errorf("--thread-id 必填")
		}

		data, err := client.GetMailThread(mailbox, threadID, format, token)
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
	mailCmd.AddCommand(mailThreadCmd)
	mailThreadCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailThreadCmd.Flags().String("thread-id", "", "线程 ID（必填）")
	mailThreadCmd.Flags().String("format", "full", "格式: full/plain_text_full")
	mailThreadCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	mailThreadCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailThreadCmd.Flags().Bool("raw-body", false, "保留 API 原始 base64url 编码的正文字段（默认解码为明文）")
	mailThreadCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailThreadCmd, "thread-id")
}
