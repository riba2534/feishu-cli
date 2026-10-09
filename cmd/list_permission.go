package cmd

import (
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var listPermissionCmd = &cobra.Command{
	Use:   "list <doc_token>",
	Short: "查看协作者列表",
	Long: `查看文档的所有协作者及其权限。

参数:
  doc_token       文档 Token
  --doc-type      文档类型（默认: docx）

文档类型:
  docx      新版文档（默认）
  doc       旧版文档
  sheet     电子表格
  bitable   多维表格
  wiki      知识库节点
  file      云空间文件
  folder    文件夹
  mindnote  思维笔记
  minutes   妙记
  slides    幻灯片

示例:
  # 查看文档的协作者列表
  feishu-cli perm list DOC_TOKEN

  # 查看电子表格的协作者列表
  feishu-cli perm list DOC_TOKEN --doc-type sheet`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		members, err := client.ListPermission(docToken, docType, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		return printJSON(members)
	},
}

func init() {
	permCmd.AddCommand(listPermissionCmd)
	listPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
}
