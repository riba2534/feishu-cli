package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var copyFileCmd = &cobra.Command{
	Use:   "copy <file_token>",
	Short: "复制文件",
	Long: `复制文件到指定位置。

参数:
  file_token    文件的 Token
  --target      目标文件夹 Token（必填）
  --type        文件类型（必填）
  --name        新文件名称（可选）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件

示例:
  # 复制文档
  feishu-cli file copy doccnXXX --target fldcnYYY --type docx

  # 复制并重命名
  feishu-cli file copy doccnXXX --target fldcnYYY --type docx --name "副本"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		targetFolder, _ := cmd.Flags().GetString("target")
		fileType, _ := cmd.Flags().GetString("type")
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")

		newToken, url, err := client.CopyFile(fileToken, targetFolder, name, fileType)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"token": newToken,
				"url":   url,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("文件复制成功！\n")
			fmt.Printf("  原文件 Token: %s\n", fileToken)
			fmt.Printf("  新文件 Token: %s\n", newToken)
			if url != "" {
				fmt.Printf("  链接:         %s\n", url)
			}
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(copyFileCmd)
	copyFileCmd.Flags().String("target", "", "目标文件夹 Token（必填）")
	copyFileCmd.Flags().String("type", "", "文件类型（必填）")
	copyFileCmd.Flags().String("name", "", "新文件名称")
	copyFileCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(copyFileCmd, "target", "type")
}
