package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventSchemaCmd = &cobra.Command{
	Use:   "schema <event_key>",
	Short: "查看某个 EventKey 的字段说明",
	Long: `查看指定 EventKey 的详细信息，包括：
  - EventType（飞书侧事件类型标识符）
  - 所需 scope（App / Tenant 权限）
  - Payload schema（事件 body 关键字段示例，部分 EventKey 提供）

示例:
  feishu-cli event schema im.message.receive_v1
  feishu-cli event schema im.message.receive_v1 --json`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		def, ok := event.Lookup(key)
		if !ok {
			return fmt.Errorf("未知 EventKey: %q（运行 `feishu-cli event list` 查看支持的 key）", key)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(def)
		}

		fmt.Printf("Key:          %s\n", def.Key)
		fmt.Printf("Event Type:   %s\n", def.EventType)
		fmt.Printf("Domain:       %s\n", def.Domain)
		fmt.Printf("Description:  %s\n", def.Description)
		if len(def.Scopes) > 0 {
			fmt.Printf("Scopes:       %s\n", strings.Join(def.Scopes, ", "))
		} else {
			fmt.Println("Scopes:       -")
		}
		if def.PayloadSchema != "" {
			fmt.Println("\nPayload Schema (示例):")
			for _, line := range strings.Split(def.PayloadSchema, "\n") {
				fmt.Printf("  %s\n", line)
			}
		} else {
			fmt.Println("\nPayload Schema: 未提供示例（订阅后实际 payload 详见飞书开放平台文档）")
		}
		return nil
	},
}

func init() {
	eventCmd.AddCommand(eventSchemaCmd)
	eventSchemaCmd.Flags().Bool("json", false, "以 JSON 输出 EventKey 定义（含 schema 原文）")
}
