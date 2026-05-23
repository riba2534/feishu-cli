package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var fileStatsCmd = &cobra.Command{
	Use:   "stats <file_token>",
	Short: "获取文件统计信息",
	Long: `获取云空间文件的访问统计信息，包括访问人数、访问次数、点赞数等。

参数:
  file_token    文件的 Token

选项:
  --doc-type    文件类型（必填）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

示例:
  # 获取文档统计信息
  feishu-cli file stats doccnXXX --doc-type docx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		stats, err := client.GetFileStatistics(fileToken, docType, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(stats)
		}

		fmt.Printf("文件统计信息:\n")
		fmt.Printf("  文件 Token: %s\n", stats.FileToken)
		fmt.Printf("  文件类型:   %s\n", stats.FileType)
		fmt.Printf("\n历史统计:\n")
		fmt.Printf("  访问人数（UV）: %d\n", stats.UV)
		fmt.Printf("  访问次数（PV）: %d\n", stats.PV)
		fmt.Printf("  点赞数:         %d\n", stats.LikeCount)
		fmt.Printf("\n今日统计:\n")
		fmt.Printf("  新增访问人数: %d\n", stats.UVToday)
		fmt.Printf("  新增访问次数: %d\n", stats.PVToday)
		fmt.Printf("  新增点赞数:   %d\n", stats.LikeCountToday)

		return nil
	},
}

func init() {
	fileCmd.AddCommand(fileStatsCmd)
	fileStatsCmd.Flags().String("doc-type", "", "文件类型（必填）")
	fileStatsCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	fileStatsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(fileStatsCmd, "doc-type")
}
