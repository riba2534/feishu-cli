package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var fileMetaCmd = &cobra.Command{
	Use:   "meta <token> [token...]",
	Short: "批量获取文件元数据",
	Long: `批量获取云空间文件的元数据信息，包括标题、所有者、创建时间等。

参数:
  token         文件 Token（支持多个，用空格分隔）

选项:
  --doc-type    文件类型（必填）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  folder    文件夹
  file      普通文件

示例:
  # 获取单个文件元数据
  feishu-cli file meta doccnXXX --doc-type docx

  # 批量获取多个文件元数据
  feishu-cli file meta doccnXXX shtcnYYY --doc-type docx`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docType, _ := cmd.Flags().GetString("doc-type")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		metas, err := client.BatchGetMeta(args, docType, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(metas)
		}

		if len(metas) == 0 {
			fmt.Println("未找到文件元数据")
			return nil
		}

		fmt.Printf("共获取 %d 个文件的元数据:\n\n", len(metas))
		for i, m := range metas {
			fmt.Printf("[%d] %s\n", i+1, m.Title)
			fmt.Printf("    Token: %s\n", m.DocToken)
			fmt.Printf("    类型:  %s\n", m.DocType)
			if m.OwnerID != "" {
				fmt.Printf("    所有者: %s\n", m.OwnerID)
			}
			if m.CreateTime != "" {
				fmt.Printf("    创建时间: %s\n", m.CreateTime)
			}
			if m.LatestModifyTime != "" {
				fmt.Printf("    最后修改: %s\n", m.LatestModifyTime)
			}
			if m.URL != "" {
				fmt.Printf("    链接: %s\n", m.URL)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(fileMetaCmd)
	fileMetaCmd.Flags().String("doc-type", "", "文件类型（必填）")
	fileMetaCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	fileMetaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(fileMetaCmd, "doc-type")
}
