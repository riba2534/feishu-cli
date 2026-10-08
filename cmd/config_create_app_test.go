package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// create-app 保存凭证时 base_url 跟随签发品牌（Lark 租户写 open.larksuite.com），其余配置行保留。
func TestSaveAppConfig_UpdatesBaseURLForBrand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	dir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config.yaml")
	existing := "app_id: \"cli_old\"\napp_secret: \"old\"\nbase_url: \"https://open.feishu.cn\"\nowner_email: \"user@example.com\"\n"
	if err := os.WriteFile(cfgFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveAppConfig("cli_new", "sec_new", "https://open.larksuite.com"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(cfgFile)
	got := string(raw)
	for _, want := range []string{`app_id: "cli_new"`, `app_secret: "sec_new"`, `base_url: "https://open.larksuite.com"`, `owner_email: "user@example.com"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("配置应包含 %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "open.feishu.cn") {
		t.Fatalf("Lark 租户不应保留 open.feishu.cn:\n%s", got)
	}
	info, _ := os.Stat(cfgFile)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("权限应为 0600，得到 %o", info.Mode().Perm())
	}

	// 新建文件
	if err := os.Remove(cfgFile); err != nil {
		t.Fatal(err)
	}
	if err := saveAppConfig("cli_lark", "sec", "https://open.larksuite.com"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cfgFile)
	if !strings.Contains(string(raw), `base_url: "https://open.larksuite.com"`) {
		t.Fatalf("新建配置应写入 lark base_url:\n%s", raw)
	}
}

func TestConfigCreateApp_RejectsUnknownBrand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, err := runCLI(t, "config", "create-app", "--brand", "nope")
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("未知 --brand 应为用法错误: %v", err)
	}
}
