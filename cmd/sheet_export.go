package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var sheetExportCmd = &cobra.Command{
	Use:   "export <spreadsheet_token_or_url>",
	Short: "导出电子表格为 XLSX、CSV 或 Markdown 文件",
	Long: `导出电子表格为文件。支持 XLSX、CSV 和 Markdown 三种格式。

CSV 格式导出时必须指定 --sheet-id 参数（只能导出单个工作表）。
Markdown 格式不指定 --sheet-id 时会导出所有可见工作表。
<spreadsheet_token_or_url> 支持直接传 spreadsheet token，或 /sheets/、/spreadsheets/、/wiki/ URL
（wiki 自动换出底层表格；CSV/Markdown 未指定 --sheet-id 时使用 URL 中的 ?sheet= 参数）。

示例:
  # 导出为 XLSX
  feishu-cli sheet export SPREADSHEET_TOKEN -o output.xlsx

  # 导出为 CSV（需指定工作表）
  feishu-cli sheet export SPREADSHEET_TOKEN --format csv --sheet-id SHEET_ID -o output.csv

  # 导出为 Markdown（默认所有可见工作表）
  feishu-cli sheet export SPREADSHEET_TOKEN --format markdown -o output.md
  feishu-cli sheet export https://xxx.feishu.cn/sheets/SPREADSHEET_TOKEN --format markdown -o output.md

  # 导出为 XLSX（自定义重试次数）
  feishu-cli sheet export SPREADSHEET_TOKEN -o report.xlsx --max-retries 60`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		format = normalizeSheetExportFormat(format)
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		outputPath, _ := cmd.Flags().GetString("output")
		maxRetries, _ := cmd.Flags().GetInt("max-retries")

		if !isSupportedSheetExportFormat(format) {
			return fmt.Errorf("不支持的导出格式: %s（支持 xlsx/csv/markdown）", format)
		}
		// 显式输出路径在任何网络请求（含 token 刷新、wiki 解析）之前校验，敏感目录直接拒绝
		if outputPath != "" {
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
		}

		// 解析身份与表格参数（支持 /sheets/、/spreadsheets/、/wiki/ URL）
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT
		if sheetID == "" && target.URLSheetID != "" && format != "xlsx" {
			sheetID = target.URLSheetID
		}

		// CSV 格式必须指定 sheet-id
		if format == "csv" && sheetID == "" {
			return clierr.Usagef("CSV 格式导出必须指定 --sheet-id 参数（使用 feishu-cli sheet list-sheets <token> 查看工作表 ID）")
		}

		// 默认输出文件名（落在当前目录，同样在创建导出任务之前校验）
		if outputPath == "" {
			outputPath = spreadsheetToken + "." + sheetExportFileExt(format)
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
		}

		if format == "markdown" {
			return exportSheetAsMarkdown(spreadsheetToken, sheetID, outputPath, userAccessToken, converter.FetchSheetDataForMarkdown, safefile.AtomicWriteFile)
		}

		// 创建导出任务
		fmt.Fprintf(os.Stderr, "正在创建导出任务...\n")
		ticket, err := client.CreateExportTaskWithSubId(spreadsheetToken, "sheet", format, sheetID, userAccessToken)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  任务 ID: %s\n", ticket)

		// 轮询等待任务完成
		fmt.Fprintf(os.Stderr, "正在等待导出完成...\n")
		fileToken, err := client.WaitExportTask(ticket, spreadsheetToken, userAccessToken, maxRetries)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  导出文件 Token: %s\n", fileToken)

		// 下载导出文件
		fmt.Fprintf(os.Stderr, "正在下载文件...\n")
		if err := client.DownloadExportFile(fileToken, outputPath, userAccessToken); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "导出成功！\n")
		fmt.Fprintf(os.Stderr, "  保存路径: %s\n", outputPath)

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetExportCmd)
	sheetExportCmd.Flags().StringP("format", "f", "xlsx", "导出格式（xlsx/csv/markdown）")
	sheetExportCmd.Flags().String("sheet-id", "", "工作表 ID（CSV 必填；Markdown 可选，留空导出所有可见工作表）")
	sheetExportCmd.Flags().StringP("output", "o", "", "输出文件路径")
	sheetExportCmd.Flags().Int("max-retries", 30, "最大轮询重试次数")
	sheetExportCmd.Flags().String("user-access-token", "", "User Access Token")
}

func normalizeSheetExportFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "md" {
		return "markdown"
	}
	return format
}

func isSupportedSheetExportFormat(format string) bool {
	return format == "xlsx" || format == "csv" || format == "markdown"
}

func sheetExportFileExt(format string) string {
	if format == "markdown" {
		return "md"
	}
	return format
}

func exportSheetAsMarkdown(
	spreadsheetToken, sheetID, outputPath, userAccessToken string,
	fetch converter.SheetDataProvider,
	writeFile func(string, []byte, os.FileMode) error,
) error {
	fmt.Fprintf(os.Stderr, "正在读取电子表格数据...\n")
	sheets, err := fetch(spreadsheetToken, sheetID, userAccessToken)
	if err != nil {
		return err
	}

	markdown := converter.SheetToMarkdown(sheets)
	if err := writeFile(outputPath, []byte(markdown), 0600); err != nil {
		return fmt.Errorf("写入输出文件失败: %w", err)
	}

	fmt.Fprintf(os.Stderr, "导出成功！\n")
	fmt.Fprintf(os.Stderr, "  保存路径: %s\n", outputPath)
	return nil
}
