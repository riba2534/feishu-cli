package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

var exportWikiCmd = &cobra.Command{
	Use:   "export <node_token|url>",
	Short: "导出知识库文档为 Markdown",
	Long: `导出知识库文档为 Markdown 文件。

支持的文档类型:
  docx      新版文档（完整支持）
  doc       旧版文档（暂不支持）

工作流程:
  1. 获取节点信息，获取实际文档 Token
  2. 调用文档 API 获取所有块
  3. 转换为 Markdown 格式
  4. 保存到本地文件

参数:
  node_token        节点 Token
  url               知识库文档 URL
  --output, -o      输出文件路径
  --download-images 下载文档中的图片

示例:
  # 导出到默认路径
  feishu-cli wiki export Ad8Iw0oz3iSp4kkIi7QctVhin3e

  # 导出到指定路径
  feishu-cli wiki export Ad8Iw0oz3iSp4kkIi7QctVhin3e --output doc.md

  # 通过 URL 导出
  feishu-cli wiki export https://xxx.feishu.cn/wiki/Ad8Iw0oz3iSp4kkIi7QctVhin3e

  # 导出并下载图片
  feishu-cli wiki export Ad8Iw0oz3iSp4kkIi7QctVhin3e --download-images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		// 解析 node_token
		nodeToken, err := extractWikiToken(args[0])
		if err != nil {
			return err
		}

		// 1. 获取节点信息
		fmt.Printf("正在获取节点信息: %s\n", nodeToken)
		node, err := client.GetWikiNode(nodeToken)
		if err != nil {
			return err
		}

		fmt.Printf("文档标题: %s\n", node.Title)
		fmt.Printf("文档类型: %s\n", node.ObjType)
		fmt.Printf("文档 Token: %s\n", node.ObjToken)

		// 2. 检查文档类型
		if node.ObjType != "docx" {
			return fmt.Errorf("暂不支持导出 %s 类型的文档，目前仅支持 docx", node.ObjType)
		}

		// 3. 获取文档块
		fmt.Println("正在获取文档内容...")
		blocks, err := client.GetAllBlocks(node.ObjToken)
		if err != nil {
			return fmt.Errorf("获取块失败: %w", err)
		}

		// 4. 转换为 Markdown
		downloadImages, _ := cmd.Flags().GetBool("download-images")
		assetsDir, _ := cmd.Flags().GetString("assets-dir")

		options := converter.ConvertOptions{
			DocumentID:     node.ObjToken,
			DownloadImages: downloadImages,
			AssetsDir:      assetsDir,
		}

		conv := converter.NewBlockToMarkdown(blocks, options)
		markdown, err := conv.Convert()
		if err != nil {
			return fmt.Errorf("转换为 Markdown 失败: %w", err)
		}

		// 5. 保存文件
		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" {
			// 使用标题作为文件名
			safeTitle := node.Title
			if safeTitle == "" {
				safeTitle = nodeToken
			}
			outputPath = fmt.Sprintf("/tmp/%s.md", safeTitle)
		}

		// 路径安全检查
		if err := validateOutputPath(outputPath, ""); err != nil {
			return fmt.Errorf("输出路径不安全: %w", err)
		}

		// 确保目录存在（使用 0700 权限保护）
		dir := filepath.Dir(outputPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("创建目录失败: %w", err)
			}
		}

		// 使用 0600 权限保护导出文件
		if err := os.WriteFile(outputPath, []byte(markdown), 0600); err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}

		fmt.Printf("已导出到 %s\n", outputPath)
		return nil
	},
}

func init() {
	wikiCmd.AddCommand(exportWikiCmd)
	exportWikiCmd.Flags().StringP("output", "o", "", "输出文件路径")
	exportWikiCmd.Flags().Bool("download-images", false, "下载图片到本地目录")
	exportWikiCmd.Flags().String("assets-dir", "./assets", "下载资源的保存目录")
}
