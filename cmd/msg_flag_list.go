package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgFlagListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出当前用户的消息书签",
	Long: `列出当前用户的所有消息书签（含 message 层和 feed 层）。

可选 flag:
  --page-size          每页数量 (1-50，默认 50)
  --page-token         翻页标记
  --user-access-token  显式指定 User Access Token

输出:
  完整 JSON，包含 flag_items / delete_flag_items / messages / has_more / page_token。
  其中 flag_items[i].item_type 与 flag_type 为整数枚举（与服务端 OpenAPI 对齐）：
    item_type:  0=default, 4=thread, 11=msg_thread
    flag_type:  1=feed, 2=message

示例:
  feishu-cli msg flag list
  feishu-cli msg flag list --page-size 20
  feishu-cli msg flag list --page-token "xxxx"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.ListFlags(pageSize, pageToken, token)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	msgFlagCmd.AddCommand(msgFlagListCmd)
	msgFlagListCmd.Flags().Int("page-size", 50, "每页数量 (1-50)")
	msgFlagListCmd.Flags().String("page-token", "", "翻页标记")
	msgFlagListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
