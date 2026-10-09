package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

func TestSheetImageWriteBatchRegistered(t *testing.T) {
	found := false
	for _, sub := range sheetImageCmd.Commands() {
		if firstWord(sub.Use) == "write-batch" {
			found = true
			if len(sub.Aliases) == 0 || sub.Aliases[0] != "batch-write" {
				t.Errorf("expected alias batch-write, got %#v", sub.Aliases)
			}
		}
	}
	if !found {
		t.Fatal("sheet image write-batch not registered")
	}

	if sheetImageWriteBatchCmd.Args == nil {
		t.Error("write-batch 应有参数校验")
	}
	if err := sheetImageWriteBatchCmd.Args(sheetImageWriteBatchCmd, []string{"token"}); err == nil {
		t.Error("write-batch 应拒绝 1 个参数")
	}
	if err := sheetImageWriteBatchCmd.Args(sheetImageWriteBatchCmd, []string{"token", "sheet1"}); err != nil {
		t.Errorf("write-batch 应接受 2 个参数: %v", err)
	}
}

func TestSheetImageWriteBatchFlags(t *testing.T) {
	for _, n := range []string{"manifest", "workers", "max-image-bytes", "allow-private-net", "output", "user-access-token"} {
		if sheetImageWriteBatchCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on write-batch", n)
		}
	}
}

func TestReadSheetImageBatchManifest_Sources(t *testing.T) {
	// 1. 文件读取
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")
	raw, _ := json.Marshal([]client.BatchWriteSheetImageItem{
		{Cell: "b2", URL: "https://example.com/1.jpg"},
		{Cell: "s1!C3", Path: "/tmp/2.png"},
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := readSheetImageBatchManifest(path, "s1", strings.NewReader(""))
	if err != nil {
		t.Fatalf("文件读取失败: %v", err)
	}
	if len(items) != 2 || items[0].Cell != "s1!B2:B2" || items[1].Cell != "s1!C3:C3" {
		t.Fatalf("unexpected items: %#v", items)
	}

	// 2. 行内 JSON 数组
	inline := `[{"cell":"D4","url":"https://example.com/4.png"}]`
	items, err = readSheetImageBatchManifest(inline, "s1", strings.NewReader(""))
	if err != nil {
		t.Fatalf("行内 JSON 解析失败: %v", err)
	}
	if len(items) != 1 || items[0].Cell != "s1!D4:D4" {
		t.Fatalf("unexpected items: %#v", items)
	}

	// 3. 标准输入 '-'
	stdinData := `[{"cell":"E5:E5","path":"/tmp/5.png"}]`
	items, err = readSheetImageBatchManifest("-", "s1", strings.NewReader(stdinData))
	if err != nil {
		t.Fatalf("标准输入解析失败: %v", err)
	}
	if len(items) != 1 || items[0].Cell != "s1!E5:E5" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestReadSheetImageBatchManifest_Validation(t *testing.T) {
	// 空输入
	if _, err := readSheetImageBatchManifest("", "s1", strings.NewReader("")); err == nil {
		t.Error("空 manifest 应报错")
	}

	// 非数组 JSON
	if _, err := readSheetImageBatchManifest(`{"cell":"A1"}`, "s1", strings.NewReader("")); err == nil {
		t.Error("非数组 JSON 应报错")
	}

	// 重复单元格
	dup := `[{"cell":"B2","url":"https://example.com/1.jpg"},{"cell":"s1!b2:b2","path":"/tmp/2.png"}]`
	if _, err := readSheetImageBatchManifest(dup, "s1", strings.NewReader("")); err == nil {
		t.Error("重复单元格应报错")
	}

	// 同时设置 url 和 path
	both := `[{"cell":"B2","url":"https://example.com/1.jpg","path":"/tmp/2.png"}]`
	if _, err := readSheetImageBatchManifest(both, "s1", strings.NewReader("")); err == nil {
		t.Error("同时设置 url/path 应报错")
	}

	// 既无 url 也无 path
	neither := `[{"cell":"B2"}]`
	if _, err := readSheetImageBatchManifest(neither, "s1", strings.NewReader("")); err == nil {
		t.Error("未设置 url/path 应报错")
	}
}

func TestNormalizeSheetImageBatchCell(t *testing.T) {
	tests := []struct {
		input   string
		sheetID string
		want    string
		wantErr bool
	}{
		{"B2", "0b12", "0b12!B2:B2", false},
		{"B2:B2", "0b12", "0b12!B2:B2", false},
		{"0b12!B2", "0b12", "0b12!B2:B2", false},
		{"0b12!B2:B2", "0b12", "0b12!B2:B2", false},
		{"b2", "0b12", "0b12!B2:B2", false},
		{"AA10", "0b12", "0b12!AA10:AA10", false},
		{"other!B2", "0b12", "", true},
		{"B2:C3", "0b12", "", true},
		{"invalid", "0b12", "", true},
		{"A0", "0b12", "", true},
	}

	for _, tt := range tests {
		got, err := normalizeSheetImageBatchCell(tt.input, tt.sheetID)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeSheetImageBatchCell(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeSheetImageBatchCell(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeSheetImageBatchCell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
