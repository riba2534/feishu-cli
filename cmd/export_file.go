package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var exportFileCmd = &cobra.Command{
	Use:   "export-file <doc_token|url>",
	Short: "导出文档为文件",
	Long: `将飞书云文档导出为指定格式的文件（PDF、DOCX、XLSX 等）。

这是一个异步操作：创建导出任务 → 轮询任务状态 → 下载导出文件。

参数:
  doc_token     文档的 Token 或 URL（/docx/、/sheets/、/base/ 按路径推断 --doc-type；/wiki/ 自动解析为底层文档）

选项:
  --type        导出格式（pdf/docx/xlsx，必填）
  --doc-type    文档类型（默认 docx）
  -o, --output  输出文件路径

支持的导出格式:
  pdf       PDF 格式
  docx      Word 格式
  xlsx      Excel 格式

文档类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

示例:
  # 导出文档为 PDF
  feishu-cli doc export-file doccnXXX --type pdf -o output.pdf
  feishu-cli doc export-file https://xxx.feishu.cn/wiki/wikcnXXX --type docx -o output.docx

  # 导出电子表格为 Excel
  feishu-cli doc export-file shtcnXXX --type xlsx --doc-type sheet -o report.xlsx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileType, _ := cmd.Flags().GetString("type")
		docType, _ := cmd.Flags().GetString("doc-type")
		outputPath, _ := cmd.Flags().GetString("output")

		// 获取可选的 User Access Token
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		// 支持裸 token 与 URL：/docx/ /sheets/ /base/ 等按路径推断类型，/wiki/ 解包为底层文档。
		// 显式 --doc-type 与 URL 推断冲突时报错；裸 token 沿用 --doc-type（默认 docx）。
		explicitType := ""
		if cmd.Flags().Changed("doc-type") {
			explicitType = docType
		}
		res, err := resolveResourceArg(args[0], resourceArgOptions{
			ArgName:         "<doc_token|url>",
			ExplicitType:    explicitType,
			DefaultType:     docType,
			Allowed:         []string{client.ResourceTypeDoc, client.ResourceTypeDocx, client.ResourceTypeSheet, client.ResourceTypeBitable},
			ResolveWiki:     true,
			UserAccessToken: userAccessToken,
		})
		if err != nil {
			return err
		}
		noteWikiResolved(res)
		docToken, docType := res.Token, res.Type

		if outputPath == "" {
			outputPath = fmt.Sprintf("%s.%s", docToken, fileType)
		}

		// 创建导出任务
		fmt.Printf("正在创建导出任务...\n")
		ticket, err := client.CreateExportTask(docToken, docType, fileType, userAccessToken)
		if err != nil {
			return err
		}
		fmt.Printf("  任务 ID: %s\n", ticket)

		// 轮询等待任务完成
		fmt.Printf("正在等待导出完成...\n")
		fileToken, err := client.WaitExportTask(ticket, docToken, userAccessToken, 60)
		if err != nil {
			return err
		}
		fmt.Printf("  导出文件 Token: %s\n", fileToken)

		// 下载导出文件
		fmt.Printf("正在下载文件...\n")
		if err := client.DownloadExportFile(fileToken, outputPath, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("导出成功！\n")
		fmt.Printf("  保存路径: %s\n", outputPath)

		return nil
	},
}

func init() {
	docCmd.AddCommand(exportFileCmd)
	exportFileCmd.Flags().String("type", "", "导出格式（pdf/docx/xlsx，必填）")
	exportFileCmd.Flags().String("doc-type", "docx", "文档类型")
	exportFileCmd.Flags().StringP("output", "o", "", "输出文件路径")
	exportFileCmd.Flags().String("user-access-token", "", "User Access Token（用于导出无 App 权限的文档）")
	mustMarkFlagRequired(exportFileCmd, "type")
}
