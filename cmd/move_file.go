package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var moveFileCmd = &cobra.Command{
	Use:   "move <file_token>",
	Short: "移动文件或文件夹",
	Long: `将文件或文件夹移动到指定位置。

参数:
  file_token    文件或文件夹的 Token
  --target      目标文件夹 Token（必填）
  --type        文件类型（必填）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件
  folder    文件夹

示例:
  # 移动文档
  feishu-cli file move doccnXXX --target fldcnYYY --type docx

  # 移动文件夹
  feishu-cli file move fldcnXXX --target fldcnYYY --type folder`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		targetFolder, _ := cmd.Flags().GetString("target")
		fileType, _ := cmd.Flags().GetString("type")

		taskID, err := client.MoveFile(fileToken, targetFolder, fileType)
		if err != nil {
			return err
		}

		fmt.Printf("移动操作已提交！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  目标文件夹: %s\n", targetFolder)
		if taskID != "" {
			fmt.Printf("  任务 ID:    %s\n", taskID)
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(moveFileCmd)
	moveFileCmd.Flags().String("target", "", "目标文件夹 Token（必填）")
	moveFileCmd.Flags().String("type", "", "文件类型（必填）")
	_ = moveFileCmd.MarkFlagRequired("target")
	_ = moveFileCmd.MarkFlagRequired("type")
}
