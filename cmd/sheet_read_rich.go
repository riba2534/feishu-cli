package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetReadRichCmd = &cobra.Command{
	Use:   "read-rich <spreadsheet_token> <sheet_id> <range1> [range2...]",
	Short: "获取富文本内容（V3 API）",
	Long: `使用 V3 API 批量获取工作表的富文本内容。

范围格式:
  SheetID!A1:B2    - 指定工作表的范围
  A1:B2            - 当前工作表的范围

特点:
  - 支持批量获取多个范围
  - 返回结构化的富文本内容，包含类型信息
  - 支持的元素类型: text, mention_user, mention_document, value, date_time, image, file, link, reminder, formula

示例:
  # 获取单个范围
  feishu-cli sheet read-rich shtcnxxxxxx 0b12 "0b12!A1:C10"

  # 获取多个范围
  feishu-cli sheet read-rich shtcnxxxxxx 0b12 "0b12!A1:A1" "0b12!G2:G2"

  # 指定日期时间渲染选项
  feishu-cli sheet read-rich shtcnxxxxxx 0b12 "0b12!A1:C10" --datetime-render formatted_string`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		ranges := args[2:]
		dateTimeRender, _ := cmd.Flags().GetString("datetime-render")
		valueRender, _ := cmd.Flags().GetString("value-render")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		outputFormat, _ := cmd.Flags().GetString("output")

		// 处理 shell 转义
		for i := range ranges {
			ranges[i] = unescapeSheetRange(ranges[i])
		}

		result, err := client.ReadCellsRichV3(client.Context(), spreadsheetToken, sheetID, ranges, dateTimeRender, valueRender, userIDType)
		if err != nil {
			return err
		}

		if outputFormat == "json" {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
		} else {
			for _, cellRange := range result {
				fmt.Printf("范围: %s\n", cellRange.Range)
				if len(cellRange.Values) == 0 {
					fmt.Println("  （空数据）")
					continue
				}
				for i, row := range cellRange.Values {
					fmt.Printf("  行 %d:\n", i+1)
					for j, cell := range row {
						fmt.Printf("    列 %d: ", j+1)
						// 打印每个元素
						if len(cell) == 0 {
							fmt.Println("(空)")
							continue
						}
						for k, elem := range cell {
							if k > 0 {
								fmt.Print(" + ")
							}
							printCellElement(elem)
						}
						fmt.Println()
					}
				}
				fmt.Println()
			}
		}

		return nil
	},
}

func printCellElement(elem *client.CellElement) {
	switch elem.Type {
	case "text":
		if elem.Text != nil {
			fmt.Printf("[text: %q]", elem.Text.Text)
		}
	case "value":
		if elem.Value != nil {
			fmt.Printf("[value: %s]", elem.Value.Value)
		}
	case "date_time":
		if elem.DateTime != nil {
			fmt.Printf("[datetime: %s]", elem.DateTime.DateTime)
		}
	case "mention_user":
		if elem.MentionUser != nil {
			fmt.Printf("[mention_user: %s (%s)]", elem.MentionUser.Name, elem.MentionUser.UserID)
		}
	case "mention_document":
		if elem.MentionDocument != nil {
			fmt.Printf("[mention_doc: %s (%s)]", elem.MentionDocument.Title, elem.MentionDocument.Token)
		}
	case "image":
		if elem.Image != nil {
			fmt.Printf("[image: %s]", elem.Image.ImageToken)
		}
	case "file":
		if elem.File != nil {
			fmt.Printf("[file: %s (%s)]", elem.File.Name, elem.File.FileToken)
		}
	case "link":
		if elem.Link != nil {
			fmt.Printf("[link: %s -> %s]", elem.Link.Text, elem.Link.Link)
		}
	case "formula":
		if elem.Formula != nil {
			fmt.Printf("[formula: %s = %s]", elem.Formula.Formula, elem.Formula.FormulaValue)
		}
	case "reminder":
		if elem.Reminder != nil {
			fmt.Printf("[reminder: %s]", elem.Reminder.NotifyDateTime)
		}
	default:
		fmt.Printf("[%s]", elem.Type)
	}
}

func init() {
	sheetCmd.AddCommand(sheetReadRichCmd)

	sheetReadRichCmd.Flags().String("datetime-render", "", "日期时间渲染选项: formatted_string, serial_number")
	sheetReadRichCmd.Flags().String("value-render", "", "数值渲染选项: formatted_value, unformatted_value")
	sheetReadRichCmd.Flags().String("user-id-type", "", "用户 ID 类型: open_id, union_id, user_id")
	sheetReadRichCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
}
