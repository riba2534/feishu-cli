package cmd

import (
	"fmt"
	"os"

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
  不传 --item-type/--flag-type 时：先取消消息层书签，
  再尽量自动判断并取消 feed 层书签。自动判断失败时会跳过 feed 层并打印 warning。
  如果只想取消某一层，可显式传 --item-type 和 --flag-type。

示例:
  # 尽量取消消息层 + feed 层书签（默认）
  feishu-cli msg flag cancel om_xxx

  # 取消 feed 层书签（普通群线程）
  feishu-cli msg flag cancel om_xxx --item-type msg_thread --flag-type feed`,
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

		if shouldDoubleCancelMsgFlag(cmd) {
			return runMsgFlagDoubleCancel(messageID, output, token)
		}

		if shouldAutoDetectMsgFlagItemType(cmd) {
			itemType, itemTypeStr, err = client.ResolveFlagFeedItemType(messageID, token)
			if err != nil {
				return err
			}
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

func runMsgFlagDoubleCancel(messageID, output, token string) error {
	results := make([]map[string]any, 0, 2)
	var lastErr error

	cancelOne := func(itemType int, itemTypeStr string, flagType int, flagTypeStr string) {
		result := map[string]any{
			"message_id": messageID,
			"item_type":  itemTypeStr,
			"flag_type":  flagTypeStr,
		}
		data, err := client.CancelFlag(messageID, itemType, flagType, token)
		if err != nil {
			result["status"] = "failed"
			result["error"] = err.Error()
			lastErr = err
			fmt.Fprintf(os.Stderr, "warning: 取消 %s/%s 书签失败: %v\n", itemTypeStr, flagTypeStr, err)
		} else {
			result["status"] = "ok"
			result["response"] = data
		}
		results = append(results, result)
	}

	defaultItem, _ := client.ParseFlagItemType("default")
	messageFlag, _ := client.ParseFlagFlagType("message")
	cancelOne(defaultItem, "default", messageFlag, "message")

	feedFlag, _ := client.ParseFlagFlagType("feed")
	feedItem, feedItemStr, err := client.ResolveFlagFeedItemType(messageID, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: 无法自动判断 feed 层 item_type，跳过 feed 层取消: %v\n", err)
	} else {
		cancelOne(feedItem, feedItemStr, feedFlag, "feed")
	}

	if output == "json" {
		if err := printJSON(map[string]any{"results": results}); err != nil {
			return err
		}
		return lastErr
	}

	fmt.Printf("书签取消请求已执行：%d 层\n", len(results))
	for _, r := range results {
		fmt.Printf("  %s/%s: %s\n", r["item_type"], r["flag_type"], r["status"])
	}
	return lastErr
}

func init() {
	msgFlagCmd.AddCommand(msgFlagCancelCmd)
	msgFlagCancelCmd.Flags().String("item-type", "default", "item 类型：default | thread | msg_thread")
	msgFlagCancelCmd.Flags().String("flag-type", "message", "flag 类型：message | feed")
	msgFlagCancelCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	msgFlagCancelCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
