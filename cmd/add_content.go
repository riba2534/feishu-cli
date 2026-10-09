package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	Use:   "add <document_id|url> [source]",
	Short: "向文档添加内容块",
	Long: `向飞书文档添加内容块。

内容可以是 JSON 格式的块对象数组或 Markdown 格式文本。

参数:
  <document_id>        文档 ID 或 URL（必填；/wiki/ URL 自动解析为底层文档）
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

		contentStr, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		contentType, _ := cmd.Flags().GetString("content-type")
		sourceType, _ := cmd.Flags().GetString("source-type")
		blockID, _ := cmd.Flags().GetString("block-id")
		index, _ := cmd.Flags().GetInt("index")
		uploadImages, _ := cmd.Flags().GetBool("upload-images")
		output, _ := cmd.Flags().GetString("output")
		colWidthRaw, _ := cmd.Flags().GetString("table-column-width")
		colWidthMode, colWidthValues, errFlag := parseTableColumnWidthFlag(colWidthRaw)
		if errFlag != nil {
			return errFlag
		}
		userAccessToken := resolveOptionalUserToken(cmd)
		// 支持文档 ID、/docx/ URL 与 /wiki/ URL（wiki 自动解包为底层 docx 的 obj_token）
		documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
		if err != nil {
			return err
		}

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
			return addContentMarkdownWithOptions(documentID, blockID, contentData, basePath, uploadImages, index, output, userAccessToken, colWidthMode, colWidthValues)
		}

		// JSON 模式
		var blocks []*larkdocx.Block
		if err := json.Unmarshal([]byte(contentData), &blocks); err != nil {
			return fmt.Errorf("解析内容 JSON 失败: %w", err)
		}
		if len(blocks) == 0 {
			return fmt.Errorf("没有内容可添加")
		}

		createdBlocks, _, err := client.CreateBlock(documentID, blockID, blocks, index, userAccessToken)
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

// isEmptyTextBlock 判断一个块是否为空文本块（API 自动生成的占位块）。
// 只有 Elements 为空或全是 content="" 的 TextRun 才视为空块；
// 含有 MentionUser/MentionDoc/File 等非 TextRun 元素的块不视为空。
func isEmptyTextBlock(block *larkdocx.Block) bool {
	if block.BlockType == nil || *block.BlockType != int(converter.BlockTypeText) {
		return false
	}
	if block.Text == nil || len(block.Text.Elements) == 0 {
		return true
	}
	for _, elem := range block.Text.Elements {
		if elem.MentionUser != nil || elem.MentionDoc != nil || elem.File != nil ||
			elem.Reminder != nil || elem.InlineBlock != nil || elem.Equation != nil ||
			elem.Undefined != nil {
			return false
		}
		if elem.TextRun != nil && elem.TextRun.Content != nil && *elem.TextRun.Content != "" {
			return false
		}
	}
	return true
}

// deleteContainerAutoEmptyBlock 删除 QuoteContainer/Callout 容器块中飞书 API 自动生成的空文本子块。
// 飞书 API 在创建容器块时会异步在 index 0 插入一个空 Text 块，导致渲染时顶部出现多余空行。
// 必须在 createNestedChildren 完成后调用；仅检查 index 0，不会误删段落间空行。
// 对非容器块类型直接 no-op，由调用方无条件传入 block type 即可。
func deleteContainerAutoEmptyBlock(documentID, parentID string, blockType int, userAccessToken string) {
	var blockTypeName string
	switch blockType {
	case int(converter.BlockTypeQuoteContainer):
		blockTypeName = "QuoteContainer"
	case int(converter.BlockTypeCallout):
		blockTypeName = "Callout"
	case int(converter.BlockTypeGridColumn):
		blockTypeName = "GridColumn"
	default:
		return
	}

	// 飞书 API 异步插入空子块，首次未命中时等待后重试。
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(300 * time.Millisecond)
		}

		childrenResult := client.DoWithRetry(func() ([]*larkdocx.Block, http.Header, error) {
			return client.GetBlockChildren(documentID, parentID, userAccessToken)
		}, client.RetryConfig{
			MaxRetries:       3,
			RetryOnRateLimit: true,
		})
		if childrenResult.Err != nil || len(childrenResult.Value) == 0 {
			continue
		}

		firstChild := childrenResult.Value[0]
		if !isEmptyTextBlock(firstChild) {
			continue
		}

		delResult := client.DoWithRetry(func() (struct{}, http.Header, error) {
			headers, err := client.DeleteBlocks(documentID, parentID, 0, 1, userAccessToken)
			return struct{}{}, headers, err
		}, client.RetryConfig{
			MaxRetries:       5,
			RetryOnRateLimit: true,
		})
		if delResult.Err != nil {
			fmt.Fprintf(os.Stderr, "[Warning] %s 空子块删除失败 (parent=%s): %v\n", blockTypeName, parentID, delResult.Err)
		}
		return
	}
}

// addContentMarkdownWithOptions 处理 Markdown 模式的内容添加，支持嵌套结构、分批创建、表格 429 重试。
// colWidthMode/colWidthValues 来自 --table-column-width flag（见 cmd/table_column_width.go）。
func addContentMarkdownWithOptions(documentID, blockID, contentData, basePath string, uploadImages bool, index int, output, userAccessToken, colWidthMode string, colWidthValues []int) error {
	opts := converter.ConvertOptions{
		DocumentID:   documentID,
		UploadImages: uploadImages,
	}
	applyColumnWidthOptions(&opts, colWidthMode, colWidthValues)
	conv := converter.NewMarkdownToBlock([]byte(contentData), opts, basePath)
	result, err := conv.ConvertWithTableData()
	if err != nil {
		return fmt.Errorf("转换 Markdown 失败: %w", err)
	}

	if len(result.BlockNodes) == 0 {
		return fmt.Errorf("没有内容可添加")
	}

	for _, d := range result.Degradations {
		fmt.Fprintf(os.Stderr, "[Warning] %s %s: %s\n", d.Kind, d.Source, d.Reason)
	}

	topLevelBlocks := make([]*larkdocx.Block, len(result.BlockNodes))
	for i, node := range result.BlockNodes {
		topLevelBlocks[i] = node.Block
	}

	// 批量添加顶层块（飞书 API 限制每次最多 50 个块）
	const batchSize = 50
	var createdTop []*larkdocx.Block
	totalCreated := 0
	currentIndex := index

	for i := 0; i < len(topLevelBlocks); i += batchSize {
		end := min(i+batchSize, len(topLevelBlocks))
		batch := topLevelBlocks[i:end]

		createdBlocks, _, err := client.CreateBlock(documentID, blockID, batch, currentIndex, userAccessToken)
		if err != nil {
			return fmt.Errorf("添加内容失败: %w", err)
		}
		totalCreated += len(createdBlocks)

		// 递增插入位置，避免多批次插入时顺序反转
		if currentIndex >= 0 {
			currentIndex += len(createdBlocks)
		}
		createdTop = append(createdTop, createdBlocks...)
	}

	// 递归创建嵌套子块（如嵌套列表、分栏列内容），按树序收集已创建节点
	var createdAll []createdBlockNode
	for i, node := range result.BlockNodes {
		if i >= len(createdTop) || createdTop[i] == nil || createdTop[i].BlockId == nil {
			continue
		}
		top := createdBlockNode{node: node, blockID: *createdTop[i].BlockId, parentID: blockID, created: createdTop[i]}
		createdAll = append(createdAll, top)
		nestedCount, nestedCreated, nestedErr := createChildrenOf(documentID, top, userAccessToken)
		if nestedErr != nil {
			fmt.Fprintf(os.Stderr, "[Warning] 嵌套子块创建失败: %v\n", nestedErr)
		}
		totalCreated += nestedCount
		createdAll = append(createdAll, nestedCreated...)
	}

	// QuoteContainer / Callout：遍历所有顶层节点清理飞书 API 异步生成的空子块。
	// 必须在所有嵌套子块创建完成后执行，确保 API 已稳定。
	for i, node := range result.BlockNodes {
		if i >= len(createdTop) || createdTop[i] == nil || createdTop[i].BlockId == nil {
			continue
		}
		if node.Block.BlockType != nil {
			deleteContainerAutoEmptyBlock(documentID, *createdTop[i].BlockId, *node.Block.BlockType, userAccessToken)
		}
	}

	// 填充表格内容（带 429 重试）：按块指针取填充数据，覆盖分栏等容器内的表格
	tableSuccess := 0
	tableFailed := 0
	for _, cn := range createdAll {
		td := result.TableDataByBlock[cn.node.Block]
		if td == nil {
			continue
		}
		if fillTableWithRetry(documentID, cn.blockID, td, userAccessToken) {
			tableSuccess++
		} else {
			tableFailed++
		}
	}

	// 补齐资源：图片（本地/网络/素材 token 复用）、附件与视频、带 token 的画板复制。
	// 转换阶段只创建了空块，块 ID 在创建后才能回填素材 token。
	var media mediaTaskSet
	collectMediaTasks(&media, result.MediaRefs, createdAll, basePath)
	imageSuccess := 0
	imageFailed := 0
	for _, task := range media.images {
		res := processImageTask(documentID, task, false, userAccessToken)
		if res.success {
			imageSuccess++
		} else {
			imageFailed++
		}
	}
	fileSuccess := 0
	fileFailed := 0
	for _, task := range media.files {
		if res := processVideoTask(documentID, task, false, userAccessToken); res.success {
			fileSuccess++
		} else {
			fileFailed++
		}
	}
	for _, task := range media.boards {
		if _, err := processBoardCopyTask(documentID, task, false, userAccessToken); err == nil {
			fileSuccess++
		} else {
			fileFailed++
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
			"image_total":   imageSuccess + imageFailed,
			"image_success": imageSuccess,
			"image_failed":  imageFailed,
			"media_total":   fileSuccess + fileFailed,
			"media_success": fileSuccess,
			"media_failed":  fileFailed,
		})
	}

	fmt.Printf("成功添加 %d 个块！\n", totalCreated)
	tableTotal := tableSuccess + tableFailed
	if tableTotal > 0 {
		fmt.Printf("  表格: %d/%d 成功\n", tableSuccess, tableTotal)
	}
	imageTotal := imageSuccess + imageFailed
	if imageTotal > 0 {
		fmt.Printf("  图片: %d/%d 成功\n", imageSuccess, imageTotal)
	}
	if mediaTotal := fileSuccess + fileFailed; mediaTotal > 0 {
		fmt.Printf("  附件/视频/画板: %d/%d 成功\n", fileSuccess, mediaTotal)
	}
	return nil
}

// fillTableWithRetry 填充单个表格内容，带重试（最多 5 次）。
// 委托给共享的 fillTableWithExtraRows，保证：
//   - 行数超过 9 行时通过 insert_table_row 追加到同一个 block（视觉连贯）
//   - 重试幂等，不会因 retry 而重复追加行
//   - cellMap 传 nil 即可：下层在重试闭包内按本表格局部补建映射，
//     使 batch_update 批量填充在本路径生效（issue #172）
func fillTableWithRetry(documentID, tableBlockID string, td *converter.TableData, userAccessToken string) bool {
	// 追加行数 ≥ 5 时每 5 行打一次进度
	onProgress := tableAppendProgress(len(td.ExtraRowContents), 5, 5, func(appended, total int) {
		fmt.Printf("  [表格] 追加行 %d/%d\n", appended, total)
	})

	result := client.DoVoidWithRetry(func() (http.Header, error) {
		return nil, fillTableWithExtraRows(documentID, tableBlockID, td, userAccessToken, onProgress, nil)
	}, client.RetryConfig{
		MaxRetries:       5,
		RetryOnRateLimit: true,
	})

	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "[Warning] 表格 %s: %v\n", tableBlockID, result.Err)
		return false
	}
	return true
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
	addContentCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文档）")
	addContentCmd.Flags().String("table-column-width", "auto",
		"Markdown 表格列宽策略：auto | fixed | 像素列表如 80,200,*,120（仅 markdown 内容类型生效）")
}
