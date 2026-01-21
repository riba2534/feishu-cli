package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteBlocksCmd = &cobra.Command{
	Use:   "delete <document_id> <block_id>",
	Short: "删除父块下的子块",
	Long: `删除飞书文档中父块下的子块。

删除基于索引范围。可以指定起始和结束索引，
或使用 --all 删除所有子块。

示例:
  feishu-cli doc delete DOC_ID PARENT_BLOCK_ID --start 0 --end 3
  feishu-cli doc delete DOC_ID PARENT_BLOCK_ID --all`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		blockID := args[1]
		startIndex, _ := cmd.Flags().GetInt("start")
		endIndex, _ := cmd.Flags().GetInt("end")
		deleteAll, _ := cmd.Flags().GetBool("all")
		force, _ := cmd.Flags().GetBool("force")

		if deleteAll {
			// Get block children count first
			children, err := client.GetBlockChildren(documentID, blockID)
			if err != nil {
				return fmt.Errorf("获取子块失败: %w", err)
			}
			if len(children) == 0 {
				fmt.Println("没有可删除的子块")
				return nil
			}
			startIndex = 0
			endIndex = len(children)
		} else {
			// Validate index range when not using --all
			if !cmd.Flags().Changed("end") {
				return fmt.Errorf("必须指定 --all 或 --end")
			}
			if endIndex <= startIndex {
				return fmt.Errorf("结束索引 (%d) 必须大于起始索引 (%d)", endIndex, startIndex)
			}
			if startIndex < 0 {
				return fmt.Errorf("起始索引必须非负")
			}
		}

		// 危险操作确认
		if !force {
			prompt := fmt.Sprintf("确定要删除块 %s 下索引 %d 到 %d 的子块吗？此操作不可恢复", blockID, startIndex, endIndex)
			if !confirmAction(prompt) {
				fmt.Println("操作已取消")
				return nil
			}
		}

		if err := client.DeleteBlocks(documentID, blockID, startIndex, endIndex); err != nil {
			return err
		}

		fmt.Printf("成功删除索引 %d 到 %d 的块!\n", startIndex, endIndex)
		return nil
	},
}

func init() {
	docCmd.AddCommand(deleteBlocksCmd)
	deleteBlocksCmd.Flags().Int("start", 0, "起始索引 (从0开始)")
	deleteBlocksCmd.Flags().Int("end", 0, "结束索引 (不包含)")
	deleteBlocksCmd.Flags().Bool("all", false, "删除所有子块")
	deleteBlocksCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
}
