package cmd

import (
	"encoding/json"
	"fmt"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// callout 类型对应的背景色
// 飞书 Callout 背景色值: 1-灰色, 2-红色, 3-橙色, 4-黄色, 5-绿色, 6-蓝色, 7-紫色
var calloutTypeConfig = map[string]int{
	"info":    6, // 蓝色
	"warning": 4, // 黄色
	"error":   2, // 红色
	"success": 5, // 绿色
}

var addCalloutCmd = &cobra.Command{
	Use:   "add-callout <document_id> <content>",
	Short: "添加高亮块",
	Long: `向飞书文档添加高亮块（Callout）。

参数:
  <document_id>    文档 ID（必填）
  <content>        高亮块内容（必填）
  --parent-id      父块 ID，空表示根级别，默认空
  --index          插入位置索引，-1 表示末尾，默认 -1
  --callout-type   类型：info/warning/error/success，默认 info
  --icon           自定义图标（emoji shortcode，如 "bulb"）
  --output, -o     输出格式 (json)

高亮块类型:
  info      信息提示（蓝色背景，灯泡图标）
  warning   警告提示（黄色背景，警告图标）
  error     错误提示（红色背景，错误图标）
  success   成功提示（绿色背景，对勾图标）

示例:
  # 添加信息提示
  feishu-cli doc add-callout DOC_ID "这是一条提示信息"

  # 添加警告提示
  feishu-cli doc add-callout DOC_ID "请注意这个警告" --callout-type warning

  # 自定义图标
  feishu-cli doc add-callout DOC_ID "重要提示" --icon fire

  # 指定插入位置
  feishu-cli doc add-callout DOC_ID "内容" --parent-id BLOCK_ID --index 0`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		content := args[1]
		parentID, _ := cmd.Flags().GetString("parent-id")
		index, _ := cmd.Flags().GetInt("index")
		calloutType, _ := cmd.Flags().GetString("callout-type")
		icon, _ := cmd.Flags().GetString("icon")
		output, _ := cmd.Flags().GetString("output")

		// 获取 callout 类型配置
		bgColor, ok := calloutTypeConfig[calloutType]
		if !ok {
			bgColor = calloutTypeConfig["info"]
		}

		// 构建 callout 块
		blockType := 19 // Callout
		callout := &larkdocx.Callout{
			BackgroundColor: &bgColor,
		}

		// 如果指定了自定义图标，添加图标
		if icon != "" {
			callout.EmojiId = &icon
		}

		calloutBlock := &larkdocx.Block{
			BlockType: &blockType,
			Callout:   callout,
		}

		// 如果父块 ID 为空，使用文档根节点
		if parentID == "" {
			parentID = documentID
		}

		// 创建 callout 块
		createdBlocks, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{calloutBlock}, index)
		if err != nil {
			return err
		}

		if len(createdBlocks) == 0 {
			return fmt.Errorf("创建高亮块失败：未返回块信息")
		}

		// 获取创建的 callout 块 ID
		calloutBlockID := ""
		if createdBlocks[0].BlockId != nil {
			calloutBlockID = *createdBlocks[0].BlockId
		}

		// 在 callout 块内添加文本内容
		textBlockType := 2 // Text
		textBlock := &larkdocx.Block{
			BlockType: &textBlockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: &content,
						},
					},
				},
			},
		}

		_, err = client.CreateBlock(documentID, calloutBlockID, []*larkdocx.Block{textBlock}, 0)
		if err != nil {
			return fmt.Errorf("添加高亮块内容失败: %w", err)
		}

		if output == "json" {
			result := map[string]interface{}{
				"block_id":     calloutBlockID,
				"callout_type": calloutType,
				"content":      content,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("高亮块添加成功！\n")
			fmt.Printf("  块 ID: %s\n", calloutBlockID)
			fmt.Printf("  类型: %s\n", calloutType)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(addCalloutCmd)
	addCalloutCmd.Flags().String("parent-id", "", "父块 ID（默认: 文档根节点）")
	addCalloutCmd.Flags().Int("index", -1, "插入位置索引（-1 表示末尾）")
	addCalloutCmd.Flags().String("callout-type", "info", "高亮块类型 (info/warning/error/success)")
	addCalloutCmd.Flags().String("icon", "", "自定义图标（emoji shortcode）")
	addCalloutCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
