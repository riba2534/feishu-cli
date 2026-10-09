package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var docTableUnmergeCellsCmd = &cobra.Command{
	Use:   "unmerge-cells <document_id|url> <table_block_id>",
	Short: "取消合并单元格",
	Long: `取消指定单元格的合并状态。

参数:
  document_id     文档 ID 或 URL（/wiki/ URL 自动解析为底层文档）
  table_block_id  表格块 ID（Block 类型 31）
  --row           单元格所在行索引
  --col           单元格所在列索引

示例:
  # 取消 A1 单元格的合并
  feishu-cli doc table unmerge-cells DOC_ID TABLE_BLOCK_ID --row 0 --col 0`,
	Args: cobra.ExactArgs(2),
	RunE: runDocTableUnmergeCells,
}

func init() {
	docTableCmd.AddCommand(docTableUnmergeCellsCmd)
	docTableUnmergeCellsCmd.Flags().Int("row", 0, "单元格所在行索引")
	docTableUnmergeCellsCmd.Flags().Int("col", 0, "单元格所在列索引")
	docTableUnmergeCellsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	docTableUnmergeCellsCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docTableUnmergeCellsCmd, "row", "col")
}

func runDocTableUnmergeCells(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}

	tableBlockID := args[1]
	row, _ := cmd.Flags().GetInt("row")
	col, _ := cmd.Flags().GetInt("col")
	output, _ := cmd.Flags().GetString("output")
	userAccessToken := resolveOptionalUserToken(cmd)
	// 支持文档 ID、/docx/ URL 与 /wiki/ URL（wiki 自动解包为底层 docx 的 obj_token）
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}

	if row < 0 || col < 0 {
		return fmt.Errorf("行列索引不能为负数")
	}

	err = client.UnmergeTableCells(documentID, tableBlockID, row, col, userAccessToken)
	if err != nil {
		return fmt.Errorf("取消合并失败: %w", err)
	}

	if output == "json" {
		return printJSON(map[string]any{
			"document_id":    documentID,
			"table_block_id": tableBlockID,
			"operation":      "unmerge_cells",
			"row_index":      row,
			"column_index":   col,
		})
	}

	fmt.Printf("已成功取消单元格合并（行 %d，列 %d）\n", row, col)
	return nil
}
