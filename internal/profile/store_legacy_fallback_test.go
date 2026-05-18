package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestActiveNameLegacyFallback 验证 legacy 文件存在时，profile 没有 active pointer
// 应优先 legacy（返回 ""），而不是字典序第一个 profile（codex review P2 修复）。
func TestActiveNameLegacyFallback(t *testing.T) {
	tmp := t.TempDir()
	restore := SetHomeFunc(func() (string, error) { return tmp, nil })
	t.Cleanup(restore)
	root := filepath.Join(tmp, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// 创建一个 profile（profiles/work/）但不设 active pointer
	if err := Create("work", CreateOpts{AppID: "cli_xxx"}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}

	// case 1: 无 legacy 文件 → fallback 字典序第一个
	got, err := ActiveName()
	if err != nil {
		t.Fatalf("ActiveName: %v", err)
	}
	if got != "work" {
		t.Errorf("无 legacy 时 ActiveName = %q, want %q", got, "work")
	}

	// case 2: 创建 legacy config.yaml → 应优先返回 ""（caller fallback 旧布局）
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: cli_legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = ActiveName()
	if err != nil {
		t.Fatalf("ActiveName with legacy: %v", err)
	}
	if got != "" {
		t.Errorf("有 legacy config.yaml 时 ActiveName = %q, want \"\" (优先 legacy)", got)
	}
}

// TestActiveNameLegacyTokenAlsoTriggers 验证只有 token.json 也算 legacy
func TestActiveNameLegacyTokenAlsoTriggers(t *testing.T) {
	tmp := t.TempDir()
	restore := SetHomeFunc(func() (string, error) { return tmp, nil })
	t.Cleanup(restore)
	root := filepath.Join(tmp, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Create("p1", CreateOpts{AppID: "cli_xxx"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "token.json"), []byte(`{"access_token":"u-xxx"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ActiveName()
	if err != nil || got != "" {
		t.Errorf("只有 legacy token.json 时 ActiveName = %q (err=%v), want \"\" nil", got, err)
	}
}
