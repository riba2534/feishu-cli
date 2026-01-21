package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updatePermissionCmd = &cobra.Command{
	Use:   "update <doc_token>",
	Short: "更新协作者权限",
	Long: `更新文档协作者的权限级别。

参数:
  doc_token       文档 Token
  --doc-type      文档类型（默认: docx）
  --member-type   成员类型（必填）
  --member-id     成员标识（必填）
  --perm          新权限级别（必填）

权限级别:
  view          查看权限
  edit          编辑权限
  full_access   完全访问权限

成员类型:
  email             邮箱
  openid            Open ID
  userid            用户 ID
  unionid           Union ID
  openchat          群组 ID
  opendepartmentid  部门 ID
  groupid           群组 ID
  wikispaceid       知识空间 ID

示例:
  # 将用户权限更新为编辑权限
  feishu-cli perm update DOC_TOKEN \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm edit

  # 将用户权限更新为完全访问权限
  feishu-cli perm update DOC_TOKEN \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm full_access`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		perm, _ := cmd.Flags().GetString("perm")

		if err := client.UpdatePermission(docToken, docType, memberID, memberType, perm); err != nil {
			return err
		}

		fmt.Printf("权限更新成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  成员: %s（%s）\n", memberID, memberType)
		fmt.Printf("  新权限: %s\n", perm)
		return nil
	},
}

func init() {
	permCmd.AddCommand(updatePermissionCmd)
	updatePermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	updatePermissionCmd.Flags().String("member-type", "", "成员类型（email/openid/userid 等）")
	updatePermissionCmd.Flags().String("member-id", "", "成员标识")
	updatePermissionCmd.Flags().String("perm", "", "新权限级别（view/edit/full_access）")
	_ = updatePermissionCmd.MarkFlagRequired("member-type")
	_ = updatePermissionCmd.MarkFlagRequired("member-id")
	_ = updatePermissionCmd.MarkFlagRequired("perm")
}
