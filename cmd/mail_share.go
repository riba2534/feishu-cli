package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailShareReceiveIDTypes = []string{"chat_id", "open_id", "user_id", "union_id", "email"}

var mailShareToChatCmd = &cobra.Command{
	Use:   "share-to-chat",
	Short: "把邮件或邮件线程以卡片形式分享到 IM 会话",
	Long: `把一封邮件（或整个邮件线程）以卡片形式分享到飞书群聊或私聊，以当前登录用户身份发出。

两步完成：
  1. POST /open-apis/mail/v1/user_mailboxes/{mailbox}/messages/share_token 生成分享卡片（返回 card_id）
  2. POST /open-apis/mail/v1/user_mailboxes/{mailbox}/share_tokens/{card_id}/send 投递到会话
第 2 步失败时错误信息会带上 card_id。

必填:
  --message-id | --thread-id   要分享的邮件 ID 或线程 ID（二选一）
  --receive-id                 接收者 ID

可选:
  --receive-id-type   chat_id（默认）| open_id | user_id | union_id | email
  --mailbox           邮箱地址（默认 me）
  --dry-run           只打印两步请求，不调用接口
  -o json             JSON 输出（card_id / im_message_id）

权限:
  - User Access Token
  - mail:user_mailbox.message:readonly、im:message、im:message.send_as_user

示例:
  feishu-cli mail share-to-chat --message-id <message_id> --receive-id oc_xxx --dry-run
  feishu-cli mail share-to-chat --thread-id <thread_id> --receive-id user@example.com --receive-id-type email`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mailbox, _ := cmd.Flags().GetString("mailbox")
		messageID, _ := cmd.Flags().GetString("message-id")
		threadID, _ := cmd.Flags().GetString("thread-id")
		receiveID, _ := cmd.Flags().GetString("receive-id")
		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
		output, _ := cmd.Flags().GetString("output")
		messageID = strings.TrimSpace(messageID)
		threadID = strings.TrimSpace(threadID)
		receiveID = strings.TrimSpace(receiveID)
		receiveIDType = strings.TrimSpace(receiveIDType)
		mailbox = strings.TrimSpace(mailbox)
		if mailbox == "" {
			mailbox = "me"
		}

		switch {
		case messageID == "" && threadID == "":
			return clierr.Usagef("必须指定 --message-id 或 --thread-id 之一")
		case messageID != "" && threadID != "":
			return clierr.Usagef("--message-id 与 --thread-id 互斥，只能指定一个")
		}
		if receiveID == "" {
			return clierr.Usagef("--receive-id 必填")
		}
		if !mailShareStringIn(mailShareReceiveIDTypes, receiveIDType) {
			return clierr.Usagef("--receive-id-type 仅支持 %s，得到 %q", strings.Join(mailShareReceiveIDTypes, "/"), receiveIDType)
		}

		createBody := map[string]any{"message_id": messageID}
		if threadID != "" {
			createBody = map[string]any{"thread_id": threadID}
		}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, "分享邮件卡片到 IM 会话（预览，不会调用接口）", map[string]any{"identity": "user"}, []dryRunStep{
				{
					Method: "POST",
					URL:    client.MailMailboxPath(mailbox, "messages", "share_token"),
					Desc:   "生成分享卡片，返回 card_id",
					Body:   createBody,
				},
				{
					Method: "POST",
					URL:    client.MailMailboxPath(mailbox, "share_tokens") + "/<card_id>/send",
					Desc:   "把卡片投递到会话",
					Params: map[string]any{"receive_id_type": receiveIDType},
					Body:   map[string]any{"receive_id": receiveID},
				},
			})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail share-to-chat")
		if err != nil {
			return err
		}

		cardID, err := client.CreateMailShareToken(mailbox, messageID, threadID, token)
		if err != nil {
			return fmt.Errorf("生成邮件分享卡片失败: %w", err)
		}
		imMessageID, err := client.SendMailShareCard(mailbox, cardID, receiveID, receiveIDType, token)
		if err != nil {
			return fmt.Errorf("分享卡片已生成（card_id=%s）但投递失败: %w", cardID, err)
		}

		if output == "json" {
			return printJSON(map[string]any{"card_id": cardID, "im_message_id": imMessageID})
		}
		fmt.Printf("邮件卡片已分享\n")
		fmt.Printf("  card_id:       %s\n", cardID)
		fmt.Printf("  im_message_id: %s\n", imMessageID)
		return nil
	},
}

func mailShareStringIn(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func init() {
	mailCmd.AddCommand(mailShareToChatCmd)
	mailShareToChatCmd.Flags().String("message-id", "", "要分享的邮件 ID（与 --thread-id 互斥）")
	mailShareToChatCmd.Flags().String("thread-id", "", "要分享的邮件线程 ID（与 --message-id 互斥）")
	mailShareToChatCmd.Flags().String("receive-id", "", "接收者 ID（必填）")
	mailShareToChatCmd.Flags().String("receive-id-type", "chat_id", "接收者 ID 类型: chat_id | open_id | user_id | union_id | email")
	mailShareToChatCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailShareToChatCmd.Flags().Bool("dry-run", false, "只打印两步请求，不调用接口")
	mailShareToChatCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailShareToChatCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
