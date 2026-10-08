package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// parseMailMessageIDs 解析 --message-ids 列表（逗号分隔）。
// 规则：
// 1. 保留请求顺序和重复 ID（不去重）；
// 2. 不设 50 条上限（由底层 client 自动按 20 条切片分块请求）；
// 3. 空串、全空白串、或包含空 segment（如 "m1,,m2"、",m1"、"m1,"、"m1, ,m2"）时返回明确错误。
func parseMailMessageIDs(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("--message-ids 至少需要一个 ID")
	}
	parts := strings.Split(trimmed, ",")
	res := make([]string, 0, len(parts))
	for i, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return nil, fmt.Errorf("--message-ids 包含空的邮件 ID（第 %d 项为空）", i+1)
		}
		res = append(res, item)
	}
	return res, nil
}

var mailMessagesCmd = &cobra.Command{
	Use:   "messages",
	Short: "批量获取多封邮件",
	Long: `批量获取多封邮件（客户端自动按每批 20 条分块请求，严格保序并支持重复 ID）。

必填:
  --message-ids  邮件 ID 列表（逗号分隔）

正文字段默认从 base64url 解码为明文（--raw-body 保留原始编码）。

可选:
  --mailbox    默认 me（Bot 身份需显式指定具体邮箱地址）
  --format     full / plain_text_full（默认 full）
  --raw-body   保留 API 原始 base64url 正文（不解码）
  --as         身份选择: bot | user | auto（默认 auto）
  -o json      JSON 格式

示例:
  feishu-cli mail messages --message-ids msg_1,msg_2,msg_3`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		raw, _ := cmd.Flags().GetString("message-ids")
		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")
		rawBody, _ := cmd.Flags().GetBool("raw-body")

		// 1. 本地参数校验前置（在身份解析与任何网络请求前执行）
		ids, err := parseMailMessageIDs(raw)
		if err != nil {
			return err
		}

		if format == "" {
			format = "full"
		}
		if format != "full" && format != "plain_text_full" {
			return fmt.Errorf("--format 仅支持 full|plain_text_full，得到 %q", format)
		}

		if output != "" && output != "json" {
			return fmt.Errorf("-o/--output 仅支持 json，得到 %q", output)
		}

		// 2. 身份解析（仅在本地参数校验全绿后才执行，避免非法输入触发 token 刷新）
		token, mailbox, err := resolveMailReadIdentity(cmd)
		if err != nil {
			return err
		}

		data, err := client.BatchGetMailMessages(mailbox, ids, format, token)
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
	mailCmd.AddCommand(mailMessagesCmd)
	mailMessagesCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailMessagesCmd.Flags().String("message-ids", "", "邮件 ID 列表，逗号分隔（必填）")
	mailMessagesCmd.Flags().String("format", "full", "格式: full/plain_text_full")
	mailMessagesCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	mailMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailMessagesCmd.Flags().Bool("raw-body", false, "保留 API 原始 base64url 编码的正文字段（默认解码为明文）")
	mailMessagesCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailMessagesCmd, "message-ids")
}
