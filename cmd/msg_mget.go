package cmd

import (
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgMgetCmd = &cobra.Command{
	Use:   "mget",
	Short: "批量获取消息详情",
	Long: `批量获取多条消息的详情。

选项:
  --message-ids         消息 ID 列表（逗号分隔，必填）
  --card-content-type   interactive 卡片返回格式：user / raw / rendered（默认 user）

示例:
  # 获取多条消息
  feishu-cli msg mget --message-ids om_xxx,om_yyy,om_zzz

  # interactive 卡片返回 schema 2.0 JSON，并额外提取 card_texts（默认）
  feishu-cli msg mget --message-ids om_xxx,om_yyy --card-content-type user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageIDsStr, _ := cmd.Flags().GetString("message-ids")
		userToken := resolveOptionalUserTokenWithFallback(cmd)

		messageIDs := splitAndTrim(messageIDsStr)
		if len(messageIDs) == 0 {
			return fmt.Errorf("请提供至少一个消息 ID")
		}

		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}

		result, err := client.BatchGetMessages(messageIDs, userToken, cardContentType)
		if err != nil {
			return err
		}

		// 合并主消息 + 所有 merge_forward 子消息，统一解析 sender_names
		allMsgs := append([]*larkim.Message{}, result.Messages...)
		for _, subs := range result.MergeForwardSubMessages {
			allMsgs = append(allMsgs, subs...)
		}
		senderNames := client.ResolveSenderNames(allMsgs, userToken)

		out := map[string]any{
			"messages":     result.Messages,
			"sender_names": senderNames,
		}
		if cardTextMap := client.ExtractCardTextMap(result.Messages); len(cardTextMap) > 0 {
			out["card_texts"] = cardTextMap
		}
		if len(result.MergeForwardSubMessages) > 0 {
			out["merge_forward_sub_messages"] = result.MergeForwardSubMessages
			var subMsgs []*larkim.Message
			for _, subs := range result.MergeForwardSubMessages {
				subMsgs = append(subMsgs, subs...)
			}
			if cardTextMap := client.ExtractCardTextMap(subMsgs); len(cardTextMap) > 0 {
				out["merge_forward_card_texts"] = cardTextMap
			}
		}
		return printJSON(out)
	},
}

func init() {
	msgCmd.AddCommand(msgMgetCmd)
	msgMgetCmd.Flags().String("message-ids", "", "消息 ID 列表（逗号分隔）")
	msgMgetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addCardContentTypeFlag(msgMgetCmd)
	mustMarkFlagRequired(msgMgetCmd, "message-ids")
}
