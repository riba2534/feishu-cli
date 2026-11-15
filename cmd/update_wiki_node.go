package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updateWikiNodeCmd = &cobra.Command{
	Use:   "update <node_token>",
	Short: "更新知识库节点标题",
	Long: `更新知识库节点的标题。

参数:
  node_token    节点 Token（必填）
  --title       新标题（必填）
  --dry-run     只打印将要发出的请求（不读取凭据、不联网；space_id 以占位符表示）

注意:
  - 此接口需要先获取节点所在的知识空间 ID
  - 仅支持更新文档(doc)、新版文档(docx)和快捷方式类型的节点

示例:
  # 更新节点标题
  feishu-cli wiki update wikcnXXXXXX --title "新标题"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeToken, err := extractWikiLookupToken(args[0])
		if err != nil {
			return clierr.Usage(err)
		}
		title, _ := cmd.Flags().GetString("title")

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, "Update wiki node title", map[string]any{"node_token": nodeToken}, []dryRunStep{
				{Method: "GET", URL: client.WikiNodeByTokenPath, Params: map[string]any{"token": nodeToken}, Desc: "Resolve space_id and real node_token"},
				{Method: "POST", URL: "/open-apis/wiki/v2/spaces/<space_id>/nodes/<node_token>/update_title", Body: map[string]any{"title": title}},
			})
		}

		if err := config.Validate(); err != nil {
			return err
		}

		// 先通过 node_by_token 获取 space_id 与真实 node_token（输入可能是文档 obj_token）
		token := resolveOptionalUserToken(cmd)
		node, err := client.ResolveWikiNode(nodeToken, token)
		if err != nil {
			return fmt.Errorf("获取节点信息失败: %w", err)
		}

		err = client.UpdateWikiNode(node.SpaceID, node.NodeToken, title, token)
		if err != nil {
			return err
		}

		fmt.Printf("知识库节点标题更新成功！\n")
		fmt.Printf("  节点 Token: %s\n", node.NodeToken)
		fmt.Printf("  新标题:     %s\n", title)

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(updateWikiNodeCmd)
	updateWikiNodeCmd.Flags().String("title", "", "新标题（必填）")
	updateWikiNodeCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求（不读取凭据、不联网）")
	mustMarkFlagRequired(updateWikiNodeCmd, "title")
	updateWikiNodeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
