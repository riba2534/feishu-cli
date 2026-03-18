package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var transferOwnerCmd = &cobra.Command{
	Use:   "transfer-owner <doc_token>",
	Short: "转移文档所有权",
	Long: `将文档所有权转移给指定用户。

参数:
  doc_token            文档 Token
  --doc-type           文档类型（默认: docx）
  --member-type        新所有者类型（必填）
  --member-id          新所有者标识（必填）
  --notification       通知新所有者（默认: true）
  --remove-old-owner   移除原所有者权限（默认: false）
  --stay-put           文档保留在原位置（默认: false）
  --old-owner-perm     原所有者保留权限（默认: full_access，仅 remove-old-owner=false 时生效）

成员类型:
  email             飞书邮箱
  openid/open_id    开放平台 ID
  userid/user_id    用户自定义 ID

文档类型:
  docx      新版文档（默认）
  doc       旧版文档
  sheet     电子表格
  bitable   多维表格
  wiki      知识库节点
  file      云空间文件
  folder    文件夹
  mindnote  思维笔记
  minutes   妙记
  slides    幻灯片

示例:
  # 通过邮箱转移文档所有权
  feishu-cli perm transfer-owner DOC_TOKEN \
    --member-type email \
    --member-id user@example.com

  # 转移所有权并移除原所有者
  feishu-cli perm transfer-owner DOC_TOKEN \
    --member-type email \
    --member-id user@example.com \
    --remove-old-owner

  # 转移所有权，原所有者保留查看权限
  feishu-cli perm transfer-owner DOC_TOKEN \
    --member-type email \
    --member-id user@example.com \
    --old-owner-perm view`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		notification, _ := cmd.Flags().GetBool("notification")
		removeOldOwner, _ := cmd.Flags().GetBool("remove-old-owner")
		stayPut, _ := cmd.Flags().GetBool("stay-put")
		oldOwnerPerm, _ := cmd.Flags().GetString("old-owner-perm")

		memberType = normalizePermMemberType(memberType)

		if err := client.TransferOwnership(docToken, docType, memberType, memberID, notification, removeOldOwner, stayPut, oldOwnerPerm); err != nil {
			return err
		}

		fmt.Printf("文档所有权转移成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  新所有者: %s（%s）\n", memberID, memberType)
		if removeOldOwner {
			fmt.Printf("  原所有者: 已移除\n")
		} else {
			fmt.Printf("  原所有者保留权限: %s\n", oldOwnerPerm)
		}
		return nil
	},
}

func init() {
	permCmd.AddCommand(transferOwnerCmd)
	transferOwnerCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	transferOwnerCmd.Flags().String("member-type", "", "新所有者类型（email/openid/open_id/userid/user_id）")
	transferOwnerCmd.Flags().String("member-id", "", "新所有者标识")
	transferOwnerCmd.Flags().Bool("notification", true, "通知新所有者")
	transferOwnerCmd.Flags().Bool("remove-old-owner", false, "移除原所有者权限")
	transferOwnerCmd.Flags().Bool("stay-put", false, "文档保留在原位置")
	transferOwnerCmd.Flags().String("old-owner-perm", "full_access", "原所有者保留权限（view/edit/full_access）")
	mustMarkFlagRequired(transferOwnerCmd, "member-type", "member-id")
}
