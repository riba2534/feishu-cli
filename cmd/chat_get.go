package cmd

import (
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var chatGetCmd = &cobra.Command{
	Use:   "get <chat_id>",
	Short: "获取群聊信息",
	Long: `获取指定群聊的详细信息。

参数:
  chat_id    群 ID（必填）

身份:
  --as bot|user|auto   默认 auto：已登录用 User Token（与旧版一致），未登录回退 Bot；
                       User Token 已配置但不可用时直接报错，不会静默切 Bot。
                       --as bot 用应用身份（Bot 需在群内）。

示例:
  feishu-cli chat get oc_xxx
  feishu-cli chat get oc_xxx --as bot`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		chatID := args[0]

		data, err := client.GetChat(chatID, token)
		if err != nil {
			return translateChatError(err)
		}

		return printJSON(data)
	},
}

func init() {
	chatCmd.AddCommand(chatGetCmd)
	chatGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addAsFlag(chatGetCmd)
}
