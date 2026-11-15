package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var batchAddPermissionCmd = &cobra.Command{
	Use:   "batch-add <doc_token>",
	Short: "批量添加协作者权限",
	Long: `批量为文档添加多个协作者权限。

参数:
  doc_token         文档 Token
  --doc-type        文档类型（默认: docx）
  --members-file    成员列表 JSON 文件路径（必填）
  --notification    发送通知给成员
  --dry-run         只打印将要发出的请求（不读取凭据、不联网）

<doc_token> 可传裸 token 或文档链接（自动推断 token 与类型）。

成员列表 JSON 格式:
  [
    {"member_type": "email", "member_id": "user1@example.com", "perm": "edit"},
    {"member_type": "email", "member_id": "user2@example.com", "perm": "view"},
    {"member_type": "openid", "member_id": "ou_xxxxx", "perm": "full_access"}
  ]
  知识库（--doc-type wiki）成员可额外带 "perm_type": "container"|"single_page"。
  member_type 为 wikispaceid 时必须带 "type": "wiki_space_member"|"wiki_space_viewer"|"wiki_space_editor"（同 perm add --member-kind）。

权限级别:
  view          查看权限
  edit          编辑权限
  full_access   完全访问权限

示例:
  # 从文件批量添加协作者
  feishu-cli perm batch-add DOC_TOKEN --members-file members.json

  # 批量添加并发送通知
  feishu-cli perm batch-add DOC_TOKEN --members-file members.json --notification`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docToken, docType, err := resolvePermTarget(cmd, args[0])
		if err != nil {
			return err
		}
		membersFile, _ := cmd.Flags().GetString("members-file")
		notification, _ := cmd.Flags().GetBool("notification")

		data, err := readLocalInputFile(membersFile)
		if err != nil {
			return fmt.Errorf("读取成员列表文件失败: %w", err)
		}

		var members []*client.PermissionMember
		if err := json.Unmarshal(data, &members); err != nil {
			return clierr.Usagef("解析成员列表 JSON 失败: %w", err)
		}

		if len(members) == 0 {
			return clierr.Usagef("成员列表不能为空")
		}

		for i, m := range members {
			if m == nil {
				return clierr.Usagef("第 %d 个成员不能为 null", i+1)
			}
			m.MemberType = normalizePermMemberType(m.MemberType)
			if m.MemberType == "" {
				return clierr.Usagef("第 %d 个成员的 member_type 不能为空", i+1)
			}
			if m.MemberID == "" {
				return clierr.Usagef("第 %d 个成员的 member_id 不能为空", i+1)
			}
			if m.Perm == "" {
				return clierr.Usagef("第 %d 个成员的 perm 不能为空", i+1)
			}
			// 知识空间成员组（wikispaceid）的 type 即 --member-kind，规则与 perm add 一致
			kind, err := resolvePermMemberKind(m.MemberType, m.Type, m.PermType)
			if err != nil {
				return clierr.Usagef("第 %d 个成员: %w", i+1, err)
			}
			m.Type = kind
			permType, err := validatePermType(m.PermType, docType)
			if err != nil {
				return clierr.Usagef("第 %d 个成员: %w", i+1, err)
			}
			m.PermType = permType
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			bodyMembers := make([]map[string]any, 0, len(members))
			for _, m := range members {
				bodyMembers = append(bodyMembers, permMemberBody(*m))
			}
			return printPermDryRun(cmd, "Batch add document collaborators", "POST",
				permAPIPath("/open-apis/drive/v1/permissions/%s/members/batch_create", docToken),
				map[string]any{"type": docType, "need_notification": notification}, map[string]any{"members": bodyMembers})
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		if err := client.BatchAddPermission(docToken, docType, members, notification, userToken); err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("批量添加权限成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  添加成员数: %d\n", len(members))
		return nil
	},
}

func init() {
	permCmd.AddCommand(batchAddPermissionCmd)
	batchAddPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	batchAddPermissionCmd.Flags().String("members-file", "", "成员列表 JSON 文件路径")
	batchAddPermissionCmd.Flags().Bool("notification", false, "发送通知给成员")
	mustMarkFlagRequired(batchAddPermissionCmd, "members-file")
}
