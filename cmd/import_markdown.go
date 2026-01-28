package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

// segment 表示 Markdown 中的一个片段
type segment struct {
	kind    string // "markdown" 或 "mermaid"
	content string
}

// parseMarkdownSegments 将 Markdown 解析为片段，分离出 mermaid 代码块
func parseMarkdownSegments(markdown string) []segment {
	var segments []segment
	lines := strings.Split(markdown, "\n")
	var buf []string
	i := 0

	for i < len(lines) {
		line := lines[i]
		// 检查是否是 mermaid 代码块开始
		if strings.HasPrefix(strings.TrimSpace(line), "```mermaid") {
			// 先保存之前的普通内容
			if len(buf) > 0 {
				segments = append(segments, segment{kind: "markdown", content: strings.Join(buf, "\n")})
				buf = nil
			}

			// 收集 mermaid 代码块内容
			i++
			var mermaidLines []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				mermaidLines = append(mermaidLines, lines[i])
				i++
			}
			// 跳过结束的 ```
			if i < len(lines) {
				i++
			}

			if len(mermaidLines) > 0 {
				segments = append(segments, segment{kind: "mermaid", content: strings.Join(mermaidLines, "\n")})
			}
		} else {
			buf = append(buf, line)
			i++
		}
	}

	// 保存剩余的普通内容
	if len(buf) > 0 {
		segments = append(segments, segment{kind: "markdown", content: strings.Join(buf, "\n")})
	}

	return segments
}

// countMermaidBlocks 统计 mermaid 代码块数量
func countMermaidBlocks(markdown string) int {
	re := regexp.MustCompile("(?m)^```mermaid")
	return len(re.FindAllString(markdown, -1))
}

var importMarkdownCmd = &cobra.Command{
	Use:   "import <file.md>",
	Short: "从 Markdown 导入创建/更新文档",
	Long: `从 Markdown 文件导入内容，创建新的飞书文档或更新已有文档。

特性:
  - 支持 Mermaid 图表自动转换为飞书画板
  - 支持表格、代码块、列表等标准 Markdown 语法
  - 大文件自动分段写入

示例:
  feishu-cli doc import doc.md --title "我的文档"
  feishu-cli doc import doc.md --document-id ABC123def456
  feishu-cli doc import doc.md --title "我的文档" --upload-images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath := args[0]
		title, _ := cmd.Flags().GetString("title")
		documentID, _ := cmd.Flags().GetString("document-id")
		uploadImages, _ := cmd.Flags().GetBool("upload-images")
		folder, _ := cmd.Flags().GetString("folder")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Read markdown file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}

		basePath := filepath.Dir(filePath)
		markdownText := string(content)

		// 统计 mermaid 数量
		mermaidCount := countMermaidBlocks(markdownText)
		if verbose && mermaidCount > 0 {
			fmt.Printf("[信息] 检测到 %d 个 Mermaid 图表\n", mermaidCount)
		}

		// If no document ID, create new document
		if documentID == "" {
			if title == "" {
				// Use filename as title
				title = filepath.Base(filePath)
				ext := filepath.Ext(title)
				if len(ext) < len(title) {
					title = title[:len(title)-len(ext)]
				}
				// If title is still empty, use a default
				if title == "" {
					title = "无标题文档"
				}
			}

			doc, err := client.CreateDocument(title, folder)
			if err != nil {
				return fmt.Errorf("创建文档失败: %w", err)
			}
			if doc.DocumentId == nil {
				return fmt.Errorf("文档已创建但未返回ID")
			}
			documentID = *doc.DocumentId
			fmt.Printf("已创建文档: %s\n", documentID)
			fmt.Printf("链接: https://feishu.cn/docx/%s\n", documentID)
		}

		// 解析 Markdown 为片段
		segments := parseMarkdownSegments(markdownText)

		var totalBlocks int
		var mermaidSuccess int
		mermaidIdx := 0

		var tableSuccess, tableFailed int

		for _, seg := range segments {
			if seg.kind == "markdown" {
				// 普通 Markdown 内容，转换为块并添加
				if strings.TrimSpace(seg.content) == "" {
					continue
				}

				options := converter.ConvertOptions{
					UploadImages: uploadImages,
					DocumentID:   documentID,
				}

				conv := converter.NewMarkdownToBlock([]byte(seg.content), options, basePath)
				result, err := conv.ConvertWithTableData()
				if err != nil {
					return fmt.Errorf("转换 Markdown 失败: %w", err)
				}

				if len(result.Blocks) == 0 {
					continue
				}

				// 记录表格块的索引，用于后续填充内容
				var tableIndices []int
				for i, block := range result.Blocks {
					if block.BlockType != nil && *block.BlockType == 31 { // BlockTypeTable
						tableIndices = append(tableIndices, i)
					}
				}

				// 批量添加块（飞书 API 限制每次最多 50 个块）
				const batchSize = 50
				var createdBlockIDs []string
				for i := 0; i < len(result.Blocks); i += batchSize {
					end := i + batchSize
					if end > len(result.Blocks) {
						end = len(result.Blocks)
					}
					batch := result.Blocks[i:end]

					createdBlocks, err := client.CreateBlock(documentID, documentID, batch, -1)
					if err != nil {
						return fmt.Errorf("添加内容失败: %w", err)
					}
					totalBlocks += len(createdBlocks)

					// 收集创建的块 ID
					for _, block := range createdBlocks {
						if block.BlockId != nil {
							createdBlockIDs = append(createdBlockIDs, *block.BlockId)
						}
					}
				}

				// 填充表格内容
				tableDataIdx := 0
				for _, tableIdx := range tableIndices {
					if tableIdx >= len(createdBlockIDs) || tableDataIdx >= len(result.TableDatas) {
						continue
					}

					tableBlockID := createdBlockIDs[tableIdx]
					tableData := result.TableDatas[tableDataIdx]
					tableDataIdx++

					if verbose {
						fmt.Printf("[表格] 填充表格内容 (%d×%d)...\n", tableData.Rows, tableData.Cols)
					}

					// 获取表格单元格 ID
					cellIDs, err := client.GetTableCellIDs(documentID, tableBlockID)
					if err != nil {
						if verbose {
							fmt.Printf("  ⚠ 获取表格单元格失败: %v\n", err)
						}
						tableFailed++
						continue
					}

					// 填充单元格内容
					if err := client.FillTableCells(documentID, cellIDs, tableData.CellContents); err != nil {
						if verbose {
							fmt.Printf("  ⚠ 填充表格内容失败: %v\n", err)
						}
						tableFailed++
						continue
					}

					tableSuccess++
					if verbose {
						fmt.Printf("  ✓ 表格填充成功\n")
					}

					// 避免频控
					time.Sleep(300 * time.Millisecond)
				}

			} else if seg.kind == "mermaid" {
				mermaidIdx++
				if verbose {
					fmt.Printf("[渲染] Mermaid 图表 %d/%d...\n", mermaidIdx, mermaidCount)
				}

				// 创建画板
				boardResult, err := client.AddBoard(documentID, "", -1)
				if err != nil {
					fmt.Printf("⚠ Mermaid 图表 %d 创建画板失败: %v\n", mermaidIdx, err)
					continue
				}

				if boardResult.WhiteboardID == "" {
					fmt.Printf("⚠ Mermaid 图表 %d 未返回画板 ID\n", mermaidIdx)
					continue
				}

				// 导入 mermaid 图表到画板（带重试机制处理服务端 500 错误）
				opts := client.ImportDiagramOptions{
					SourceType: "content",
					Syntax:     "mermaid",
				}

				var importErr error
				maxRetries := 3
				for retry := 0; retry < maxRetries; retry++ {
					_, importErr = client.ImportDiagram(boardResult.WhiteboardID, seg.content, opts)
					if importErr == nil {
						break
					}
					// 检查是否是服务端 500 错误（临时性错误，值得重试）
					if strings.Contains(importErr.Error(), "500") || strings.Contains(importErr.Error(), "internal error") {
						if verbose && retry < maxRetries-1 {
							fmt.Printf("  ⚠ 服务端错误，%d 秒后重试 (%d/%d)...\n", (retry+1)*2, retry+1, maxRetries-1)
						}
						time.Sleep(time.Duration((retry+1)*2) * time.Second)
						continue
					}
					// 非 500 错误，不重试
					break
				}
				if importErr != nil {
					fmt.Printf("⚠ Mermaid 图表 %d 导入失败: %v\n", mermaidIdx, importErr)
					continue
				}

				mermaidSuccess++
				totalBlocks++

				if verbose {
					fmt.Printf("  ✓ Mermaid 图表 %d 成功\n", mermaidIdx)
				}

				// 避免频控，等待一段时间
				if mermaidIdx < mermaidCount {
					time.Sleep(2 * time.Second)
				}
			}
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"document_id":     documentID,
				"blocks":          totalBlocks,
				"mermaid_total":   mermaidCount,
				"mermaid_success": mermaidSuccess,
				"table_success":   tableSuccess,
				"table_failed":    tableFailed,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("导入成功!\n")
			fmt.Printf("  文档ID: %s\n", documentID)
			fmt.Printf("  添加块数: %d\n", totalBlocks)
			if tableSuccess > 0 || tableFailed > 0 {
				fmt.Printf("  表格: %d 成功, %d 失败\n", tableSuccess, tableFailed)
			}
			if mermaidCount > 0 {
				fmt.Printf("  Mermaid图表: %d/%d 成功\n", mermaidSuccess, mermaidCount)
			}
			fmt.Printf("  链接: https://feishu.cn/docx/%s\n", documentID)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(importMarkdownCmd)
	importMarkdownCmd.Flags().StringP("title", "t", "", "文档标题 (用于新建文档)")
	importMarkdownCmd.Flags().StringP("document-id", "d", "", "已有文档ID (用于更新)")
	importMarkdownCmd.Flags().Bool("upload-images", true, "上传本地图片")
	importMarkdownCmd.Flags().StringP("folder", "f", "", "新文档的文件夹 Token")
	importMarkdownCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	importMarkdownCmd.Flags().BoolP("verbose", "v", false, "显示详细进度")
}
