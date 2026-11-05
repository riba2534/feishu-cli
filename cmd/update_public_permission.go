package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updatePublicPermissionCmd = &cobra.Command{
	Use:   "public-update <doc_token>",
	Short: "更新文档公共权限设置",
	Long: `更新文档的公共权限设置。所有权限参数均为可选，仅更新指定的字段。

参数:
  doc_token            文档 Token
  --doc-type           文档类型（默认: docx）
  --external-access    是否允许内容被分享到组织外（true/false）
  --security-entity    谁可以复制内容、创建副本、打印、下载
  --comment-entity     谁可以评论
  --share-entity       谁可以添加和管理协作者
  --link-share-entity  链接分享设置
  --invite-external    是否允许非管理权限的人分享到组织外（true/false）
  --dry-run            只打印将要发出的请求（不读取凭据、不联网）

<doc_token> 可传裸 token 或文档链接（自动推断 token 与类型）。

常用值:
  security_entity: anyone_can_view, anyone_can_edit, only_full_access
  comment_entity:  anyone_can_view, anyone_can_edit
  share_entity:    anyone, same_tenant, only_full_access
  link_share_entity: tenant_readable, tenant_editable, anyone_readable, anyone_editable, closed

示例:
  # 开启外部访问
  feishu-cli perm public-update DOC_TOKEN --external-access=true

  # 设置链接分享为组织内可阅读
  feishu-cli perm public-update DOC_TOKEN --link-share-entity tenant_readable

  # 同时更新多个设置
  feishu-cli perm public-update DOC_TOKEN \
    --external-access=true \
    --link-share-entity tenant_readable \
    --comment-entity anyone_can_view`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docToken, docType, err := resolvePermTarget(cmd, args[0])
		if err != nil {
			return err
		}

		update := client.PublicPermissionUpdate{}
		hasUpdate := false

		if cmd.Flags().Changed("external-access") {
			v, _ := cmd.Flags().GetBool("external-access")
			update.ExternalAccess = &v
			hasUpdate = true
		}
		if cmd.Flags().Changed("security-entity") {
			v, _ := cmd.Flags().GetString("security-entity")
			update.SecurityEntity = &v
			hasUpdate = true
		}
		if cmd.Flags().Changed("comment-entity") {
			v, _ := cmd.Flags().GetString("comment-entity")
			update.CommentEntity = &v
			hasUpdate = true
		}
		if cmd.Flags().Changed("share-entity") {
			v, _ := cmd.Flags().GetString("share-entity")
			update.ShareEntity = &v
			hasUpdate = true
		}
		if cmd.Flags().Changed("link-share-entity") {
			v, _ := cmd.Flags().GetString("link-share-entity")
			update.LinkShareEntity = &v
			hasUpdate = true
		}
		if cmd.Flags().Changed("invite-external") {
			v, _ := cmd.Flags().GetBool("invite-external")
			update.InviteExternal = &v
			hasUpdate = true
		}

		if !hasUpdate {
			return clierr.Usagef("请至少指定一个要更新的权限字段")
		}

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printPermDryRun(cmd, "Update public permission settings", "PATCH",
				permAPIPath("/open-apis/drive/v1/permissions/%s/public", docToken),
				map[string]any{"type": docType}, publicPermissionUpdateBody(update))
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		result, err := client.UpdatePublicPermissionV2(docToken, docType, update, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("公共权限更新成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		return printJSON(result)
	},
}

// publicPermissionUpdateBody 返回只含已指定字段的 PATCH 请求体，供 --dry-run 预览。
func publicPermissionUpdateBody(u client.PublicPermissionUpdate) map[string]any {
	body := map[string]any{}
	if u.ExternalAccess != nil {
		body["external_access"] = *u.ExternalAccess
	}
	if u.SecurityEntity != nil {
		body["security_entity"] = *u.SecurityEntity
	}
	if u.CommentEntity != nil {
		body["comment_entity"] = *u.CommentEntity
	}
	if u.ShareEntity != nil {
		body["share_entity"] = *u.ShareEntity
	}
	if u.LinkShareEntity != nil {
		body["link_share_entity"] = *u.LinkShareEntity
	}
	if u.InviteExternal != nil {
		body["invite_external"] = *u.InviteExternal
	}
	return body
}

func init() {
	permCmd.AddCommand(updatePublicPermissionCmd)
	updatePublicPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	updatePublicPermissionCmd.Flags().Bool("external-access", false, "是否允许内容被分享到组织外")
	updatePublicPermissionCmd.Flags().String("security-entity", "", "谁可以复制内容、创建副本、打印、下载")
	updatePublicPermissionCmd.Flags().String("comment-entity", "", "谁可以评论")
	updatePublicPermissionCmd.Flags().String("share-entity", "", "谁可以添加和管理协作者")
	updatePublicPermissionCmd.Flags().String("link-share-entity", "", "链接分享设置")
	updatePublicPermissionCmd.Flags().Bool("invite-external", false, "是否允许非管理权限的人分享到组织外")
}
