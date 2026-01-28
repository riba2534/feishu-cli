package cmd

import (
	"fmt"
	"os"
	"regexp"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

var exportMarkdownCmd = &cobra.Command{
	Use:   "export <document_id|url>",
	Short: "导出文档为 Markdown",
	Long: `将飞书文档导出为 Markdown 格式。

支持通过文档 ID 或 URL 导出：
  feishu-cli doc export ABC123def456
  feishu-cli doc export https://xxx.feishu.cn/docx/ABC123def456
  feishu-cli doc export https://xxx.larkoffice.com/docx/ABC123def456

示例:
  feishu-cli doc export ABC123def456
  feishu-cli doc export ABC123def456 --output doc.md
  feishu-cli doc export ABC123def456 --download-images --assets-dir ./images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID, err := extractDocToken(args[0])
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		downloadImages, _ := cmd.Flags().GetBool("download-images")
		assetsDir, _ := cmd.Flags().GetString("assets-dir")

		// Get all blocks
		blocks, err := client.GetAllBlocks(documentID)
		if err != nil {
			return fmt.Errorf("获取块失败: %w", err)
		}

		// Convert to Markdown
		options := converter.ConvertOptions{
			DownloadImages: downloadImages,
			AssetsDir:      assetsDir,
			DocumentID:     documentID,
		}

		conv := converter.NewBlockToMarkdown(blocks, options)
		markdown, err := conv.Convert()
		if err != nil {
			return fmt.Errorf("转换为 Markdown 失败: %w", err)
		}

		// Output
		if output != "" {
			if err := os.WriteFile(output, []byte(markdown), 0644); err != nil {
				return fmt.Errorf("写入输出文件失败: %w", err)
			}
			fmt.Printf("已导出到 %s\n", output)
		} else {
			fmt.Print(markdown)
		}

		return nil
	},
}

// extractDocToken 从 URL 或直接的 token 中提取 document_id
func extractDocToken(input string) (string, error) {
	// 尝试匹配 docx URL
	re := regexp.MustCompile(`/docx/([a-zA-Z0-9]+)`)
	matches := re.FindStringSubmatch(input)
	token := input
	if len(matches) > 1 {
		token = matches[1]
	}

	// 验证 token 格式
	if !isValidToken(token) {
		return "", fmt.Errorf("无效的文档 token: %s", token)
	}

	return token, nil
}

func init() {
	docCmd.AddCommand(exportMarkdownCmd)
	exportMarkdownCmd.Flags().StringP("output", "o", "", "输出文件路径")
	exportMarkdownCmd.Flags().Bool("download-images", false, "下载图片到本地目录")
	exportMarkdownCmd.Flags().String("assets-dir", "./assets", "下载资源的保存目录")
}
