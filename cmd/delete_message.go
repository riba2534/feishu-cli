package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteMessageCmd = &cobra.Command{
	Use:   "delete <message_id>",
	Short: "删除消息",
	Long: `删除指定的消息。

参数:
  message_id    消息 ID（必填）
  --output, -o  输出格式（json）

注意:
  - 只能删除机器人发送的消息
  - 删除后消息不可恢复

示例:
  # 删除消息
  feishu-cli msg delete om_xxx

  # JSON 格式输出
  feishu-cli msg delete om_xxx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]

		err := client.DeleteMessage(messageID)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"success":    true,
				"message_id": messageID,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("消息删除成功！\n")
			fmt.Printf("  消息 ID: %s\n", messageID)
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(deleteMessageCmd)
	deleteMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
