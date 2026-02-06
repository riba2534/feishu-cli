package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

var addContentCmd = &cobra.Command{
	Use:   "add <document_id> [source]",
	Short: "向文档添加内容块",
	Long: `向飞书文档添加内容块。

内容可以是 JSON 格式的块对象数组或 Markdown 格式文本。

参数:
  <document_id>        文档 ID（必填）
  [source]             源文件路径（与 --content 二选一）
  --content, -c        内容字符串
  --content-file       内容文件路径
  --content-type       内容类型：json/markdown，默认 json
  --source-type        源类型：file/content，默认 file
  --block-id, -b       父块 ID（默认: 文档根节点）
  --index, -i          插入位置索引（-1 表示末尾）
  --upload-images      上传 Markdown 中的本地图片
  --output, -o         输出格式 (json)

示例:
  # JSON 格式块（保持兼容）
  feishu-cli doc add DOC_ID --content '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"你好"}}]}}]'
  feishu-cli doc add DOC_ID --content-file blocks.json

  # Markdown 格式
  feishu-cli doc add DOC_ID README.md --content-type markdown
  feishu-cli doc add DOC_ID --content "# 标题\n这是内容" --content-type markdown --source-type content

  # 上传图片
  feishu-cli doc add DOC_ID doc.md --content-type markdown --upload-images

  # 指定插入位置
  feishu-cli doc add DOC_ID content.md --block-id PARENT_BLOCK_ID --index 0 --content-type markdown`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		contentStr, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		contentType, _ := cmd.Flags().GetString("content-type")
		sourceType, _ := cmd.Flags().GetString("source-type")
		blockID, _ := cmd.Flags().GetString("block-id")
		index, _ := cmd.Flags().GetInt("index")
		uploadImages, _ := cmd.Flags().GetBool("upload-images")
		output, _ := cmd.Flags().GetString("output")

		// Get source from args or flags
		var source string
		var basePath string

		if len(args) > 1 {
			// Source file from args
			source = args[1]
			basePath = filepath.Dir(source)
			if sourceType == "" {
				sourceType = "file"
			}
		} else if contentFile != "" {
			source = contentFile
			basePath = filepath.Dir(contentFile)
			sourceType = "file"
		} else if contentStr != "" {
			source = contentStr
			sourceType = "content"
		} else {
			return fmt.Errorf("必须指定源文件（第二个参数）、--content 或 --content-file")
		}

		// Get content
		var contentData string
		if sourceType == "file" {
			data, err := os.ReadFile(source)
			if err != nil {
				return fmt.Errorf("读取内容文件失败: %w", err)
			}
			contentData = string(data)
		} else {
			contentData = source
		}

		// If no block ID specified, use document root
		if blockID == "" {
			blockID = documentID
		}

		if contentType == "markdown" {
			return addContentMarkdown(documentID, blockID, contentData, basePath, uploadImages, index, output)
		}

		// JSON 模式
		var blocks []*larkdocx.Block
		if err := json.Unmarshal([]byte(contentData), &blocks); err != nil {
			return fmt.Errorf("解析内容 JSON 失败: %w", err)
		}
		if len(blocks) == 0 {
			return fmt.Errorf("没有内容可添加")
		}

		createdBlocks, err := client.CreateBlock(documentID, blockID, blocks, index)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(createdBlocks)
		}
		fmt.Printf("成功添加 %d 个块！\n", len(createdBlocks))
		for i, block := range createdBlocks {
			if block.BlockId != nil {
				fmt.Printf("  [%d] 块ID: %s\n", i+1, *block.BlockId)
			}
		}
		return nil
	},
}

// addContentMarkdown 处理 Markdown 模式的内容添加，支持嵌套结构、分批创建、表格 429 重试
func addContentMarkdown(documentID, blockID, contentData, basePath string, uploadImages bool, index int, output string) error {
	opts := converter.ConvertOptions{
		DocumentID:   documentID,
		UploadImages: uploadImages,
	}
	conv := converter.NewMarkdownToBlock([]byte(contentData), opts, basePath)
	result, err := conv.ConvertWithTableData()
	if err != nil {
		return fmt.Errorf("转换 Markdown 失败: %w", err)
	}

	if len(result.BlockNodes) == 0 {
		return fmt.Errorf("没有内容可添加")
	}

	// 提取顶层块，记录带有嵌套子块的节点
	var topLevelBlocks []*larkdocx.Block
	nodeChildrenMap := map[int][]*converter.BlockNode{} // 顶层索引 -> 嵌套子节点

	for i, node := range result.BlockNodes {
		topLevelBlocks = append(topLevelBlocks, node.Block)
		if len(node.Children) > 0 {
			nodeChildrenMap[i] = node.Children
		}
	}

	// 记录表格块的索引
	var tableIndices []int
	for i, block := range topLevelBlocks {
		if block.BlockType != nil && *block.BlockType == int(converter.BlockTypeTable) {
			tableIndices = append(tableIndices, i)
		}
	}

	// 批量添加顶层块（飞书 API 限制每次最多 50 个块）
	const batchSize = 50
	var createdBlockIDs []string
	totalCreated := 0
	currentIndex := index

	for i := 0; i < len(topLevelBlocks); i += batchSize {
		end := i + batchSize
		if end > len(topLevelBlocks) {
			end = len(topLevelBlocks)
		}
		batch := topLevelBlocks[i:end]

		createdBlocks, err := client.CreateBlock(documentID, blockID, batch, currentIndex)
		if err != nil {
			return fmt.Errorf("添加内容失败: %w", err)
		}
		totalCreated += len(createdBlocks)

		// 递增插入位置，避免多批次插入时顺序反转
		if currentIndex >= 0 {
			currentIndex += len(createdBlocks)
		}

		for _, block := range createdBlocks {
			if block.BlockId != nil {
				createdBlockIDs = append(createdBlockIDs, *block.BlockId)
			}
		}
	}

	// 递归创建嵌套子块（如嵌套列表）
	for idx, children := range nodeChildrenMap {
		if idx < len(createdBlockIDs) {
			parentID := createdBlockIDs[idx]
			nestedCount, nestedErr := createNestedChildren(documentID, parentID, children)
			if nestedErr != nil {
				fmt.Fprintf(os.Stderr, "[Warning] 嵌套子块创建失败: %v\n", nestedErr)
			}
			totalCreated += nestedCount
		}
	}

	// 填充表格内容（带 429 重试）
	tableSuccess := 0
	tableFailed := 0
	if len(tableIndices) > 0 && len(result.TableDatas) > 0 {
		tableDataIdx := 0
		for _, tableIdx := range tableIndices {
			if tableIdx >= len(createdBlockIDs) || tableDataIdx >= len(result.TableDatas) {
				break
			}
			tableBlockID := createdBlockIDs[tableIdx]
			if tableBlockID == "" {
				tableDataIdx++
				tableFailed++
				continue
			}
			td := result.TableDatas[tableDataIdx]
			tableDataIdx++

			if fillTableWithRetry(documentID, tableBlockID, td) {
				tableSuccess++
			} else {
				tableFailed++
			}
		}
	}

	// 输出结果
	if output == "json" {
		return printJSON(map[string]any{
			"document_id":   documentID,
			"blocks":        totalCreated,
			"table_total":   tableSuccess + tableFailed,
			"table_success": tableSuccess,
			"table_failed":  tableFailed,
		})
	}

	fmt.Printf("成功添加 %d 个块！\n", totalCreated)
	tableTotal := tableSuccess + tableFailed
	if tableTotal > 0 {
		fmt.Printf("  表格: %d/%d 成功\n", tableSuccess, tableTotal)
	}
	return nil
}

// fillTableWithRetry 填充单个表格内容，带 429 重试（最多 5 次，指数退避）
func fillTableWithRetry(documentID, tableBlockID string, td *converter.TableData) bool {
	const maxRetries = 5

	for retry := 0; retry <= maxRetries; retry++ {
		if retry > 0 {
			delay := time.Duration(retry) * 2 * time.Second
			time.Sleep(delay)
		}

		cellIDs, err := client.GetTableCellIDs(documentID, tableBlockID)
		if err != nil {
			if isRateLimitError(err) && retry < maxRetries {
				continue
			}
			fmt.Fprintf(os.Stderr, "[Warning] 获取表格 %s 单元格失败: %v\n", tableBlockID, err)
			return false
		}

		var fillErr error
		if len(td.CellElements) > 0 {
			fillErr = client.FillTableCellsRich(documentID, cellIDs, td.CellElements, td.CellContents)
		} else {
			fillErr = client.FillTableCells(documentID, cellIDs, td.CellContents)
		}
		if fillErr != nil {
			if isRateLimitError(fillErr) && retry < maxRetries {
				continue
			}
			fmt.Fprintf(os.Stderr, "[Warning] 填充表格 %s 内容失败: %v\n", tableBlockID, fillErr)
			return false
		}

		return true
	}

	return false
}

func init() {
	docCmd.AddCommand(addContentCmd)
	addContentCmd.Flags().StringP("content", "c", "", "要添加的块内容")
	addContentCmd.Flags().String("content-file", "", "包含块内容的文件")
	addContentCmd.Flags().String("content-type", "json", "内容类型 (json/markdown)")
	addContentCmd.Flags().String("source-type", "", "源类型 (file/content)")
	addContentCmd.Flags().StringP("block-id", "b", "", "父块ID (默认: 文档根节点)")
	addContentCmd.Flags().IntP("index", "i", -1, "插入位置索引 (-1 表示末尾)")
	addContentCmd.Flags().Bool("upload-images", false, "上传 Markdown 中的本地图片")
	addContentCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
