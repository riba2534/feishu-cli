package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var forwardMessageCmd = &cobra.Command{
	Use:   "forward <message_id|thread_id>",
	Short: "转发消息或整个话题",
	Long: `将消息转发给指定的接收者；传入 omt_ 开头的话题 ID 时转发整个话题
（POST /open-apis/im/v1/threads/{thread_id}/forward）。

参数:
  message_id|thread_id  om_ 消息 ID，或 omt_ 话题 ID（必填）
  --receive-id          接收者 ID（必填）
  --receive-id-type     接收者 ID 类型（必填）
  --dry-run             只打印将调用的接口与请求体，不转发
  --output, -o          输出格式（json）

接收者类型:
  email       邮箱
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID
  chat_id     群组 ID
  thread_id   话题 ID（转发到某个话题内）

身份: 默认 Bot；显式传 --user-access-token 或设置 FEISHU_USER_ACCESS_TOKEN 时以 User 身份转发。

示例:
  # 转发消息给用户
  feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email

  # 转发消息到群组
  feishu-cli msg forward om_xxx --receive-id oc_xxx --receive-id-type chat_id

  # 转发整个话题到群组（先预览）
  feishu-cli msg forward omt_xxx --receive-id oc_xxx --receive-id-type chat_id --dry-run

  # JSON 格式输出
  feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(args[0])
		receiveID, _ := cmd.Flags().GetString("receive-id")
		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
		if messageID == "" {
			return clierr.Usagef("message_id 不能为空")
		}
		isThread := strings.HasPrefix(messageID, "omt_")

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			identity := "bot"
			if flagToken, _ := cmd.Flags().GetString("user-access-token"); flagToken != "" || os.Getenv("FEISHU_USER_ACCESS_TOKEN") != "" {
				identity = "user"
			}
			url := "/open-apis/im/v1/messages/" + messageID + "/forward"
			desc := "转发消息（预览，不会真正转发）"
			if isThread {
				url = "/open-apis/im/v1/threads/" + messageID + "/forward"
				desc = "转发话题（预览，不会真正转发）"
			}
			return printDryRunPlan(cmd, desc, map[string]any{"identity": identity}, []dryRunStep{{
				Method: "POST",
				URL:    url,
				Params: map[string]any{"receive_id_type": receiveIDType},
				Body:   map[string]any{"receive_id": receiveID},
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		forward := client.ForwardMessage
		if isThread {
			forward = client.ForwardThread
		}
		newMessageID, err := forward(messageID, receiveID, receiveIDType, token)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]string{
				"original_message_id": messageID,
				"new_message_id":      newMessageID,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("消息转发成功！\n")
			fmt.Printf("  原消息 ID: %s\n", messageID)
			fmt.Printf("  新消息 ID: %s\n", newMessageID)
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(forwardMessageCmd)
	forwardMessageCmd.Flags().String("receive-id", "", "接收者 ID")
	forwardMessageCmd.Flags().String("receive-id-type", "", "接收者 ID 类型（email/open_id/user_id/union_id/chat_id/thread_id）")
	forwardMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	forwardMessageCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	forwardMessageCmd.Flags().Bool("dry-run", false, "只打印将调用的接口与请求体，不转发")
	mustMarkFlagRequired(forwardMessageCmd, "receive-id", "receive-id-type")
}
