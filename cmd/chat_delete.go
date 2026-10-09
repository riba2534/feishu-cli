package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var chatDeleteCmd = &cobra.Command{
	Use:   "delete <chat_id>",
	Short: "解散群聊",
	Long: `解散指定的群聊。此操作不可逆，请谨慎操作。

参数:
  chat_id    群 ID（必填）

身份:
  --as bot|user|auto   默认 auto：已登录用 User Token（与旧版一致），未登录回退 Bot；
                       User Token 已配置但不可用时直接报错，不会静默切 Bot。
                       --as bot 用应用身份（Bot 需在群内）。

示例:
  feishu-cli chat delete oc_xxx --yes`,
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

		if err := confirmDangerousAction(cmd, fmt.Sprintf("确定要解散群聊 %s 吗？此操作不可逆", chatID)); err != nil {
			return err
		}

		if err := client.DeleteChat(chatID, token); err != nil {
			return translateChatError(err)
		}

		fmt.Printf("群聊已解散！\n")
		fmt.Printf("  群 ID: %s\n", chatID)

		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatDeleteCmd)
	chatDeleteCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addAsFlag(chatDeleteCmd)
}
