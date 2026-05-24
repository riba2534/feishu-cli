package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var chatGetCmd = &cobra.Command{
	Use:   "get <chat_id>",
	Short: "获取群聊信息",
	Long: `获取指定群聊的详细信息。

参数:
  chat_id    群 ID（必填）

示例:
  feishu-cli chat get oc_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveRequiredUserToken(cmd)
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
}
