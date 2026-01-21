package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteWikiNodeCmd = &cobra.Command{
	Use:   "delete <node_token>",
	Short: "删除知识库节点",
	Long: `删除知识库节点（将节点对应的文档移至回收站）。

参数:
  node_token    节点 Token（必填）

警告:
  删除操作会将文档移至回收站，请谨慎操作！

注意:
  - 此命令通过 Drive API 删除节点对应的文档
  - 删除操作可能是异步的，会返回任务 ID

示例:
  # 删除节点
  feishu-cli wiki delete wikcnXXXXXX`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nodeToken := extractWikiToken(args[0])
		force, _ := cmd.Flags().GetBool("force")

		// 先获取节点信息以获取 obj_token 和 obj_type
		node, err := client.GetWikiNode(nodeToken)
		if err != nil {
			return fmt.Errorf("获取节点信息失败: %w", err)
		}

		// 危险操作确认
		if !force {
			if !confirmAction(fmt.Sprintf("确定要删除知识库节点 \"%s\" (%s) 吗？此操作会将文档移至回收站", node.Title, nodeToken)) {
				fmt.Println("操作已取消")
				return nil
			}
		}

		// 通过 Drive API 删除对应的文档
		taskID, err := client.DeleteFile(node.ObjToken, node.ObjType)
		if err != nil {
			return err
		}

		fmt.Printf("知识库节点删除操作已提交！\n")
		fmt.Printf("  节点 Token: %s\n", nodeToken)
		fmt.Printf("  文档 Token: %s\n", node.ObjToken)
		fmt.Printf("  文档类型:   %s\n", node.ObjType)
		fmt.Printf("  标题:       %s\n", node.Title)
		if taskID != "" {
			fmt.Printf("  任务 ID:    %s\n", taskID)
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(deleteWikiNodeCmd)
	deleteWikiNodeCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
}
