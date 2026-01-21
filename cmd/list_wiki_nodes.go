package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listWikiNodesCmd = &cobra.Command{
	Use:   "nodes <space_id>",
	Short: "列出空间下的节点",
	Long: `列出指定知识空间下的节点列表。

参数:
  space_id          知识空间 ID
  --parent          父节点 Token（不指定则列出根节点）

节点类型（obj_type）:
  docx      新版文档
  doc       旧版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      文件
  slides    幻灯片

示例:
  # 列出空间根节点
  feishu-cli wiki nodes 7012345678901234567

  # 列出指定父节点下的子节点
  feishu-cli wiki nodes 7012345678901234567 --parent Ad8Iw0oz3iSp4kkIi7Q

  # JSON 格式输出
  feishu-cli wiki nodes 7012345678901234567 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		parentToken, _ := cmd.Flags().GetString("parent")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		nodes, _, _, err := client.ListWikiNodes(spaceID, parentToken, pageSize, "")
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(nodes, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(nodes) == 0 {
				fmt.Println("未找到节点")
				return nil
			}

			fmt.Printf("共找到 %d 个节点:\n\n", len(nodes))
			for i, node := range nodes {
				childMark := ""
				if node.HasChild {
					childMark = " [有子节点]"
				}
				fmt.Printf("[%d] %s (%s)%s\n", i+1, node.Title, node.ObjType, childMark)
				fmt.Printf("    节点 Token: %s\n", node.NodeToken)
				fmt.Printf("    文档 Token: %s\n", node.ObjToken)
				fmt.Println()
			}
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(listWikiNodesCmd)
	listWikiNodesCmd.Flags().String("parent", "", "父节点 Token（不指定则列出根节点）")
	listWikiNodesCmd.Flags().Int("page-size", 50, "每页数量")
	listWikiNodesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
