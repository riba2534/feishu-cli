package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetWriteRichCmd = &cobra.Command{
	Use:   "write-rich <spreadsheet_token> <sheet_id>",
	Short: "写入富文本内容（V3 API）",
	Long: `使用 V3 API 写入富文本数据到电子表格。

数据格式（value_ranges JSON 数组）:
  [
    {
      "range": "SheetID!A1:B2",
      "values": [
        [  // 第一行
          [  // 第一列（单元格元素数组）
            {"type": "text", "text": {"text": "Hello"}}
          ],
          [  // 第二列
            {"type": "value", "value": {"value": "123"}}
          ]
        ]
      ]
    }
  ]

支持的元素类型:
  - text: 文本，支持局部样式
  - value: 数值
  - date_time: 日期时间
  - mention_user: @用户
  - mention_document: @文档
  - image: 图片
  - file: 附件
  - link: 链接
  - reminder: 提醒
  - formula: 公式

使用限制:
  - 单次请求的区域数量不超过 10 个
  - 单次写入的单元格不超过 5,000 个
  - 每个单元格不超过 50,000 字符

示例:
  # 从文件读取数据
  feishu-cli sheet write-rich shtcnxxxxxx 0b12 --data-file data.json

  # 命令行传入简单数据
  feishu-cli sheet write-rich shtcnxxxxxx 0b12 --data '[{"range":"0b12!A1:B1","values":[[[[{"type":"text","text":{"text":"Hello"}}]],[[{"type":"value","value":{"value":"123"}}]]]]}]'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		dataStr, _ := cmd.Flags().GetString("data")
		dataFile, _ := cmd.Flags().GetString("data-file")
		userIDType, _ := cmd.Flags().GetString("user-id-type")

		// 获取数据
		var jsonData string
		if dataFile != "" {
			data, err := os.ReadFile(dataFile)
			if err != nil {
				return fmt.Errorf("读取数据文件失败: %w", err)
			}
			jsonData = string(data)
		} else if dataStr != "" {
			jsonData = dataStr
		} else {
			return fmt.Errorf("请通过 --data 或 --data-file 指定数据")
		}

		// 解析 JSON 数据
		var valueRanges []*client.ValueRangeV3
		if err := json.Unmarshal([]byte(jsonData), &valueRanges); err != nil {
			return fmt.Errorf("解析数据失败（需要 value_ranges JSON 数组）: %w", err)
		}

		err := client.WriteCellsV3(client.Context(), spreadsheetToken, sheetID, valueRanges, userIDType)
		if err != nil {
			return err
		}

		fmt.Printf("写入成功！\n")
		fmt.Printf("  工作表: %s\n", sheetID)
		fmt.Printf("  写入范围数: %d\n", len(valueRanges))

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetWriteRichCmd)

	sheetWriteRichCmd.Flags().StringP("data", "d", "", "要写入的数据（value_ranges JSON 数组）")
	sheetWriteRichCmd.Flags().String("data-file", "", "数据文件路径")
	sheetWriteRichCmd.Flags().String("user-id-type", "", "用户 ID 类型: open_id, union_id, user_id")
}
