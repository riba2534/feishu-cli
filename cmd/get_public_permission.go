package cmd

import (
	"fmt"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getPublicPermissionCmd = &cobra.Command{
	Use:   "public-get <doc_token>",
	Short: "获取文档公共权限设置",
	Long: `获取文档的公共权限设置，包括外部访问、链接分享、评论权限等。

参数:
  doc_token       文档 Token
  --doc-type      文档类型（默认: docx）

返回字段说明:
  external_access   是否允许内容被分享到组织外
  security_entity   谁可以复制内容、创建副本、打印、下载
  comment_entity    谁可以评论
  share_entity      谁可以添加和管理协作者
  link_share_entity 链接分享设置
  invite_external   是否允许非管理权限的人分享到组织外
  lock_switch       节点加锁状态

  以下为 v2 接口新增字段（主读取走 GET /open-apis/drive/v2/permissions/:token/public）:
  external_access_entity       对外分享范围（open / closed / allow_share_partner_tenant）
  copy_entity                  谁可以复制内容
  manage_collaborator_entity   谁可以管理协作者
  （external_access / invite_external 为 v1 字段，补读 v1 保持输出兼容）

示例:
  # 获取文档的公共权限设置
  feishu-cli perm public-get DOC_TOKEN

  # 获取电子表格的公共权限设置
  feishu-cli perm public-get DOC_TOKEN --doc-type sheet

  # 以当前登录用户身份读取个人文档
  feishu-cli perm public-get DOC_TOKEN --as user`,
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

		// 主读取走 v2（多出 copy_entity / manage_collaborator_entity / external_access_entity 等字段）
		v2, err := client.GetPublicPermissionV2(docToken, docType, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}
		// v2 不再返回 v1 的 external_access / invite_external：补读 v1 保持旧输出字段全部存在
		v1, v1Err := client.GetPublicPermission(docToken, docType, userToken)
		if v1Err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  补读 v1 公共权限字段失败，external_access 由 external_access_entity 推断，invite_external 缺省：%v\n", v1Err)
		}
		return printJSON(mergePublicPermission(v2, v1))
	},
}

// mergePublicPermission 以 v2 permission_public 为主，补齐 v1 独有的旧字段（external_access、invite_external），
// 保证升级 v2 后旧输出字段全部仍在；两者都有的字段以 v2 为准。
// v1 不可用时 external_access 由 external_access_entity 推断（closed → false，其余 → true）。
func mergePublicPermission(v2 map[string]any, v1 *larkdrive.PermissionPublic) map[string]any {
	out := make(map[string]any, len(v2)+2)
	for k, v := range v2 {
		out[k] = v
	}
	if v1 != nil {
		legacy := map[string]any{}
		if v1.ExternalAccess != nil {
			legacy["external_access"] = *v1.ExternalAccess
		}
		if v1.InviteExternal != nil {
			legacy["invite_external"] = *v1.InviteExternal
		}
		if v1.SecurityEntity != nil {
			legacy["security_entity"] = *v1.SecurityEntity
		}
		if v1.CommentEntity != nil {
			legacy["comment_entity"] = *v1.CommentEntity
		}
		if v1.ShareEntity != nil {
			legacy["share_entity"] = *v1.ShareEntity
		}
		if v1.LinkShareEntity != nil {
			legacy["link_share_entity"] = *v1.LinkShareEntity
		}
		if v1.LockSwitch != nil {
			legacy["lock_switch"] = *v1.LockSwitch
		}
		for k, v := range legacy {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}
	if _, ok := out["external_access"]; !ok {
		if entity, ok := out["external_access_entity"].(string); ok && entity != "" {
			out["external_access"] = entity != "closed"
		}
	}
	return out
}

func init() {
	permCmd.AddCommand(getPublicPermissionCmd)
	getPublicPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
}
