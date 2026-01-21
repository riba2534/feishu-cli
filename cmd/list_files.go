package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listFilesCmd = &cobra.Command{
	Use:   "list [folder_token]",
	Short: "列出文件夹中的文件",
	Long: `列出云空间文件夹中的文件和子文件夹。

参数:
  folder_token    文件夹 Token（不指定则列出根目录）

示例:
  # 列出根目录
  feishu-cli file list

  # 列出指定文件夹
  feishu-cli file list fldcnXXXXXXXXX

  # JSON 格式输出
  feishu-cli file list --output json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		var folderToken string
		if len(args) > 0 {
			folderToken = args[0]
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		files, _, _, err := client.ListFiles(folderToken, pageSize, "")
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(files, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(files) == 0 {
				fmt.Println("文件夹为空")
				return nil
			}

			fmt.Printf("共找到 %d 个文件/文件夹:\n\n", len(files))
			for i, f := range files {
				typeIcon := getFileTypeIcon(f.Type)
				fmt.Printf("[%d] %s %s\n", i+1, typeIcon, f.Name)
				fmt.Printf("    Token:    %s\n", f.Token)
				fmt.Printf("    类型:     %s\n", f.Type)
				if f.ModifiedTime != "" {
					fmt.Printf("    修改时间: %s\n", f.ModifiedTime)
				}
				if f.URL != "" {
					fmt.Printf("    链接:     %s\n", f.URL)
				}
				fmt.Println()
			}
		}

		return nil
	},
}

func getFileTypeIcon(fileType string) string {
	switch fileType {
	case "folder":
		return "📁"
	case "docx", "doc":
		return "📄"
	case "sheet":
		return "📊"
	case "bitable":
		return "📋"
	case "mindnote":
		return "🧠"
	case "slides":
		return "📽️"
	case "file":
		return "📎"
	default:
		return "📄"
	}
}

func init() {
	fileCmd.AddCommand(listFilesCmd)
	listFilesCmd.Flags().Int("page-size", 50, "每页数量")
	listFilesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
