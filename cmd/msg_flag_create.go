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
  --item-type          item 类型：default | thread | msg_thread（feed 层可省略并自动识别）
  --flag-type          flag 类型：message | feed                (默认 message)
  --output, -o         输出格式：json
  --user-access-token  显式指定 User Access Token

注意:
  仅支持以下组合，其余服务端会拒绝：
    default     + message    消息层书签（默认，最常见）
    thread      + feed       topic-style 话题群 feed 层
    msg_thread  + feed       普通群消息线程 feed 层

  --flag-type feed 且未显式传 --item-type 时，行为如下：
  自动读取消息 chat_id 和群 chat_mode，topic 群使用 thread，普通群使用 msg_thread。

示例:
  # 消息层书签（最常见）
  feishu-cli msg flag create om_xxx

  # feed 层书签（自动判断 thread/msg_thread）
  feishu-cli msg flag create om_xxx --flag-type feed

  # feed 层书签（显式指定普通群线程）
  feishu-cli msg flag create om_xxx --item-type msg_thread --flag-type feed`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := args[0]
		if err := validateMsgFlagMessageID(messageID); err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")

		itemTypeStr, flagTypeStr, itemType, flagType, err := parseMsgFlagTypes(cmd)
		if err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		if shouldAutoDetectMsgFlagItemType(cmd) {
			itemType, itemTypeStr, err = client.ResolveFlagFeedItemType(messageID, token)
			if err != nil {
				return err
			}
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
