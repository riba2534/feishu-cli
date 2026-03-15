package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createWikiNodeCmd = &cobra.Command{
	Use:   "create",
	Short: "创建知识库节点",
	Long: `在指定知识空间中创建新的节点（文档）。

参数:
  --space-id      知识空间 ID（必填）
  --title         节点标题（必填）
  --parent-node   父节点 Token（可选，不指定则创建在根目录）
  --obj-type      文档类型（可选，默认 docx）
  --node-type     节点类型（可选，默认 origin）

节点类型:
  docx      新版文档（默认）
  doc       旧版文档
  sheet     电子表格

示例:
  # 在知识空间根目录创建文档
  feishu-cli wiki create --space-id 7012345678901234567 --title "新文档"

  # 在指定父节点下创建文档
  feishu-cli wiki create --space-id 7012345678901234567 --title "子文档" --parent-node wikcnXXXXXX

  # 创建电子表格
  feishu-cli wiki create --space-id 7012345678901234567 --title "数据表" --obj-type sheet

  # JSON 格式输出
  feishu-cli wiki create --space-id 7012345678901234567 --title "新文档" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID, _ := cmd.Flags().GetString("space-id")
		title, _ := cmd.Flags().GetString("title")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		objType, _ := cmd.Flags().GetString("obj-type")
		nodeType, _ := cmd.Flags().GetString("node-type")
		output, _ := cmd.Flags().GetString("output")

		result, err := client.CreateWikiNode(spaceID, title, parentNode, objType, nodeType, resolveOptionalUserToken(cmd))
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Printf("知识库节点创建成功！\n")
			fmt.Printf("  空间 ID:    %s\n", result.SpaceID)
			fmt.Printf("  节点 Token: %s\n", result.NodeToken)
			fmt.Printf("  文档 Token: %s\n", result.ObjToken)
			fmt.Printf("  文档类型:   %s\n", result.ObjType)
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(createWikiNodeCmd)
	createWikiNodeCmd.Flags().String("space-id", "", "知识空间 ID（必填）")
	createWikiNodeCmd.Flags().String("title", "", "节点标题（必填）")
	createWikiNodeCmd.Flags().String("parent-node", "", "父节点 Token（可选）")
	createWikiNodeCmd.Flags().String("obj-type", "docx", "文档类型：docx/doc/sheet（默认 docx）")
	createWikiNodeCmd.Flags().String("node-type", "origin", "节点类型：origin/shortcut（默认 origin）")
	createWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(createWikiNodeCmd, "space-id", "title")
	createWikiNodeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
