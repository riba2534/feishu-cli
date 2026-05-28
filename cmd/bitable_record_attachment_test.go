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

// ---- extractAttachmentMetas 容错遍历 + 捕获 extra_info ----

func TestExtractAttachmentMetas(t *testing.T) {
	data := map[string]any{
		"records": []any{
			map[string]any{
				"fields": map[string]any{
					"附件": []any{
						map[string]any{"file_token": "boxA", "name": "a.pdf", "extra_info": "extraA"},
						map[string]any{"file_token": "boxB"},
					},
				},
			},
		},
	}
	metas := extractAttachmentMetas(data)
	if len(metas) != 2 {
		t.Fatalf("应提取 2 个附件: %v", metas)
	}
	byToken := map[string]string{}
	for _, m := range metas {
		byToken[m.FileToken] = m.ExtraInfo
	}
	if extra, ok := byToken["boxA"]; !ok || extra != "extraA" {
		t.Errorf("boxA 应带 extra_info=extraA: %v", metas)
	}
	if extra, ok := byToken["boxB"]; !ok || extra != "" {
		t.Errorf("boxB 应存在且 extra_info 为空: %v", metas)
	}
}

// ---- selectAttachmentMetas 过滤 ----

func TestSelectAttachmentMetas(t *testing.T) {
	metas := []attachmentMeta{
		{FileToken: "boxA", ExtraInfo: "ea"},
		{FileToken: "boxB", ExtraInfo: "eb"},
		{FileToken: "boxC"},
	}
	// 空 wanted → 全部
	if got := selectAttachmentMetas(metas, nil); len(got) != 3 {
		t.Errorf("空 wanted 应返回全部: %v", got)
	}
	// 过滤指定 token，按 metas 顺序，忽略不存在的
	got := selectAttachmentMetas(metas, []string{"boxC", "boxA", "nope"})
	if len(got) != 2 || got[0].FileToken != "boxA" || got[1].FileToken != "boxC" {
		t.Errorf("过滤结果不对（应保留 boxA,boxC 按出现顺序）: %v", got)
	}
}

// TestMissingFileTokens 验证多 token 部分缺失时能列出未匹配的 token（保持顺序、去重）。
func TestMissingFileTokens(t *testing.T) {
	selected := []attachmentMeta{
		{FileToken: "boxA"},
		{FileToken: "boxC"},
	}
	// wanted 含两个缺失（nope1/nope2），顺序保持，重复去重
	missing := missingFileTokens([]string{"boxA", "nope1", "boxC", "nope2", "nope1"}, selected)
	if len(missing) != 2 || missing[0] != "nope1" || missing[1] != "nope2" {
		t.Errorf("missing 应为 [nope1 nope2]（顺序+去重）: %v", missing)
	}
	// 全部命中 → 无缺失
	if m := missingFileTokens([]string{"boxA", "boxC"}, selected); len(m) != 0 {
		t.Errorf("全命中应无缺失: %v", m)
	}
	// wanted 为空（全下场景）→ 无缺失概念
	if m := missingFileTokens(nil, selected); m != nil {
		t.Errorf("空 wanted 应返回 nil: %v", m)
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

	// 省略 file-token → 应含 get_attachments 步骤 + download 带 extra
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
	if !strings.Contains(out, "extra") {
		t.Errorf("download 步骤应带 extra 参数: %s", out)
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

	// 指定单个 file-token → 仍应先 get_attachments（取 extra_info），与 lark-cli 行为一致
	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"file-token": "boxA", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/get_attachments") {
		t.Errorf("指定 file-token 时也应先 get_attachments 取 extra_info: %s", out)
	}
	if !strings.Contains(out, "extra") {
		t.Errorf("download 步骤应带 extra 参数: %s", out)
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
