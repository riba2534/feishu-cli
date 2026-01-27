package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var addPermissionCmd = &cobra.Command{
	Use:   "add <doc_token>",
	Short: "添加文档权限",
	Long: `为文档添加协作者权限。

参数:
  --doc-type      文档类型（默认: docx）
  --member-type   成员类型（必填）
  --member-id     成员标识（必填）
  --perm          权限级别（必填）
  --notification  发送通知给成员

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
  # 通过邮箱添加编辑权限
  feishu-cli perm add DOC_TOKEN \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm edit

  # 添加完全访问权限并发送通知
  feishu-cli perm add DOC_TOKEN \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm full_access \
    --notification`,
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
		notification, _ := cmd.Flags().GetBool("notification")

		member := client.PermissionMember{
			MemberType: memberType,
			MemberID:   memberID,
			Perm:       perm,
		}

		if err := client.AddPermission(docToken, docType, member, notification); err != nil {
			return err
		}

		fmt.Printf("权限添加成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  成员: %s（%s）\n", memberID, memberType)
		fmt.Printf("  权限: %s\n", perm)
		return nil
	},
}

func init() {
	permCmd.AddCommand(addPermissionCmd)
	addPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	addPermissionCmd.Flags().String("member-type", "", "成员类型（email/openid/userid 等）")
	addPermissionCmd.Flags().String("member-id", "", "成员标识")
	addPermissionCmd.Flags().String("perm", "", "权限级别（view/edit/full_access）")
	addPermissionCmd.Flags().Bool("notification", false, "发送通知给成员")
	mustMarkFlagRequired(addPermissionCmd, "member-type", "member-id", "perm")
}
