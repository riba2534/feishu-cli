package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var importFileCmd = &cobra.Command{
	Use:   "import-file <local_path>",
	Short: "导入文件为云文档",
	Long: `将本地文件导入为飞书云文档。

流程：上传文件 → 创建导入任务 → 轮询任务状态 → 返回文档信息。

参数:
  local_path    本地文件路径

选项:
  --type        目标文档格式（docx/sheet/bitable，必填）
  --name        文档名称（默认使用文件名）
  --folder      目标文件夹 Token

支持的导入格式:
  docx      导入为新版文档（支持 .docx/.doc/.md/.txt 等）
  sheet     导入为电子表格（支持 .xlsx/.xls/.csv 等）

示例:
  # 导入 Word 文档
  feishu-cli doc import-file report.docx --type docx

  # 导入 Excel 到指定文件夹
  feishu-cli doc import-file data.xlsx --type sheet --folder fldcnXXX

  # 导入并重命名
  feishu-cli doc import-file report.docx --type docx --name "月度报告"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		localPath := args[0]
		targetType, _ := cmd.Flags().GetString("type")
		docName, _ := cmd.Flags().GetString("name")
		folderToken, _ := cmd.Flags().GetString("folder")
		output, _ := cmd.Flags().GetString("output")

		if docName == "" {
			docName = strings.TrimSuffix(filepath.Base(localPath), filepath.Ext(localPath))
		}

		// 获取文件扩展名（去掉前导点号）
		fileExt := strings.TrimPrefix(filepath.Ext(localPath), ".")

		// 上传文件
		fmt.Printf("正在上传文件...\n")
		fileToken, err := client.UploadFile(localPath, folderToken, "")
		if err != nil {
			return err
		}
		fmt.Printf("  文件 Token: %s\n", fileToken)

		// 创建导入任务
		fmt.Printf("正在创建导入任务...\n")
		ticket, err := client.CreateImportTask(fileToken, fileExt, docName, targetType, folderToken)
		if err != nil {
			return err
		}
		fmt.Printf("  任务 ID: %s\n", ticket)

		// 轮询等待任务完成
		fmt.Printf("正在等待导入完成...\n")
		docToken, url, err := client.WaitImportTask(ticket, 60)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]string{
				"token": docToken,
				"url":   url,
			})
		}

		fmt.Printf("导入成功！\n")
		fmt.Printf("  文档名称: %s\n", docName)
		fmt.Printf("  文档 Token: %s\n", docToken)
		if url != "" {
			fmt.Printf("  链接: %s\n", url)
		}

		// Auto-grant owner permission
		if err := grantOwnerPermission(docToken, targetType); err != nil {
			fmt.Fprintf(os.Stderr, "警告: %v\n", err)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(importFileCmd)
	importFileCmd.Flags().String("type", "", "目标文档格式（docx/sheet/bitable，必填）")
	importFileCmd.Flags().String("name", "", "文档名称（默认使用文件名）")
	importFileCmd.Flags().String("folder", "", "目标文件夹 Token")
	importFileCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(importFileCmd, "type")
}
