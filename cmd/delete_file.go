package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteFileCmd = &cobra.Command{
	Use:   "delete <file_token>",
	Short: "删除文件或文件夹",
	Long: `删除云空间中的文件或文件夹。

警告: 删除操作不可恢复！

参数:
  file_token    文件或文件夹的 Token
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
  # 删除文档
  feishu-cli file delete doccnXXX --type docx

  # 删除文件夹
  feishu-cli file delete fldcnXXX --type folder`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		fileType, _ := cmd.Flags().GetString("type")
		force, _ := cmd.Flags().GetBool("force")

		// 危险操作确认
		if !force {
			if !confirmAction(fmt.Sprintf("确定要删除文件 %s (%s) 吗？此操作不可恢复", fileToken, fileType)) {
				fmt.Println("操作已取消")
				return nil
			}
		}

		taskID, err := client.DeleteFile(fileToken, fileType)
		if err != nil {
			return err
		}

		fmt.Printf("删除操作已提交！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  文件类型:   %s\n", fileType)
		if taskID != "" {
			fmt.Printf("  任务 ID:    %s\n", taskID)
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(deleteFileCmd)
	deleteFileCmd.Flags().String("type", "", "文件类型（必填）")
	deleteFileCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
	_ = deleteFileCmd.MarkFlagRequired("type")
}
