package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgFlagCancelCmd = &cobra.Command{
	Use:   "cancel <message_id>",
	Short: "取消（删除）消息书签",
	Long: `取消（删除）指定消息的书签。

参数:
  message_id           消息 ID (om_xxx，必填位置参数)

可选 flag:
  --item-type          item 类型：default | thread | msg_thread (默认 default)
  --flag-type          flag 类型：message | feed                (默认 message)
  --output, -o         输出格式：json
  --user-access-token  显式指定 User Access Token

注意:
  --item-type 与 --flag-type 必须与 create 时一致，否则取消会失败。
  如果不确定原书签 item_type，可用 list 子命令查看后再 cancel。

示例:
  # 取消消息层书签（默认）
  feishu-cli msg flag cancel om_xxx

  # 取消 feed 层书签（普通群线程）
  feishu-cli msg flag cancel om_xxx --item-type msg_thread --flag-type feed`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		messageID := args[0]
		itemTypeStr, _ := cmd.Flags().GetString("item-type")
		flagTypeStr, _ := cmd.Flags().GetString("flag-type")
		output, _ := cmd.Flags().GetString("output")

		itemType, err := client.ParseFlagItemType(itemTypeStr)
		if err != nil {
			return err
		}
		flagType, err := client.ParseFlagFlagType(flagTypeStr)
		if err != nil {
			return err
		}

		data, err := client.CancelFlag(messageID, itemType, flagType, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"message_id": messageID,
				"item_type":  itemTypeStr,
				"flag_type":  flagTypeStr,
				"response":   data,
			})
		}

		fmt.Printf("书签取消成功！\n")
		fmt.Printf("  消息 ID: %s\n", messageID)
		fmt.Printf("  item_type: %s, flag_type: %s\n", itemTypeStr, flagTypeStr)
		return nil
	},
}

func init() {
	msgFlagCmd.AddCommand(msgFlagCancelCmd)
	msgFlagCancelCmd.Flags().String("item-type", "default", "item 类型：default | thread | msg_thread")
	msgFlagCancelCmd.Flags().String("flag-type", "message", "flag 类型：message | feed")
	msgFlagCancelCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	msgFlagCancelCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
