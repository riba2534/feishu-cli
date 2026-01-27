package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var sendMessageCmd = &cobra.Command{
	Use:   "send",
	Short: "发送消息",
	Long: `向飞书用户或群组发送消息。

参数:
  --receive-id-type   接收者类型（必填）
  --receive-id        接收者标识（必填）
  --msg-type          消息类型（默认: text）
  --content, -c       消息内容 JSON
  --content-file      消息内容 JSON 文件
  --text, -t          简单文本消息（快捷方式）
  --output, -o        输出格式（json）

接收者类型:
  email       邮箱
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID
  chat_id     群组 ID

消息类型:
  text         文本消息
  post         富文本消息
  image        图片消息
  file         文件消息
  audio        音频消息
  media        媒体消息
  sticker      表情消息
  interactive  卡片消息
  share_chat   分享群消息
  share_user   分享用户消息

示例:
  # 发送文本消息
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --text "你好，这是一条测试消息"

  # 发送到群组
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --text "群消息"

  # 发送卡片消息
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --msg-type interactive \
    --content-file card.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
		receiveID, _ := cmd.Flags().GetString("receive-id")
		msgType, _ := cmd.Flags().GetString("msg-type")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		text, _ := cmd.Flags().GetString("text")

		// Get content from various sources
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
			// Simple text message
			msgType = "text"
			msgContent = client.CreateTextMessageContent(text)
		} else {
			return fmt.Errorf("必须指定 --content、--content-file 或 --text")
		}

		messageID, err := client.SendMessage(receiveIDType, receiveID, msgType, msgContent)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"message_id": messageID,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("消息发送成功！\n")
			fmt.Printf("  消息 ID: %s\n", messageID)
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(sendMessageCmd)
	sendMessageCmd.Flags().String("receive-id-type", "", "接收者类型（email/open_id/user_id/union_id/chat_id）")
	sendMessageCmd.Flags().String("receive-id", "", "接收者标识")
	sendMessageCmd.Flags().String("msg-type", "text", "消息类型（text/post/image/interactive 等）")
	sendMessageCmd.Flags().StringP("content", "c", "", "消息内容 JSON")
	sendMessageCmd.Flags().String("content-file", "", "消息内容 JSON 文件")
	sendMessageCmd.Flags().StringP("text", "t", "", "简单文本消息")
	sendMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(sendMessageCmd, "receive-id-type", "receive-id")
}
