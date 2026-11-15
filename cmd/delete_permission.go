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
  --member-kind   知识空间成员组（仅且必须在 --member-type wikispaceid 时传）：
                  wiki_space_member / wiki_space_viewer / wiki_space_editor
  --dry-run       只打印将要发出的请求（不读取凭据、不联网）

<doc_token> 可传裸 token 或文档链接（自动推断 token 与类型）。

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
		docToken, docType, err := resolvePermTarget(cmd, args[0])
		if err != nil {
			return err
		}
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		memberKindFlag, _ := cmd.Flags().GetString("member-kind")

		memberType = normalizePermMemberType(memberType)
		memberKind, err := resolvePermMemberKind(memberType, memberKindFlag, "")
		if err != nil {
			return err
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			var body map[string]any
			if memberKind != "" {
				body = map[string]any{"type": memberKind}
			}
			return printPermDryRun(cmd, "Remove document collaborator", "DELETE",
				permAPIPath("/open-apis/drive/v1/permissions/%s/members/%s", docToken, memberID),
				map[string]any{"type": docType, "member_type": memberType}, body)
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		if err := client.DeletePermission(docToken, docType, memberType, memberID, memberKind, userToken); err != nil {
			return wrapPermError(err, userToken)
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
	deletePermissionCmd.Flags().String("member-kind", "", "知识空间成员组（仅 --member-type wikispaceid，必填）：wiki_space_member / wiki_space_viewer / wiki_space_editor")
	mustMarkFlagRequired(deletePermissionCmd, "member-type", "member-id")
}
