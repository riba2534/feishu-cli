package cmd

import (
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var authPermissionCmd = &cobra.Command{
	Use:   "auth <doc_token>",
	Short: "判断当前用户对文档的权限",
	Long: `判断当前用户是否拥有文档的指定权限。

参数:
  doc_token     文档 Token
  --doc-type    文档类型（默认: docx）
  --action      需要判断的权限（必填）

可用的 action 值:
  view      查看
  edit      编辑
  share     分享
  comment   评论
  export    导出

示例:
  # 判断是否有查看权限
  feishu-cli perm auth DOC_TOKEN --action view

  # 判断是否有编辑权限
  feishu-cli perm auth DOC_TOKEN --action edit --doc-type sheet`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		action, _ := cmd.Flags().GetString("action")

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		result, err := client.AuthPermission(docToken, docType, action, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		return printJSON(result)
	},
}

func init() {
	permCmd.AddCommand(authPermissionCmd)
	authPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	authPermissionCmd.Flags().String("action", "", "需要判断的权限（view/edit/share/comment/export）")
	mustMarkFlagRequired(authPermissionCmd, "action")
}
