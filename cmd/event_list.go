package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/event"
	"github.com/spf13/cobra"
)

var eventListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有支持的 EventKey",
	Long: `列出 feishu-cli event 模块当前支持订阅的所有 EventKey，按 domain 分组展示。

输出格式:
  默认：人类可读表格（按 domain 分组）
  --json：机器可读 JSON 数组（每个元素含 key/event_type/domain/scopes/description）

示例:
  # 表格视图（默认）
  feishu-cli event list

  # JSON 输出，用 jq 提取 IM 域所有 EventKey
  feishu-cli event list --json | jq -r '.[] | select(.domain=="im") | .key'`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		all := event.ListAll()

		if asJSON {
			return printJSON(all)
		}

		printEventListTable(all)
		return nil
	},
}

// printEventListTable 按 domain 分组打印 EventKey 表格到 stdout。
// 列宽自适应，最后一列（描述）不补尾部空格。
func printEventListTable(all []event.KeyDefinition) {
	// 按 domain 分组
	byDomain := map[string][]event.KeyDefinition{}
	for _, def := range all {
		byDomain[def.Domain] = append(byDomain[def.Domain], def)
	}
	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	// 全局列宽（保证不同 domain 表格对齐）
	keyWidth := len("EVENT_KEY")
	scopeWidth := len("SCOPES")
	for _, def := range all {
		if l := len(def.Key); l > keyWidth {
			keyWidth = l
		}
		if l := len(strings.Join(def.Scopes, ",")); l > scopeWidth {
			scopeWidth = l
		}
	}

	fmt.Printf("%-*s  %-*s  %s\n", keyWidth, "EVENT_KEY", scopeWidth, "SCOPES", "DESCRIPTION")

	for _, d := range domains {
		fmt.Printf("\n── %s ──\n", d)
		// domain 内按 key 排序
		group := byDomain[d]
		sort.Slice(group, func(i, j int) bool { return group[i].Key < group[j].Key })
		for _, def := range group {
			scopes := strings.Join(def.Scopes, ",")
			if scopes == "" {
				scopes = "-"
			}
			fmt.Printf("%-*s  %-*s  %s\n", keyWidth, def.Key, scopeWidth, scopes, def.Description)
		}
	}
	fmt.Fprintln(os.Stderr, "\n用 `feishu-cli event schema <key>` 查看 payload 字段；`feishu-cli event consume <key>` 开始订阅。")
}

func init() {
	eventCmd.AddCommand(eventListCmd)
	eventListCmd.Flags().Bool("json", false, "以 JSON 数组输出（机器可读）")
}
