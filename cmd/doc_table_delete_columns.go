package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var docTableDeleteColumnsCmd = &cobra.Command{
	Use:   "delete-columns <document_id|url> <table_block_id>",
	Short: "删除表格中的列",
	Long: `删除指定范围的列（左闭右开区间）。

参数:
  document_id     文档 ID 或 URL（/wiki/ URL 自动解析为底层文档）
  table_block_id  表格块 ID（Block 类型 31）
  --start         起始列索引（包含，0 表示第一列）
  --end           结束列索引（不包含）

示例:
  # 删除第 2-3 列（索引 1 到 3，左闭右开）
  feishu-cli doc table delete-columns DOC_ID TABLE_BLOCK_ID --start 1 --end 3

  # 删除第一列
  feishu-cli doc table delete-columns DOC_ID TABLE_BLOCK_ID --start 0 --end 1`,
	Args: cobra.ExactArgs(2),
	RunE: runDocTableDeleteColumns,
}

func init() {
	docTableCmd.AddCommand(docTableDeleteColumnsCmd)
	docTableDeleteColumnsCmd.Flags().Int("start", 0, "起始列索引（包含）")
	docTableDeleteColumnsCmd.Flags().Int("end", 0, "结束列索引（不包含）")
	docTableDeleteColumnsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	docTableDeleteColumnsCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docTableDeleteColumnsCmd, "start", "end")
}

func runDocTableDeleteColumns(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}

	tableBlockID := args[1]
	start, _ := cmd.Flags().GetInt("start")
	end, _ := cmd.Flags().GetInt("end")
	output, _ := cmd.Flags().GetString("output")
	userAccessToken := resolveOptionalUserToken(cmd)
	// 支持文档 ID、/docx/ URL 与 /wiki/ URL（wiki 自动解包为底层 docx 的 obj_token）
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}

	if start < 0 {
		return fmt.Errorf("起始索引不能为负数")
	}
	if end <= start {
		return fmt.Errorf("结束索引必须大于起始索引")
	}

	err = client.DeleteTableColumns(documentID, tableBlockID, start, end, userAccessToken)
	if err != nil {
		return fmt.Errorf("删除列失败: %w", err)
	}

	deletedCount := end - start
	if output == "json" {
		return printJSON(map[string]any{
			"document_id":        documentID,
			"table_block_id":     tableBlockID,
			"operation":          "delete_columns",
			"column_start_index": start,
			"column_end_index":   end,
			"deleted_count":      deletedCount,
		})
	}

	fmt.Printf("已成功删除 %d 列（索引 %d 到 %d）\n", deletedCount, start, end-1)
	return nil
}
