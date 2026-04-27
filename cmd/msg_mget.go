package cmd

import (
	"fmt"

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
  --card-content-type   interactive 卡片返回格式：user / raw（默认空，返回渲染版）

示例:
  # 获取多条消息
  feishu-cli msg mget --message-ids om_xxx,om_yyy,om_zzz

  # interactive 卡片返回原始 schema 2.0 JSON
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

		messages, err := client.BatchGetMessages(messageIDs, userToken, cardContentType)
		if err != nil {
			return err
		}

		senderNames := client.ResolveSenderNames(messages, userToken)
		return printJSON(map[string]any{
			"messages":     messages,
			"sender_names": senderNames,
		})
	},
}

func init() {
	msgCmd.AddCommand(msgMgetCmd)
	msgMgetCmd.Flags().String("message-ids", "", "消息 ID 列表（逗号分隔）")
	msgMgetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addCardContentTypeFlag(msgMgetCmd)
	mustMarkFlagRequired(msgMgetCmd, "message-ids")
}
