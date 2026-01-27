package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
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

注意:
  - 此接口需要先获取节点所在的知识空间 ID
  - 仅支持更新文档(doc)、新版文档(docx)和快捷方式类型的节点

示例:
  # 更新节点标题
  feishu-cli wiki update wikcnXXXXXX --title "新标题"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nodeToken := extractWikiToken(args[0])
		title, _ := cmd.Flags().GetString("title")

		// 先获取节点信息以获取 space_id
		node, err := client.GetWikiNode(nodeToken)
		if err != nil {
			return fmt.Errorf("获取节点信息失败: %w", err)
		}

		err = client.UpdateWikiNode(node.SpaceID, nodeToken, title)
		if err != nil {
			return err
		}

		fmt.Printf("知识库节点标题更新成功！\n")
		fmt.Printf("  节点 Token: %s\n", nodeToken)
		fmt.Printf("  新标题:     %s\n", title)

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(updateWikiNodeCmd)
	updateWikiNodeCmd.Flags().String("title", "", "新标题（必填）")
	mustMarkFlagRequired(updateWikiNodeCmd, "title")
}
