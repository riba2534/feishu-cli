package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var forwardMessageCmd = &cobra.Command{
	Use:   "forward <message_id>",
	Short: "转发消息",
	Long: `将消息转发给指定的接收者。

参数:
  message_id         消息 ID（必填）
  --receive-id       接收者 ID（必填）
  --receive-id-type  接收者 ID 类型（必填）
  --output, -o       输出格式（json）

接收者类型:
  email       邮箱
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID
  chat_id     群组 ID

示例:
  # 转发消息给用户
  feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email

  # 转发消息到群组
  feishu-cli msg forward om_xxx --receive-id oc_xxx --receive-id-type chat_id

  # JSON 格式输出
  feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]
		receiveID, _ := cmd.Flags().GetString("receive-id")
		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")

		newMessageID, err := client.ForwardMessage(messageID, receiveID, receiveIDType)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"original_message_id": messageID,
				"new_message_id":      newMessageID,
			}, "", "  ")
			fmt.Println(string(data))
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
	forwardMessageCmd.Flags().String("receive-id-type", "", "接收者 ID 类型（email/open_id/user_id/union_id/chat_id）")
	forwardMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(forwardMessageCmd, "receive-id", "receive-id-type")
}
