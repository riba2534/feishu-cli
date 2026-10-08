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
		userAccessToken := resolveOptionalUserToken(cmd)

		// 建立一致性 revision 快照：先取得当前正整数版本号，无法取得时不可假装受保护
		docRev, err := client.GetDocumentRevision(documentID, userAccessToken)
		if err != nil {
			return fmt.Errorf("获取文档版本失败（无法建立安全快照）: %w", err)
		}
		if docRev <= 0 {
			return fmt.Errorf("无法取得有效正整数文档版本号（docRev=%d，无法建立一致性安全快照）", docRev)
		}

		if deleteAll {
			// 获取全部子块（基于同一 docRev 快照全分页拉取，避免仅读第一页导致遗漏或谎报全删）
			children, err := client.GetAllBlockChildrenWithRevision(documentID, blockID, docRev, userAccessToken)
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
			if err := confirmDangerousAction(cmd, prompt); err != nil {
				return err
			}
		}

		// 使用同一 docRev 发起带版本锁的删除；若期间发生并发冲突服务端将直接拒绝并返回非零错误
		if _, err := client.DeleteBlocksWithRevision(documentID, blockID, startIndex, endIndex, docRev, userAccessToken); err != nil {
			return err
		}

		if deleteAll {
			fmt.Printf("成功删除所有子块（共 %d 个）！\n", endIndex)
		} else {
			fmt.Printf("成功删除索引 %d 到 %d 的块！\n", startIndex, endIndex)
		}
		return nil
	},
}

func init() {
	docCmd.AddCommand(deleteBlocksCmd)
	deleteBlocksCmd.Flags().Int("start", 0, "起始索引 (从0开始)")
	deleteBlocksCmd.Flags().Int("end", 0, "结束索引 (不包含)")
	deleteBlocksCmd.Flags().Bool("all", false, "删除所有子块")
	deleteBlocksCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
	deleteBlocksCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文档）")
}
