package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

		// Parse content based on type
		var blocks []*larkdocx.Block

		if contentType == "markdown" {
			// Convert Markdown to blocks
			opts := converter.ConvertOptions{
				DocumentID:   documentID,
				UploadImages: uploadImages,
			}
			conv := converter.NewMarkdownToBlock([]byte(contentData), opts, basePath)
			convertedBlocks, err := conv.Convert()
			if err != nil {
				return fmt.Errorf("转换 Markdown 失败: %w", err)
			}
			blocks = convertedBlocks
		} else {
			// Parse as JSON
			if err := json.Unmarshal([]byte(contentData), &blocks); err != nil {
				return fmt.Errorf("解析内容 JSON 失败: %w", err)
			}
		}

		if len(blocks) == 0 {
			return fmt.Errorf("没有内容可添加")
		}

		// If no block ID specified, use document root
		if blockID == "" {
			blockID = documentID
		}

		createdBlocks, err := client.CreateBlock(documentID, blockID, blocks, index)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(createdBlocks, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("成功添加 %d 个块!\n", len(createdBlocks))
			for i, block := range createdBlocks {
				if block.BlockId != nil {
					fmt.Printf("  [%d] 块ID: %s\n", i+1, *block.BlockId)
				}
			}
		}

		return nil
	},
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
