package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

var importMarkdownCmd = &cobra.Command{
	Use:   "import <file.md>",
	Short: "从 Markdown 导入创建/更新文档",
	Long: `从 Markdown 文件导入内容，创建新的飞书文档或更新已有文档。

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

		// Read markdown file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}

		basePath := filepath.Dir(filePath)

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
		}

		// Convert markdown to blocks
		options := converter.ConvertOptions{
			UploadImages: uploadImages,
			DocumentID:   documentID,
		}

		conv := converter.NewMarkdownToBlock(content, options, basePath)
		blocks, err := conv.Convert()
		if err != nil {
			return fmt.Errorf("转换 Markdown 失败: %w", err)
		}

		// Add blocks to document
		if len(blocks) > 0 {
			createdBlocks, err := client.CreateBlock(documentID, documentID, blocks, -1)
			if err != nil {
				return fmt.Errorf("添加内容失败: %w", err)
			}

			output, _ := cmd.Flags().GetString("output")
			if output == "json" {
				data, _ := json.MarshalIndent(map[string]interface{}{
					"document_id": documentID,
					"blocks":      len(createdBlocks),
				}, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("导入成功!\n")
				fmt.Printf("  文档ID: %s\n", documentID)
				fmt.Printf("  添加块数: %d\n", len(createdBlocks))
				fmt.Printf("  链接: https://feishu.cn/docx/%s\n", documentID)
			}
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
}
