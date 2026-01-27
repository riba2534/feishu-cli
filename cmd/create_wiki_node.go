package cmd

import (
	"encoding/json"
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
  --node-type     节点类型（可选，默认 docx）

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
  feishu-cli wiki create --space-id 7012345678901234567 --title "数据表" --node-type sheet

  # JSON 格式输出
  feishu-cli wiki create --space-id 7012345678901234567 --title "新文档" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID, _ := cmd.Flags().GetString("space-id")
		title, _ := cmd.Flags().GetString("title")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		nodeType, _ := cmd.Flags().GetString("node-type")
		output, _ := cmd.Flags().GetString("output")

		result, err := client.CreateWikiNode(spaceID, title, parentNode, nodeType)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
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
	createWikiNodeCmd.Flags().String("node-type", "docx", "节点类型：docx/doc/sheet（默认 docx）")
	createWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(createWikiNodeCmd, "space-id", "title")
}
