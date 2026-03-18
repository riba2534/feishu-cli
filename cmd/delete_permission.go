package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deletePermissionCmd = &cobra.Command{
	Use:   "delete <doc_token>",
	Short: "删除协作者权限",
	Long: `删除文档指定协作者的权限。

参数:
  doc_token       文档 Token
  --doc-type      文档类型（默认: docx）
  --member-type   成员类型（必填）
  --member-id     成员标识（必填）

成员类型:
  email             邮箱
  openid/open_id    Open ID
  userid/user_id    用户 ID
  unionid/union_id  Union ID
  openchat/chat_id  群组 ID
  opendepartmentid  部门 ID
  groupid           群组 ID
  wikispaceid       知识空间 ID

示例:
  # 通过邮箱删除协作者
  feishu-cli perm delete DOC_TOKEN \
    --member-type email \
    --member-id user@example.com

  # 删除电子表格的协作者
  feishu-cli perm delete DOC_TOKEN \
    --doc-type sheet \
    --member-type openid \
    --member-id ou_xxxxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")

		memberType = normalizePermMemberType(memberType)

		if err := client.DeletePermission(docToken, docType, memberType, memberID); err != nil {
			return err
		}

		fmt.Printf("权限删除成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  成员: %s（%s）\n", memberID, memberType)
		return nil
	},
}

func init() {
	permCmd.AddCommand(deletePermissionCmd)
	deletePermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	deletePermissionCmd.Flags().String("member-type", "", "成员类型（email/openid/open_id/userid/user_id 等）")
	deletePermissionCmd.Flags().String("member-id", "", "成员标识")
	mustMarkFlagRequired(deletePermissionCmd, "member-type", "member-id")
}
