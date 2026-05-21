package cmd

import (
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getMessageCmd = &cobra.Command{
	Use:   "get <message_id>",
	Short: "获取消息详情",
	Long: `获取指定消息的详细信息。

参数:
  message_id            消息 ID（必填）
  --output, -o          输出格式（json）
  --card-content-type   interactive 卡片返回格式：user / raw / rendered（默认 user）

示例:
  # 获取消息详情
  feishu-cli msg get om_xxx

  # JSON 格式输出
  feishu-cli msg get om_xxx --output json

  # interactive 卡片返回 schema 2.0 JSON，并额外提取 card_texts（默认）
  feishu-cli msg get om_xxx --card-content-type user

  # interactive 卡片返回平台内部完整 cardDSL
  feishu-cli msg get om_xxx --card-content-type raw

  # 保留 OAPI 原始渲染版/降级版
  feishu-cli msg get om_xxx --card-content-type rendered`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]

		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}

		result, err := client.GetMessage(messageID, token, cardContentType)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		msg := result.Message
		// 合并主消息 + merge_forward 子消息，统一解析 sender_names
		allMsgs := []*larkim.Message{msg}
		if len(result.SubMessages) > 0 {
			allMsgs = append(allMsgs, result.SubMessages...)
		}
		senderNames := client.ResolveSenderNames(allMsgs, token)
		if output == "json" {
			enriched := map[string]any{
				"message":      msg,
				"sender_names": senderNames,
			}
			if cardTexts := client.ExtractCardTexts(msg); len(cardTexts) > 0 {
				enriched["card_texts"] = cardTexts
			}
			if len(result.SubMessages) > 0 {
				enriched["sub_messages"] = result.SubMessages
				if cardTextMap := client.ExtractCardTextMap(result.SubMessages); len(cardTextMap) > 0 {
					enriched["sub_message_card_texts"] = cardTextMap
				}
			}
			if err := printJSON(enriched); err != nil {
				return err
			}
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
			if cardTexts := client.ExtractCardTexts(msg); len(cardTexts) > 0 {
				fmt.Printf("  卡片文本:\n")
				for _, text := range cardTexts {
					fmt.Printf("    %s\n", text)
				}
			}
			if len(result.SubMessages) > 0 {
				fmt.Printf("  嵌套子消息: %d 条（含递归展开，使用 --output json 查看完整内容）\n", len(result.SubMessages))
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(getMessageCmd)
	getMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	getMessageCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addCardContentTypeFlag(getMessageCmd)
}
