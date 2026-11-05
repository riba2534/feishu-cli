package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listPermissionCmd = &cobra.Command{
	Use:   "list <doc_token>",
	Short: "查看协作者列表",
	Long: `查看文档的所有协作者及其权限。

参数:
  doc_token       文档 Token 或文档链接（链接自动推断 token 与类型，/wiki/ 链接对应 wiki）
  --doc-type      文档类型（默认: docx；传链接时可省略，显式传入时须与链接类型一致）
  --dry-run       只打印将要发出的请求（不读取凭据、不联网）

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
  # 查看文档的协作者列表
  feishu-cli perm list DOC_TOKEN

  # 查看电子表格的协作者列表
  feishu-cli perm list DOC_TOKEN --doc-type sheet

  # 直接传文档链接（自动推断 token 与 doc-type）
  feishu-cli perm list https://example.feishu.cn/wiki/wikcnxxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		docToken, docType, err := resolvePermTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printPermDryRun(cmd, "List document collaborators", "GET",
				permAPIPath("/open-apis/drive/v1/permissions/%s/members", docToken), map[string]any{"type": docType}, nil)
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolvePermIdentity(cmd)
		if err != nil {
			return err
		}
		members, err := client.ListPermission(docToken, docType, userToken)
		if err != nil {
			return wrapPermError(err, userToken)
		}

		return printJSON(members)
	},
}

func init() {
	permCmd.AddCommand(listPermissionCmd)
	listPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
}
