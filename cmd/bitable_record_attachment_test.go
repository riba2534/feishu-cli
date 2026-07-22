package cmd

import (
	"os"
	"path/filepath"
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

// newDownloadAttachmentTestCmd 镜像真实 download-attachment 的 flag 注册（见
// cmd/bitable_record_attachment.go 的 init）：刻意【不】注册 --format/--jq（真实命令不注册它们），
// --output 是「附件下载保存路径」语义。之前测试误用 addBitableWriteFlags 注册了 --format/--jq、
// 与真实命令不一致，且从不传 --output 路径，掩盖了 dry-run 把预览写进 --output 的回归。
func newDownloadAttachmentTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "download-attachment", RunE: bitableRecordDownloadAttachmentCmd.RunE}
	addBaseTokenFlag(cmd)
	cmd.Flags().String("user-access-token", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("table-id", "", "")
	cmd.Flags().String("record-id", "", "")
	cmd.Flags().StringArray("file-token", nil, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("overwrite", false, "")
	return cmd
}

func TestRecordDownloadAttachmentDryRunAll(t *testing.T) {
	initTestConfig(t)
	cmd := newDownloadAttachmentTestCmd()

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
	cmd := newDownloadAttachmentTestCmd()

	// 指定单个 file-token → 仍应先 get_attachments（取 extra_info）
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

// TestRecordDownloadAttachmentDryRunOutputNotConsumed 回归防护：download-attachment 的 --output
// 是「下载保存路径」语义，dry-run 预览必须打到 stdout，绝不能被当成结果输出文件写进该路径
// （否则会覆盖用户的下载目标文件 / 在 --output 是目录时报错）。
func TestRecordDownloadAttachmentDryRunOutputNotConsumed(t *testing.T) {
	initTestConfig(t)

	// 场景 A：--output 是文件 → dry-run 不能覆写它，预览走 stdout
	dir := t.TempDir()
	outFile := filepath.Join(dir, "target.pdf")
	if err := os.WriteFile(outFile, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDownloadAttachmentTestCmd()
	out, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"file-token": "boxA", "output": outFile, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("传 --output 文件时 dry-run 不应报错: %v", err)
	}
	if !strings.Contains(out, "get_attachments") {
		t.Errorf("dry-run 预览应打到 stdout（而非写进 --output 文件）: %q", out)
	}
	if b, _ := os.ReadFile(outFile); string(b) != "ORIGINAL" {
		t.Errorf("dry-run 不应改写 --output 下载目标文件，实际内容: %q", string(b))
	}

	// 场景 B：--output 是已存在目录（省略 file-token 下载全部到目录的合法用法）→ dry-run 不应报 is a directory
	cmd2 := newDownloadAttachmentTestCmd()
	out2, err := captureRunE(t, cmd2, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "record-id": "rec1",
		"output": dir, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("--output 为目录时 dry-run 不应报错（曾因 OutputFile 被当结果文件而 is a directory）: %v", err)
	}
	if !strings.Contains(out2, "get_attachments") {
		t.Errorf("dry-run 预览应打到 stdout: %q", out2)
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
