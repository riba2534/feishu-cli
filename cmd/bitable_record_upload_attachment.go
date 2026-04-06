package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var bitableRecordUploadAttachmentCmd = &cobra.Command{
	Use:   "record-upload-attachment <app_token> <table_id> <record_id>",
	Short: "向记录的附件字段上传文件",
	Long: `向多维表格记录的附件字段上传本地文件。

会自动追加到现有附件列表，不会覆盖已有附件。

流程：上传文件到 Drive → 读取当前记录 → 追加附件并更新记录

示例:
  # 上传文件到附件字段
  feishu-cli bitable record-upload-attachment APP_TOKEN TABLE_ID RECORD_ID \
    --field "附件" --file report.pdf

  # JSON 格式输出
  feishu-cli bitable record-upload-attachment APP_TOKEN TABLE_ID RECORD_ID \
    --field "附件" --file image.png -o json`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		appToken := args[0]
		tableID := args[1]
		recordID := args[2]
		fieldName, _ := cmd.Flags().GetString("field")
		filePath, _ := cmd.Flags().GetString("file")
		output, _ := cmd.Flags().GetString("output")
		userToken := resolveOptionalUserToken(cmd)

		fileName := filepath.Base(filePath)

		// ===== 步骤 1：上传文件到 Drive =====
		fileToken, _, err := client.UploadMedia(filePath, "bitable_file", appToken, fileName)
		if err != nil {
			return fmt.Errorf("步骤 1 失败 - 上传文件: %w", err)
		}

		// ===== 步骤 2：读取当前记录，获取现有附件列表 =====
		record, err := client.GetBitableRecord(appToken, tableID, recordID, userToken)
		if err != nil {
			return fmt.Errorf("步骤 2 失败 - 读取记录: %w", err)
		}

		// 构建新的附件列表（保留现有附件 + 追加新附件）
		var attachments []any
		if existingVal, ok := record.Fields[fieldName]; ok && existingVal != nil {
			if existingList, ok := existingVal.([]any); ok {
				attachments = append(attachments, existingList...)
			}
		}
		attachments = append(attachments, map[string]any{
			"file_token": fileToken,
		})

		// ===== 步骤 3：更新记录，将新附件追加到附件字段 =====
		fields := map[string]any{
			fieldName: attachments,
		}
		updatedRecord, err := client.UpdateBitableRecord(appToken, tableID, recordID, fields, userToken)
		if err != nil {
			return fmt.Errorf("步骤 3 失败 - 更新记录: %w", err)
		}

		// 输出结果
		result := map[string]any{
			"app_token":  appToken,
			"table_id":   tableID,
			"record_id":  updatedRecord.RecordID,
			"field_name": fieldName,
			"file_token": fileToken,
			"file":       filePath,
			"status":     "success",
		}

		if output == "json" {
			return printJSON(result)
		} else {
			fmt.Printf("上传成功！\n")
			fmt.Printf("  多维表格: %s\n", appToken)
			fmt.Printf("  数据表:   %s\n", tableID)
			fmt.Printf("  记录 ID:  %s\n", updatedRecord.RecordID)
			fmt.Printf("  附件字段: %s\n", fieldName)
			fmt.Printf("  文件:     %s\n", filePath)
			fmt.Printf("  文件 Token: %s\n", fileToken)
			fmt.Printf("  附件总数: %d\n", len(attachments))
		}

		return nil
	},
}

func init() {
	bitableCmd.AddCommand(bitableRecordUploadAttachmentCmd)

	bitableRecordUploadAttachmentCmd.Flags().String("field", "", "附件字段名称（必填）")
	bitableRecordUploadAttachmentCmd.Flags().String("file", "", "本地文件路径（必填）")
	bitableRecordUploadAttachmentCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	bitableRecordUploadAttachmentCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(bitableRecordUploadAttachmentCmd, "field", "file")
}
