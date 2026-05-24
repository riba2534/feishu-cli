package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var chatUpdateCmd = &cobra.Command{
	Use:   "update <chat_id>",
	Short: "更新群聊信息",
	Long: `更新指定群聊的名称、描述或群主。

参数:
  chat_id        群 ID（必填）
  --name         新群名称
  --description  新群描述
  --owner-id     新群主 ID

示例:
  # 更新群名称
  feishu-cli chat update oc_xxx --name "新群名"

  # 更新群描述
  feishu-cli chat update oc_xxx --description "新的群描述"

  # 转让群主
  feishu-cli chat update oc_xxx --owner-id ou_yyy`,
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
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		ownerID, _ := cmd.Flags().GetString("owner-id")

		if name == "" && description == "" && ownerID == "" {
			return fmt.Errorf("至少需要指定 --name、--description 或 --owner-id 中的一个")
		}

		if err := client.UpdateChat(chatID, name, description, ownerID, token); err != nil {
			return translateChatError(err)
		}

		fmt.Printf("群聊信息更新成功！\n")
		fmt.Printf("  群 ID: %s\n", chatID)

		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatUpdateCmd)
	chatUpdateCmd.Flags().String("name", "", "新群名称")
	chatUpdateCmd.Flags().String("description", "", "新群描述")
	chatUpdateCmd.Flags().String("owner-id", "", "新群主 ID")
	chatUpdateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
