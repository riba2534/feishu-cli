package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ==================== record batch-get 批量获取记录 ====================
var bitableRecordBatchGetCmd = &cobra.Command{
	Use:   "batch-get",
	Short: "批量获取记录（POST batch_get）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/tables/{table_id}/records/batch_get

base/v3 批量获取记录，body 字段为 record_id_list。

必填:
  --table-id     目标数据表
  --record-ids   逗号分隔的 record_id 列表
可选:
  --field-id           仅返回指定字段（投影，可重复，最多 100 个；对应 body select_fields）
  --with-shared-url    返回记录分享链接
  --automatic-fields   返回自动计算字段
  --user-id-type       用户字段 ID 类型（open_id/union_id/user_id）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
		}
		idsCSV, _ := cmd.Flags().GetString("record-ids")
		ids := splitAndTrim(idsCSV)
		if len(ids) == 0 {
			return fmt.Errorf("--record-ids 必填（逗号分隔）")
		}

		body := map[string]any{"record_id_list": ids}
		selectFields, err := recordSelectFields(cmd, 100)
		if err != nil {
			return err
		}
		if len(selectFields) > 0 {
			body["select_fields"] = selectFields
		}
		if cmd.Flags().Changed("with-shared-url") {
			v, _ := cmd.Flags().GetBool("with-shared-url")
			body["with_shared_url"] = v
		}
		if cmd.Flags().Changed("automatic-fields") {
			v, _ := cmd.Flags().GetBool("automatic-fields")
			body["automatic_fields"] = v
		}
		if uit, _ := cmd.Flags().GetString("user-id-type"); uit != "" {
			body["user_id_type"] = uit
		}

		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: bitableRecordPath(bt, tableID, "batch_get"), body: body}
		})
	},
}

func init() {
	bitableRecordCmd.AddCommand(bitableRecordBatchGetCmd)
	addBitableCommonFlags(bitableRecordBatchGetCmd)
	bitableRecordBatchGetCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableRecordBatchGetCmd.Flags().String("record-ids", "", "record_id 列表（逗号分隔，必填）")
	bitableRecordBatchGetCmd.Flags().StringArray("field-id", nil, "仅返回指定字段（字段名或字段 ID，可重复，最多 100 个）")
	bitableRecordBatchGetCmd.Flags().Bool("with-shared-url", false, "返回记录分享链接")
	bitableRecordBatchGetCmd.Flags().Bool("automatic-fields", false, "返回自动计算字段")
	bitableRecordBatchGetCmd.Flags().String("user-id-type", "", "用户字段 ID 类型（open_id/union_id/user_id）")
}
