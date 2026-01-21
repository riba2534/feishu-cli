package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var moveWikiNodeCmd = &cobra.Command{
	Use:   "move <node_token>",
	Short: "移动知识库节点",
	Long: `移动知识库节点到指定位置，支持跨知识空间移动。

如果节点有子节点，会携带子节点一起移动。

参数:
  node_token       节点 Token（必填）
  --target-space   目标知识空间 ID（必填）
  --target-parent  目标父节点 Token（可选，不指定则移动到根目录）

权限要求:
  - 节点编辑权限
  - 原父节点容器编辑权限
  - 目标父节点容器编辑权限

示例:
  # 移动到同一空间的根目录
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567

  # 移动到同一空间的指定父节点下
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567 --target-parent wikcnYYYYYY

  # 跨空间移动
  feishu-cli wiki move wikcnXXXXXX --target-space 7098765432109876543

  # JSON 格式输出
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nodeToken := extractWikiToken(args[0])
		targetSpace, _ := cmd.Flags().GetString("target-space")
		targetParent, _ := cmd.Flags().GetString("target-parent")
		output, _ := cmd.Flags().GetString("output")

		// 先获取节点信息以获取当前 space_id
		node, err := client.GetWikiNode(nodeToken)
		if err != nil {
			return fmt.Errorf("获取节点信息失败: %w", err)
		}

		result, err := client.MoveWikiNode(node.SpaceID, nodeToken, targetSpace, targetParent)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("知识库节点移动成功！\n")
			fmt.Printf("  节点 Token:     %s\n", result.NodeToken)
			fmt.Printf("  目标空间 ID:    %s\n", targetSpace)
			if targetParent != "" {
				fmt.Printf("  目标父节点:     %s\n", targetParent)
			} else {
				fmt.Printf("  目标位置:       根目录\n")
			}
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(moveWikiNodeCmd)
	moveWikiNodeCmd.Flags().String("target-space", "", "目标知识空间 ID（必填）")
	moveWikiNodeCmd.Flags().String("target-parent", "", "目标父节点 Token（可选）")
	moveWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	_ = moveWikiNodeCmd.MarkFlagRequired("target-space")
}
