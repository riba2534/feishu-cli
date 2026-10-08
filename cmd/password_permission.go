package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var passwordCmd = &cobra.Command{
	Use:   "password",
	Short: "文档密码管理",
	Long: `管理文档的分享密码，支持创建、更新和删除密码。

子命令:
  create    创建文档密码
  update    刷新文档密码
  delete    删除文档密码

示例:
  # 创建文档密码
  feishu-cli perm password create DOC_TOKEN

  # 刷新文档密码
  feishu-cli perm password update DOC_TOKEN

  # 删除文档密码
  feishu-cli perm password delete DOC_TOKEN`,
}

var passwordCreateCmd = &cobra.Command{
	Use:   "create <doc_token>",
	Short: "创建文档密码",
	Long: `为文档创建分享密码。

参数:
  doc_token     文档 Token
  --doc-type    文档类型（默认: docx）

示例:
  feishu-cli perm password create DOC_TOKEN
  feishu-cli perm password create DOC_TOKEN --doc-type sheet`,
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
		password, err := client.CreatePublicPassword(docToken, docType, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("文档密码创建成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  密码: %s\n", password)
		return nil
	},
}

var passwordUpdateCmd = &cobra.Command{
	Use:   "update <doc_token>",
	Short: "刷新文档密码",
	Long: `刷新文档的分享密码，生成新密码。

参数:
  doc_token     文档 Token
  --doc-type    文档类型（默认: docx）

示例:
  feishu-cli perm password update DOC_TOKEN
  feishu-cli perm password update DOC_TOKEN --doc-type sheet`,
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
		password, err := client.UpdatePublicPassword(docToken, docType, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("文档密码刷新成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  密码: %s\n", password)
		return nil
	},
}

var passwordDeleteCmd = &cobra.Command{
	Use:   "delete <doc_token>",
	Short: "删除文档密码",
	Long: `删除文档的分享密码。

参数:
  doc_token     文档 Token
  --doc-type    文档类型（默认: docx）

示例:
  feishu-cli perm password delete DOC_TOKEN
  feishu-cli perm password delete DOC_TOKEN --doc-type sheet`,
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
		if err := client.DeletePublicPassword(docToken, docType, userToken); err != nil {
			return wrapPermError(err, userToken)
		}

		fmt.Printf("文档密码删除成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		return nil
	},
}

func init() {
	permCmd.AddCommand(passwordCmd)

	passwordCmd.AddCommand(passwordCreateCmd)
	passwordCreateCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")

	passwordCmd.AddCommand(passwordUpdateCmd)
	passwordUpdateCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")

	passwordCmd.AddCommand(passwordDeleteCmd)
	passwordDeleteCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
}
