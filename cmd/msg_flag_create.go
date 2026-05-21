package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgFlagCreateCmd = &cobra.Command{
	Use:   "create <message_id>",
	Short: "为消息创建书签",
	Long: `为指定消息创建书签（收藏）。

参数:
  message_id           消息 ID (om_xxx，必填位置参数)

可选 flag:
  --item-type          item 类型：default | thread | msg_thread (默认 default)
  --flag-type          flag 类型：message | feed                (默认 message)
  --output, -o         输出格式：json
  --user-access-token  显式指定 User Access Token

注意:
  仅支持以下组合，其余服务端会拒绝：
    default     + message    消息层书签（默认，最常见）
    thread      + feed       topic-style 话题群 feed 层
    msg_thread  + feed       普通群消息线程 feed 层

  feed 层书签需要先确定群类型（topic-style → thread；普通群 → msg_thread）。
  如不确定，可在飞书 UI 上对该消息执行书签后用 list 命令查看实际 item_type。

示例:
  # 消息层书签（最常见）
  feishu-cli msg flag create om_xxx

  # feed 层书签（普通群线程）
  feishu-cli msg flag create om_xxx --item-type msg_thread --flag-type feed`,
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

		data, err := client.CreateFlag(messageID, itemType, flagType, token)
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

		fmt.Printf("书签创建成功！\n")
		fmt.Printf("  消息 ID: %s\n", messageID)
		fmt.Printf("  item_type: %s, flag_type: %s\n", itemTypeStr, flagTypeStr)
		return nil
	},
}

func init() {
	msgFlagCmd.AddCommand(msgFlagCreateCmd)
	msgFlagCreateCmd.Flags().String("item-type", "default", "item 类型：default | thread | msg_thread")
	msgFlagCreateCmd.Flags().String("flag-type", "message", "flag 类型：message | feed")
	msgFlagCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	msgFlagCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
