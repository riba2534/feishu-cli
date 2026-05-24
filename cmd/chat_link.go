package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var chatLinkCmd = &cobra.Command{
	Use:   "link <chat_id>",
	Short: "获取群分享链接",
	Long: `获取指定群聊的分享链接。

参数:
  chat_id             群 ID（必填）
  --validity-period   链接有效期（week/year/permanently，默认 week）

示例:
  # 获取 7 天有效的分享链接
  feishu-cli chat link oc_xxx

  # 获取永久有效的分享链接
  feishu-cli chat link oc_xxx --validity-period permanently`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		chatID := args[0]
		validityPeriod, _ := cmd.Flags().GetString("validity-period")

		link, err := client.GetChatLink(chatID, validityPeriod)
		if err != nil {
			return translateChatError(err)
		}

		fmt.Printf("群分享链接: %s\n", link)

		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatLinkCmd)
	chatLinkCmd.Flags().String("validity-period", "week", "链接有效期（week/year/permanently）")
}
