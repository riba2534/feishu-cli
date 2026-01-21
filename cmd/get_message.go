package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getMessageCmd = &cobra.Command{
	Use:   "get <message_id>",
	Short: "获取消息详情",
	Long: `获取指定消息的详细信息。

参数:
  message_id    消息 ID（必填）
  --output, -o  输出格式（json）

示例:
  # 获取消息详情
  feishu-cli msg get om_xxx

  # JSON 格式输出
  feishu-cli msg get om_xxx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]

		result, err := client.GetMessage(messageID)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		msg := result.Message
		if output == "json" {
			data, _ := json.MarshalIndent(msg, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("消息详情:\n")
			if msg.MessageId != nil {
				fmt.Printf("  消息 ID: %s\n", *msg.MessageId)
			}
			if msg.RootId != nil && *msg.RootId != "" {
				fmt.Printf("  根消息 ID: %s\n", *msg.RootId)
			}
			if msg.ParentId != nil && *msg.ParentId != "" {
				fmt.Printf("  父消息 ID: %s\n", *msg.ParentId)
			}
			if msg.MsgType != nil {
				fmt.Printf("  消息类型: %s\n", *msg.MsgType)
			}
			if msg.CreateTime != nil {
				fmt.Printf("  创建时间: %s\n", *msg.CreateTime)
			}
			if msg.UpdateTime != nil {
				fmt.Printf("  更新时间: %s\n", *msg.UpdateTime)
			}
			if msg.Deleted != nil {
				fmt.Printf("  是否已删除: %v\n", *msg.Deleted)
			}
			if msg.ChatId != nil {
				fmt.Printf("  会话 ID: %s\n", *msg.ChatId)
			}
			if msg.Sender != nil {
				fmt.Printf("  发送者:\n")
				if msg.Sender.Id != nil {
					fmt.Printf("    ID: %s\n", *msg.Sender.Id)
				}
				if msg.Sender.IdType != nil {
					fmt.Printf("    ID 类型: %s\n", *msg.Sender.IdType)
				}
				if msg.Sender.SenderType != nil {
					fmt.Printf("    发送者类型: %s\n", *msg.Sender.SenderType)
				}
				if msg.Sender.TenantKey != nil {
					fmt.Printf("    租户 Key: %s\n", *msg.Sender.TenantKey)
				}
			}
			if msg.Body != nil && msg.Body.Content != nil {
				fmt.Printf("  消息内容: %s\n", *msg.Body.Content)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(getMessageCmd)
	getMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
