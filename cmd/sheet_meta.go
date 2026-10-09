package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetMetaCmd = &cobra.Command{
	Use:   "meta <spreadsheet_token|url>",
	Short: "获取表格元信息",
	Long: `获取电子表格的详细元信息，包括工作表列表、权限等。

示例:
  feishu-cli sheet meta shtcnxxxxxx
  feishu-cli sheet meta shtcnxxxxxx --ext-fields "protectedRange,mergedCell"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extFields, _ := cmd.Flags().GetString("ext-fields")
		output, _ := cmd.Flags().GetString("output")
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		meta, err := client.GetSpreadsheetMeta(client.Context(), target.Token, extFields, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(meta); err != nil {
				return err
			}
		} else {
			// 解析并显示主要信息
			if properties, ok := meta["properties"].(map[string]any); ok {
				fmt.Printf("电子表格属性:\n")
				if title, ok := properties["title"].(string); ok {
					fmt.Printf("  标题: %s\n", title)
				}
				// 数字以 json.Number 保留原始字面量（ownerUser 是 19 位整数，经 float64 会丢精度）
				if ownerUser, ok := properties["ownerUser"]; ok && ownerUser != nil {
					fmt.Printf("  所有者 ID: %v\n", ownerUser)
				}
				if sheetCount, ok := properties["sheetCount"]; ok && sheetCount != nil {
					fmt.Printf("  工作表数量: %v\n", sheetCount)
				}
				if revision, ok := properties["revision"]; ok && revision != nil {
					fmt.Printf("  版本: %v\n", revision)
				}
			}

			if sheets, ok := meta["sheets"].([]any); ok {
				fmt.Printf("\n工作表列表:\n")
				for i, s := range sheets {
					if sheet, ok := s.(map[string]any); ok {
						title := sheet["title"]
						sheetID := sheet["sheetId"]
						fmt.Printf("  %d. %v (ID: %v)\n", i+1, title, sheetID)
					}
				}
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetMetaCmd)

	sheetMetaCmd.Flags().String("ext-fields", "", "扩展字段（逗号分隔）: protectedRange, mergedCell")
	sheetMetaCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetMetaCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
