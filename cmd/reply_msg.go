package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var replyMsgCmd = &cobra.Command{
	Use:   "reply <message_id>",
	Short: "回复消息",
	Long: `回复指定的消息。

参数:
  message_id       消息 ID（必填）
  --msg-type       消息类型（默认 text）
  --text, -t       简单文本消息（快捷方式）
  --content, -c    消息内容 JSON
  --content-file   消息内容 JSON 文件

消息类型:
  text         文本消息
  post         富文本消息
  interactive  卡片消息

示例:
  # 回复文本消息
  feishu-cli msg reply om_xxx --text "收到，谢谢！"

  # 回复富文本消息
  feishu-cli msg reply om_xxx --msg-type post --content-file reply.json

  # 回复卡片消息
  feishu-cli msg reply om_xxx --msg-type interactive --content '{"type":"template",...}'`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]
		msgType, _ := cmd.Flags().GetString("msg-type")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		text, _ := cmd.Flags().GetString("text")

		var msgContent string
		if contentFile != "" {
			data, err := os.ReadFile(contentFile)
			if err != nil {
				return fmt.Errorf("读取内容文件失败: %w", err)
			}
			msgContent = string(data)
		} else if content != "" {
			msgContent = content
		} else if text != "" {
			msgType = "text"
			msgContent = client.CreateTextMessageContent(text)
		} else {
			return fmt.Errorf("必须指定 --content、--content-file 或 --text")
		}

		newMessageID, err := client.ReplyMessage(messageID, msgType, msgContent, token)
		if err != nil {
			return err
		}

		fmt.Printf("消息回复成功！\n")
		fmt.Printf("  原消息 ID: %s\n", messageID)
		fmt.Printf("  新消息 ID: %s\n", newMessageID)

		return nil
	},
}

func init() {
	msgCmd.AddCommand(replyMsgCmd)
	replyMsgCmd.Flags().String("msg-type", "text", "消息类型（text/post/interactive 等）")
	replyMsgCmd.Flags().StringP("text", "t", "", "简单文本消息")
	replyMsgCmd.Flags().StringP("content", "c", "", "消息内容 JSON")
	replyMsgCmd.Flags().String("content-file", "", "消息内容 JSON 文件")
	replyMsgCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
