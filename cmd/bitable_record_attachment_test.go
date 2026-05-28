package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ---- attachments cell body 结构 ----

func TestBitableAttachmentCellBody(t *testing.T) {
	body := bitableAttachmentCellBody("recX", "fldY", []string{"box1", "box2"})
	att, ok := body["attachments"].(map[string]any)
	if !ok {
		t.Fatalf("缺 attachments: %v", body)
	}
	rec, ok := att["recX"].(map[string]any)
	if !ok {
		t.Fatalf("缺 record 层: %v", att)
	}
	items, ok := rec["fldY"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("缺 field 层附件数组: %v", rec)
	}
	if items[0]["file_token"] != "box1" || items[1]["file_token"] != "box2" {
		t.Errorf("file_token 不对: %v", items)
	}
}

// ---- extractAttachmentFileTokens 容错遍历 ----

func TestExtractAttachmentFileTokens(t *testing.T) {
	data := map[string]any{
		"records": []any{
			map[string]any{
				"fields": map[string]any{
					"附件": []any{
						map[string]any{"file_token": "boxA", "name": "a.pdf"},
						map[string]any{"file_token": "boxB"},
					},
				},
			},
		},
	}
	tokens := extractAttachmentFileTokens(data)
	if len(tokens) != 2 {
		t.Fatalf("应提取 2 个 token: %v", tokens)
	}
	got := strings.Join(tokens, ",")
	if !strings.Contains(got, "boxA") || !strings.Contains(got, "boxB") {
		t.Errorf("token 提取不对: %v", tokens)
	}
}

// ---- upload-attachment dry-run（多步编排预览） ----

func TestRecordUploadAttachmentDryRun(t *testing.T) {
	initTestConfig(t)
	// 新建独立命令实例，避免 StringArray flag 跨用例累积。
	cmd := &cobra.Command{Use: "upload-attachment", RunE: bitableRecordUploadAttachmentCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().String("field-id", "", "")
	cmd.Flags().StringArray("file", nil, "")

	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"field-id": "fld1", "file": "./a.pdf", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_all") {
		t.Errorf("upload 步骤端点不对: %s", out)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/append_attachments") {
		t.Errorf("append 步骤端点不对: %s", out)
	}
	if !strings.Contains(out, "bitable_file") {
		t.Errorf("upload parent_type 应为 bitable_file: %s", out)
	}
}

// ---- download-attachment dry-run ----

func TestRecordDownloadAttachmentDryRunAll(t *testing.T) {
	initTestConfig(t)
	cmd := &cobra.Command{Use: "download-attachment", RunE: bitableRecordDownloadAttachmentCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().StringArray("file-token", nil, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("overwrite", false, "")

	// 省略 file-token → 应含 get_attachments 步骤
	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/get_attachments") {
		t.Errorf("省略 file-token 时应先 get_attachments: %s", out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/medias/<file_token>/download") {
		t.Errorf("应含 medias download 步骤: %s", out)
	}
}

func TestRecordDownloadAttachmentDryRunSingleToken(t *testing.T) {
	initTestConfig(t)
	cmd := &cobra.Command{Use: "download-attachment", RunE: bitableRecordDownloadAttachmentCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().StringArray("file-token", nil, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("overwrite", false, "")

	// 指定单个 file-token → 不应有 get_attachments 步骤
	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"file-token": "boxA", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if strings.Contains(out, "get_attachments") {
		t.Errorf("指定 file-token 时不应有 get_attachments 步骤: %s", out)
	}
}

// ---- remove-attachment dry-run ----

func TestRecordRemoveAttachmentDryRun(t *testing.T) {
	initTestConfig(t)
	cmd := &cobra.Command{Use: "remove-attachment", RunE: bitableRecordRemoveAttachmentCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().String("field-id", "", "")
	cmd.Flags().StringArray("file-token", nil, "")

	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"field-id": "fld1", "file-token": "boxA", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, `"method": "POST"`) ||
		!strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/remove_attachments") {
		t.Errorf("remove 端点/方法不对: %s", out)
	}
	if !strings.Contains(out, `"file_token": "boxA"`) {
		t.Errorf("remove body 应含 file_token: %s", out)
	}
}

func TestRecordRemoveAttachmentRequiresToken(t *testing.T) {
	initTestConfig(t)
	cmd := &cobra.Command{Use: "remove-attachment", RunE: bitableRecordRemoveAttachmentCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().String("field-id", "", "")
	cmd.Flags().StringArray("file-token", nil, "")

	_, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1", "field-id": "fld1", "dry-run": "true",
	})
	if err == nil {
		t.Error("缺 --file-token 应报错")
	}
}
