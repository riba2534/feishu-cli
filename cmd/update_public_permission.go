package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
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
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")

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
			return fmt.Errorf("请至少指定一个要更新的权限字段")
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
