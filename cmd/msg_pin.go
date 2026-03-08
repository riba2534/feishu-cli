package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgPinCmd = &cobra.Command{
	Use:   "pin <message_id>",
	Short: "置顶消息",
	Long: `置顶指定的消息。

参数:
  message_id    消息 ID（必填）

示例:
  feishu-cli msg pin om_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]

		if err := client.PinMessage(messageID, token); err != nil {
			return err
		}

		fmt.Printf("消息置顶成功！\n")
		fmt.Printf("  消息 ID: %s\n", messageID)

		return nil
	},
}

var msgUnpinCmd = &cobra.Command{
	Use:   "unpin <message_id>",
	Short: "取消置顶消息",
	Long: `取消置顶指定的消息。

参数:
  message_id    消息 ID（必填）

示例:
  feishu-cli msg unpin om_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]

		if err := client.UnpinMessage(messageID, token); err != nil {
			return err
		}

		fmt.Printf("消息取消置顶成功！\n")
		fmt.Printf("  消息 ID: %s\n", messageID)

		return nil
	},
}

var msgPinsCmd = &cobra.Command{
	Use:   "pins",
	Short: "获取群内置顶消息列表",
	Long: `获取指定群聊内的置顶消息列表。

参数:
  --chat-id       群 ID（必填）
  --start-time    起始时间（毫秒级时间戳）
  --end-time      结束时间（毫秒级时间戳）
  --page-size     每页数量
  --page-token    分页标记

示例:
  feishu-cli msg pins --chat-id oc_xxx
  feishu-cli msg pins --chat-id oc_xxx --page-size 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		chatID, _ := cmd.Flags().GetString("chat-id")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.ListPins(chatID, startTime, endTime, pageToken, pageSize, token)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	msgCmd.AddCommand(msgPinCmd)
	msgPinCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	msgCmd.AddCommand(msgUnpinCmd)
	msgUnpinCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	msgCmd.AddCommand(msgPinsCmd)
	msgPinsCmd.Flags().String("chat-id", "", "群 ID")
	msgPinsCmd.Flags().String("start-time", "", "起始时间（毫秒级时间戳）")
	msgPinsCmd.Flags().String("end-time", "", "结束时间（毫秒级时间戳）")
	msgPinsCmd.Flags().Int("page-size", 0, "每页数量")
	msgPinsCmd.Flags().String("page-token", "", "分页标记")
	msgPinsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(msgPinsCmd, "chat-id")
}
