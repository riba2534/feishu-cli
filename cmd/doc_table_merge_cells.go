package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var docTableMergeCellsCmd = &cobra.Command{
	Use:   "merge-cells <document_id|url> <table_block_id>",
	Short: "合并表格单元格",
	Long: `合并指定范围的单元格（左闭右开区间）。

参数:
  document_id     文档 ID 或 URL（/wiki/ URL 自动解析为底层文档）
  table_block_id  表格块 ID（Block 类型 31）
  --row-start     起始行索引（包含）
  --row-end       结束行索引（不包含）
  --col-start     起始列索引（包含）
  --col-end       结束列索引（不包含）

示例:
  # 合并 A1:C2 区域（行 0-2，列 0-3）
  feishu-cli doc table merge-cells DOC_ID TABLE_BLOCK_ID \
    --row-start 0 --row-end 2 --col-start 0 --col-end 3`,
	Args: cobra.ExactArgs(2),
	RunE: runDocTableMergeCells,
}

func init() {
	docTableCmd.AddCommand(docTableMergeCellsCmd)
	docTableMergeCellsCmd.Flags().Int("row-start", 0, "起始行索引（包含）")
	docTableMergeCellsCmd.Flags().Int("row-end", 0, "结束行索引（不包含）")
	docTableMergeCellsCmd.Flags().Int("col-start", 0, "起始列索引（包含）")
	docTableMergeCellsCmd.Flags().Int("col-end", 0, "结束列索引（不包含）")
	docTableMergeCellsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	docTableMergeCellsCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docTableMergeCellsCmd, "row-start", "row-end", "col-start", "col-end")
}

func runDocTableMergeCells(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}

	tableBlockID := args[1]
	rowStart, _ := cmd.Flags().GetInt("row-start")
	rowEnd, _ := cmd.Flags().GetInt("row-end")
	colStart, _ := cmd.Flags().GetInt("col-start")
	colEnd, _ := cmd.Flags().GetInt("col-end")
	output, _ := cmd.Flags().GetString("output")
	userAccessToken := resolveOptionalUserToken(cmd)
	// 支持文档 ID、/docx/ URL 与 /wiki/ URL（wiki 自动解包为底层 docx 的 obj_token）
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}

	if rowStart < 0 || colStart < 0 {
		return fmt.Errorf("起始索引不能为负数")
	}
	if rowEnd <= rowStart {
		return fmt.Errorf("行结束索引必须大于起始索引")
	}
	if colEnd <= colStart {
		return fmt.Errorf("列结束索引必须大于起始索引")
	}

	err = client.MergeTableCells(documentID, tableBlockID, rowStart, rowEnd, colStart, colEnd, userAccessToken)
	if err != nil {
		return fmt.Errorf("合并单元格失败: %w", err)
	}

	if output == "json" {
		return printJSON(map[string]any{
			"document_id":        documentID,
			"table_block_id":     tableBlockID,
			"operation":          "merge_cells",
			"row_start_index":    rowStart,
			"row_end_index":      rowEnd,
			"column_start_index": colStart,
			"column_end_index":   colEnd,
		})
	}

	fmt.Printf("已成功合并单元格（行 %d-%d，列 %d-%d）\n", rowStart, rowEnd-1, colStart, colEnd-1)
	return nil
}
