package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var uploadImageCmd = &cobra.Command{
	Use:   "upload-image <file>",
	Short: "上传图片获取 image_key",
	Long: `上传图片到飞书消息图片库，返回 image_key 用于发送图片消息。

支持格式: PNG, JPG, JPEG, GIF, BMP, SVG, WEBP, ICO, TIFF
大小限制: 最大 10MB

上传后可配合 msg send 发送图片消息:
  feishu-cli msg send --msg-type image -c '{"image_key":"<image_key>"}' ...

或使用 --send 参数直接上传并发送:
  feishu-cli msg upload-image photo.png --send --receive-id-type email --receive-id user@example.com

示例:
  # 仅上传，返回 image_key
  feishu-cli msg upload-image screenshot.png

  # 上传并发送给用户
  feishu-cli msg upload-image screenshot.png \
    --send \
    --receive-id-type email \
    --receive-id user@example.com

  # 上传并发送到群聊
  feishu-cli msg upload-image chart.png \
    --send \
    --receive-id-type chat_id \
    --receive-id oc_xxx

  # JSON 输出（便于脚本使用）
  feishu-cli msg upload-image screenshot.png -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath := args[0]
		output, _ := cmd.Flags().GetString("output")
		send, _ := cmd.Flags().GetBool("send")

		if send {
			// Upload and send in one step
			receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
			receiveID, _ := cmd.Flags().GetString("receive-id")
			token := resolveOptionalUserToken(cmd)

			if receiveIDType == "" || receiveID == "" {
				return fmt.Errorf("使用 --send 时必须指定 --receive-id-type 和 --receive-id")
			}

			imageKey, messageID, err := client.UploadAndSendImage(filePath, receiveIDType, receiveID, token)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(map[string]string{
					"image_key":  imageKey,
					"message_id": messageID,
				})
			}
			fmt.Printf("图片上传并发送成功！\n")
			fmt.Printf("  image_key: %s\n", imageKey)
			fmt.Printf("  消息 ID:   %s\n", messageID)
		} else {
			// Upload only
			imageKey, err := client.UploadImage(filePath)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(map[string]string{
					"image_key": imageKey,
				})
			}
			fmt.Printf("图片上传成功！\n")
			fmt.Printf("  image_key: %s\n", imageKey)
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(uploadImageCmd)
	uploadImageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	uploadImageCmd.Flags().Bool("send", false, "上传后直接发送图片消息")
	uploadImageCmd.Flags().String("receive-id-type", "", "接收者类型（email/open_id/user_id/union_id/chat_id）")
	uploadImageCmd.Flags().String("receive-id", "", "接收者标识")
	uploadImageCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
