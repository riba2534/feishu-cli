package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createShortcutCmd = &cobra.Command{
	Use:   "shortcut <file_token>",
	Short: "创建文件快捷方式",
	Long: `在指定文件夹中创建文件的快捷方式。

参数:
  file_token    目标文件的 Token
  --target      目标文件夹 Token（必填）
  --type        文件类型（必填）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件
  slides    幻灯片

示例:
  # 创建文档快捷方式
  feishu-cli file shortcut doccnXXX --target fldcnYYY --type docx

  # JSON 格式输出
  feishu-cli file shortcut doccnXXX --target fldcnYYY --type docx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		targetFolder, _ := cmd.Flags().GetString("target")
		fileType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")

		info, err := client.CreateShortcut(targetFolder, fileToken, fileType)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("快捷方式创建成功！\n")
			fmt.Printf("  快捷方式 Token: %s\n", info.Token)
			fmt.Printf("  目标文件 Token: %s\n", info.TargetToken)
			fmt.Printf("  目标文件类型:   %s\n", info.TargetType)
			if info.ParentToken != "" {
				fmt.Printf("  所在文件夹:     %s\n", info.ParentToken)
			}
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(createShortcutCmd)
	createShortcutCmd.Flags().String("target", "", "目标文件夹 Token（必填）")
	createShortcutCmd.Flags().String("type", "", "文件类型（必填）")
	createShortcutCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	_ = createShortcutCmd.MarkFlagRequired("target")
	_ = createShortcutCmd.MarkFlagRequired("type")
}
