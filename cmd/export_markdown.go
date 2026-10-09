package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var exportMarkdownCmd = &cobra.Command{
	Use:   "export <document_id|url>",
	Short: "导出文档为 Markdown",
	Long: `将飞书文档导出为 Markdown 格式。

支持通过文档 ID 或 URL 导出（wiki URL 会自动解析为底层 docx 文档）：
  feishu-cli doc export ABC123def456
  feishu-cli doc export https://xxx.feishu.cn/docx/ABC123def456
  feishu-cli doc export https://xxx.larkoffice.com/docx/ABC123def456
  feishu-cli doc export https://xxx.feishu.cn/wiki/wikcnXXXXXX

使用 --download-images 可同时下载文档中的图片和画板（画板自动导出为 PNG），
通过 --assets-dir 指定资源保存目录（默认 ./assets，相对当前工作目录）。用 -o 写文件时，
Markdown 中的资源引用路径相对输出文件所在目录，导出后可直接对该文件 doc import。
不带 --download-images 时图片/附件/画板输出为 <image token>/<file token>/<whiteboard token> 标签，
doc import 会复用素材（下载原素材后重新上传）、复制源画板节点；当前身份无源文档权限时降级为占位文本并计入 failures。
内嵌飞书电子表格默认会自动展开为 Markdown 表格，可用 --expand-sheets=false 保留为 <sheet/> 引用。
跨文档引用同步块会自动读取源文档并展开；权限或 API 异常时输出带源标识的 WARNING 占位和 stderr 诊断，不会静默丢失内容。

docs_ai 引擎（--engine docs_ai，服务端导出，与 doc content-update / docs_ai 写入同一方言，写回无需方言转换）：
  保留 callout emoji、分栏宽度比例、@人/@文档引用、mermaid/plantuml 画板源码、原图 token；
  callout 颜色、文字颜色、下划线等样式只在 --doc-format xml --detail full 中保留（Markdown 序列化会丢失）。
  --detail with-ids|full 仅用于 xml，输出 block id 与样式属性。画板写回到其它文档仍可能克隆失败（degrade 2105）。
  docs_ai 引擎不支持 --download-images / --highlight / --expand-mentions / --expand-sheets（本地引擎专属）。

示例:
  feishu-cli doc export ABC123def456
  feishu-cli doc export ABC123def456 --output doc.md
  feishu-cli doc export ABC123def456 --engine docs_ai -o doc.md        # 服务端 Markdown（docs_ai 方言）
  feishu-cli doc export ABC123def456 --engine docs_ai --doc-format xml --detail full -o doc.xml
  feishu-cli doc export ABC123def456 --download-images
  feishu-cli doc export ABC123def456 --download-images --assets-dir ./images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		downloadImages, _ := cmd.Flags().GetBool("download-images")
		assetsDir, _ := cmd.Flags().GetString("assets-dir")

		engine, _ := cmd.Flags().GetString("engine")
		engine = strings.ToLower(strings.TrimSpace(engine))
		switch engine {
		case "", "local":
			for _, name := range []string{"doc-format", "detail"} {
				if cmd.Flags().Changed(name) {
					return clierr.Usagef("--%s 只用于 --engine docs_ai", name)
				}
			}
		case "docs_ai", "docs-ai":
			for _, name := range []string{"download-images", "assets-dir", "highlight", "expand-mentions", "expand-sheets"} {
				if cmd.Flags().Changed(name) {
					return clierr.Usagef("--%s 是本地引擎专属参数，不能与 --engine docs_ai 同时使用", name)
				}
			}
		default:
			return clierr.Usagef("不支持的 --engine %q，可选 local（默认）或 docs_ai", engine)
		}

		// 本地输出路径在任何网络请求（含 token 刷新、wiki 解析）之前校验，敏感目录直接拒绝
		if err := validateDocExportPaths(output, assetsDir, downloadImages); err != nil {
			return err
		}

		// 获取可选的 User Access Token（用于访问无 App 权限的文档）
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		// 支持 docx token、/docx/ URL 与 /wiki/ URL（wiki 经 node_by_token 换出底层 docx）
		documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
		if err != nil {
			return err
		}

		if engine == "docs_ai" || engine == "docs-ai" {
			return exportDocsAI(cmd, documentID, userAccessToken, output)
		}

		// Get all blocks
		blocks, err := client.GetAllBlocksWithToken(documentID, userAccessToken)
		if err != nil {
			return fmt.Errorf("获取块失败: %w", err)
		}

		frontMatter, _ := cmd.Flags().GetBool("front-matter")
		highlight, _ := cmd.Flags().GetBool("highlight")

		expandMentions, _ := cmd.Flags().GetBool("expand-mentions")
		expandSheets, _ := cmd.Flags().GetBool("expand-sheets")

		// Convert to Markdown
		cfg := config.Get()
		// 写文件时，下载资源的引用路径改写为相对输出 Markdown 所在目录，保证「导出 → 原地 doc import」可用
		// （导入按 Markdown 文件所在目录解析相对路径）；输出到 stdout 时保持相对当前工作目录。
		assetsLinkBase := ""
		if output != "" {
			assetsLinkBase = filepath.Dir(output)
		}
		options := converter.ConvertOptions{
			DownloadImages:  downloadImages,
			AssetsDir:       assetsDir,
			AssetsLinkBase:  assetsLinkBase,
			DocumentID:      documentID,
			UserAccessToken: userAccessToken,
			Debug:           cfg.Debug,
			FrontMatter:     frontMatter,
			Highlight:       highlight,
			ExpandMentions:  expandMentions,
			ExpandSheets:    expandSheets,
		}

		var conv *converter.BlockToMarkdown
		if expandMentions {
			resolver := &FeishuUserResolver{}
			conv = converter.NewBlockToMarkdownWithResolver(blocks, options, resolver)
		} else {
			conv = converter.NewBlockToMarkdown(blocks, options)
		}
		markdown, err := conv.Convert()
		if err != nil {
			return fmt.Errorf("转换为 Markdown 失败: %w", err)
		}

		// 添加 Front Matter
		if frontMatter {
			docTitle := ""
			doc, docErr := client.GetDocumentWithToken(documentID, userAccessToken)
			if docErr == nil && doc != nil && doc.Title != nil {
				docTitle = *doc.Title
			}
			fm := fmt.Sprintf("---\ntitle: %q\ndocument_id: %s\n---\n\n", docTitle, documentID)
			markdown = fm + markdown
		}

		// Output
		return writeExportOutput(output, markdown)
	},
}

// validateDocExportPaths 校验 doc export 的本地输出位置：-o 文件与（--download-images 时）--assets-dir。
func validateDocExportPaths(output, assetsDir string, downloadImages bool) error {
	if output != "" {
		if err := validateOutputPath(output, ""); err != nil {
			return err
		}
	}
	if downloadImages {
		if err := validateOutputPath(assetsDir, ""); err != nil {
			return fmt.Errorf("--assets-dir 无效: %w", err)
		}
	}
	return nil
}

// writeExportOutput 原子写入输出文件（或打印到 stdout）。
func writeExportOutput(output, content string) error {
	if output == "" {
		fmt.Print(content)
		return nil
	}
	if err := safefile.AtomicWriteFile(output, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入输出文件失败: %w", err)
	}
	fmt.Printf("已导出到 %s\n", output)
	return nil
}

// exportDocsAI 用 docs_ai fetch 导出全文（服务端方言，可经 content-update 无损写回）。
func exportDocsAI(cmd *cobra.Command, documentID, userAccessToken, output string) error {
	format, _ := cmd.Flags().GetString("doc-format")
	detail, _ := cmd.Flags().GetString("detail")
	format = strings.ToLower(strings.TrimSpace(format))
	detail = strings.ToLower(strings.TrimSpace(detail))
	if format == "" {
		format = "markdown"
	}
	if detail == "" {
		detail = "simple"
	}
	if format != "markdown" && format != "xml" {
		return clierr.Usagef("不支持的 --doc-format %q，可选 markdown / xml", format)
	}
	switch detail {
	case "simple", "with-ids", "full":
	default:
		return clierr.Usagef("不支持的 --detail %q，可选 simple / with-ids / full", detail)
	}
	if format == "markdown" && detail != "simple" {
		return clierr.Usagef("--detail %s 只支持 --doc-format xml", detail)
	}
	opts := &docsAIReadOptions{format: format, detail: detail, scope: "full", maxDepth: -1}
	data, err := client.FetchDocsAI(documentID, buildDocsAIFetchBody(opts), userAccessToken)
	if err != nil {
		return err
	}
	content, _ := client.DocsAIDocumentContent(data)
	if frontMatter, _ := cmd.Flags().GetBool("front-matter"); frontMatter {
		docTitle := ""
		if doc, docErr := client.GetDocumentWithToken(documentID, userAccessToken); docErr == nil && doc != nil && doc.Title != nil {
			docTitle = *doc.Title
		}
		content = fmt.Sprintf("---\ntitle: %q\ndocument_id: %s\n---\n\n", docTitle, documentID) + content
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	for _, w := range client.DocsAIWarnings(data) {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 服务端警告: %s\n", w)
	}
	return writeExportOutput(output, content)
}

func init() {
	docCmd.AddCommand(exportMarkdownCmd)
	exportMarkdownCmd.Flags().StringP("output", "o", "", "输出文件路径")
	exportMarkdownCmd.Flags().Bool("download-images", false, "下载图片和画板到本地目录（画板自动导出为 PNG）")
	exportMarkdownCmd.Flags().String("assets-dir", "./assets", "图片和画板的保存目录")
	exportMarkdownCmd.Flags().Bool("front-matter", false, "添加 YAML front matter (标题和文档 ID)")
	exportMarkdownCmd.Flags().Bool("highlight", false, "保留文本颜色和背景色 (输出为 HTML span)")
	exportMarkdownCmd.Flags().Bool("expand-mentions", true, "展开 @用户为友好格式 (需要 contact:user.base:readonly 权限)")
	exportMarkdownCmd.Flags().Bool("expand-sheets", true, "自动展开内嵌飞书电子表格为 Markdown 表格")
	exportMarkdownCmd.Flags().String("user-access-token", "", "User Access Token（用于访问无 App 权限的文档，自动从 auth login 读取）")
	exportMarkdownCmd.Flags().String("engine", "local", "导出引擎: local（本地块树转换，默认）| docs_ai（服务端导出，docs_ai 方言）")
	exportMarkdownCmd.Flags().String("doc-format", "", "docs_ai: 输出格式 markdown（默认）| xml")
	exportMarkdownCmd.Flags().String("detail", "", "docs_ai: xml 详细程度 simple | with-ids | full")
}
