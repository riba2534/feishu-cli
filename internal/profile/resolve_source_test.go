package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLegacyFile(t *testing.T, home, name string) {
	t.Helper()
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("app_id: cli_legacy\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

// active_source 是写进 Skill 文档的 Agent 契约字段，六个取值都必须被钉死。
func TestResolveActiveSources(t *testing.T) {
	t.Run("none：空目录", func(t *testing.T) {
		withTempHome(t)
		name, source, err := ResolveActive()
		if err != nil || name != "" || source != SourceNone {
			t.Errorf("= %q/%q/%v, want \"\"/%s/nil", name, source, err, SourceNone)
		}
	})

	t.Run("legacy：无 profiles 但有旧布局文件", func(t *testing.T) {
		home := withTempHome(t)
		writeLegacyFile(t, home, "config.yaml")
		name, source, err := ResolveActive()
		if err != nil || name != "" || source != SourceLegacy {
			t.Errorf("= %q/%q/%v, want \"\"/%s/nil", name, source, err, SourceLegacy)
		}
	})

	t.Run("legacy：有 profiles、无指针、旧布局仍在（保护 zero-touch 升级）", func(t *testing.T) {
		home := withTempHome(t)
		if err := Create("work", CreateOpts{AppID: "cli_work"}); err != nil {
			t.Fatal(err)
		}
		writeLegacyFile(t, home, "token.json")
		name, source, err := ResolveActive()
		if err != nil || name != "" || source != SourceLegacy {
			t.Errorf("= %q/%q/%v, want \"\"/%s/nil", name, source, err, SourceLegacy)
		}
	})

	t.Run("fallback：无指针、无旧布局，回退字典序第一个", func(t *testing.T) {
		withTempHome(t)
		if err := Create("zeta", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		if err := Create("alpha", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		name, source, err := ResolveActive()
		if err != nil || name != "alpha" || source != SourceFallback {
			t.Errorf("= %q/%q/%v, want alpha/%s/nil", name, source, err, SourceFallback)
		}
	})

	t.Run("fallback：指针指向已删除的 profile", func(t *testing.T) {
		withTempHome(t)
		if err := Create("gone", CreateOpts{SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		if err := Create("kept", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		dir, err := ProfileDir("gone")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		name, source, err := ResolveActive()
		if err != nil || name != "kept" || source != SourceFallback {
			t.Errorf("= %q/%q/%v, want kept/%s/nil", name, source, err, SourceFallback)
		}
	})

	t.Run("legacy：指针失效但旧布局还在（与指针缺失同语义）", func(t *testing.T) {
		home := withTempHome(t)
		if err := Create("gone", CreateOpts{SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		if err := Create("kept", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		dir, err := ProfileDir("gone")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		writeLegacyFile(t, home, "config.yaml")
		name, source, err := ResolveActive()
		if err != nil || name != "" || source != SourceLegacy {
			t.Errorf("= %q/%q/%v, want \"\"/%s/nil（不能把老用户静默切到别的 Bot）", name, source, err, SourceLegacy)
		}
	})

	t.Run("pointer：active-profile 指针", func(t *testing.T) {
		withTempHome(t)
		if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		name, source, err := ResolveActive()
		if err != nil || name != "work" || source != SourcePointer {
			t.Errorf("= %q/%q/%v, want work/%s/nil", name, source, err, SourcePointer)
		}
	})

	t.Run("env：FEISHU_PROFILE", func(t *testing.T) {
		withTempHome(t)
		if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		if err := Create("personal", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvVar, "personal")
		name, source, err := ResolveActive()
		if err != nil || name != "personal" || source != SourceEnv {
			t.Errorf("= %q/%q/%v, want personal/%s/nil", name, source, err, SourceEnv)
		}
	})

	t.Run("flag 压过 env", func(t *testing.T) {
		withTempHome(t)
		t.Cleanup(func() { _ = SetCommandOverride("") })
		if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		if err := Create("personal", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvVar, "personal")
		if err := SetCommandOverride("work"); err != nil {
			t.Fatal(err)
		}
		name, source, err := ResolveActive()
		if err != nil || name != "work" || source != SourceFlag {
			t.Errorf("= %q/%q/%v, want work/%s/nil", name, source, err, SourceFlag)
		}
	})
}
