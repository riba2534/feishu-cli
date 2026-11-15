package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
  --perm-type     知识库权限范围（仅 --doc-type wiki）：container=当前页面及子页面，single_page=仅当前页面；
                  不传时不下发，由服务端按默认（container）处理
  --member-kind   知识空间成员组（仅且必须在 --member-type wikispaceid 时传）：
                  wiki_space_member / wiki_space_viewer / wiki_space_editor，写入请求体 type；此时不能传 --perm-type
  --dry-run       只打印将要发出的请求（不读取凭据、不联网）

<doc_token> 可传裸 token 或文档链接：链接自动推断 token 与类型（/wiki/ 对应 wiki），显式 --doc-type 须与链接一致。

权限级别:
  view          查看权限
  edit          编辑权限
  full_access   完全访问权限

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
    --notification

  # 知识库节点：仅授予当前页面（不含子页面）的查看权限
  feishu-cli perm add WIKI_NODE_TOKEN --doc-type wiki \
    --member-type openid --member-id ou_xxx --perm view --perm-type single_page

  # 把知识空间的可阅读成员组整体授权到文档
  feishu-cli perm add DOC_TOKEN --member-type wikispaceid --member-id 7xxxxxxxx \
    --member-kind wiki_space_viewer --perm view --dry-run

  # 以当前登录用户身份为个人文档添加协作者
  feishu-cli perm add DOC_TOKEN --as user \
    --member-type email --member-id user@example.com --perm view`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docToken, docType, err := resolvePermTarget(cmd, args[0])
		if err != nil {
			return err
		}
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		perm, _ := cmd.Flags().GetString("perm")
		notification, _ := cmd.Flags().GetBool("notification")

		permType, _ := cmd.Flags().GetString("perm-type")
		memberKindFlag, _ := cmd.Flags().GetString("member-kind")

		memberType = normalizePermMemberType(memberType)
		memberKind, err := resolvePermMemberKind(memberType, memberKindFlag, permType)
		if err != nil {
			return err
		}
		permType, err = validatePermType(permType, docType)
		if err != nil {
			return err
		}

		member := client.PermissionMember{
			MemberType: memberType,
			MemberID:   memberID,
			Perm:       perm,
			PermType:   permType,
			Type:       memberKind,
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printPermDryRun(cmd, "Add document collaborator", "POST",
				permAPIPath("/open-apis/drive/v1/permissions/%s/members", docToken),
				map[string]any{"type": docType, "need_notification": notification}, permMemberBody(member))
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		if err := client.AddPermission(docToken, docType, member, notification, userToken); err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("权限添加成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  成员: %s（%s）\n", memberID, memberType)
		fmt.Printf("  权限: %s\n", perm)
		return nil
	},
}

// permMemberBody 返回与 client.buildBaseMember 一致的请求体（perm_type/type 为空不下发），供 --dry-run 预览。
func permMemberBody(m client.PermissionMember) map[string]any {
	body := map[string]any{"member_type": m.MemberType, "member_id": m.MemberID, "perm": m.Perm}
	if m.PermType != "" {
		body["perm_type"] = m.PermType
	}
	if m.Type != "" {
		body["type"] = m.Type
	}
	return body
}

// validatePermType 校验 perm_type：仅知识库（wiki）文档有效，取值 container / single_page。
func validatePermType(permType, docType string) (string, error) {
	permType = strings.TrimSpace(permType)
	if permType == "" {
		return "", nil
	}
	if strings.TrimSpace(docType) != "wiki" {
		return "", clierr.Usagef("--perm-type / perm_type 仅在文档类型为 wiki 时有效，当前 --doc-type=%q", docType)
	}
	switch permType {
	case "container", "single_page":
		return permType, nil
	}
	return "", clierr.Usagef("perm_type 只能是 container 或 single_page，得到 %q", permType)
}

func init() {
	permCmd.AddCommand(addPermissionCmd)
	addPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	addPermissionCmd.Flags().String("member-type", "", "成员类型（email/openid/open_id/userid/user_id 等）")
	addPermissionCmd.Flags().String("member-id", "", "成员标识")
	addPermissionCmd.Flags().String("perm", "", "权限级别（view/edit/full_access）")
	addPermissionCmd.Flags().Bool("notification", false, "发送通知给成员")
	addPermissionCmd.Flags().String("perm-type", "", "知识库权限范围（仅 --doc-type wiki）：container / single_page")
	addPermissionCmd.Flags().String("member-kind", "", "知识空间成员组（仅 --member-type wikispaceid，必填）：wiki_space_member / wiki_space_viewer / wiki_space_editor")
	mustMarkFlagRequired(addPermissionCmd, "member-type", "member-id", "perm")
}
