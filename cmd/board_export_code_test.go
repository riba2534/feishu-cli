package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestParseBoardExportNodesArray(t *testing.T) {
	nodes, err := parseBoardExportNodes(json.RawMessage(`[{"id":"n1","type":"svg"}]`))
	if err != nil {
		t.Fatalf("parseBoardExportNodes() error = %v", err)
	}
	if len(nodes) != 1 || nodes[0]["id"] != "n1" {
		t.Fatalf("nodes = %#v", nodes)
	}
}

func TestParseBoardExportNodesMap(t *testing.T) {
	nodes, err := parseBoardExportNodes(json.RawMessage(`{"n1":{"type":"svg"}}`))
	if err != nil {
		t.Fatalf("parseBoardExportNodes() error = %v", err)
	}
	if len(nodes) != 1 || nodes[0]["id"] != "n1" || nodes[0]["type"] != "svg" {
		t.Fatalf("nodes = %#v", nodes)
	}
}

// TestEnsureBoardExportWritableIsUsageError 输出文件已存在且未加 --overwrite 属于本地路径校验失败，
// 与 markdown fetch / doc media-download 一致返回用法错误（退出码 2），而不是一般错误（1）。
func TestEnsureBoardExportWritableIsUsageError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.svg")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := ensureBoardExportWritable(path, false)
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("已存在且未 --overwrite 应为用法错误: %v", err)
	}
	if err := ensureBoardExportWritable(path, true); err != nil {
		t.Fatalf("--overwrite 时应放行: %v", err)
	}
}
