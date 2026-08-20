package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/profile"
)

// snapshotFromDir 承接了原 profile.Describe 的职责（active 标记 + 文件存在性），
// 并额外给出 app_id / token 状态 / 登录用户。
func TestSnapshotFromDirReportsFiles(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("personal", profile.CreateOpts{AppID: "cli_personal", AppSecret: "sec2"}); err != nil {
		t.Fatal(err)
	}
	dir, err := profile.ProfileDir("work")
	if err != nil {
		t.Fatal(err)
	}
	token := `{"access_token":"u-x","expires_at":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	cache := `{"open_id":"ou_x","name":"张三","cached_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "user_profile.json"), []byte(cache), 0600); err != nil {
		t.Fatal(err)
	}

	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]profileSnapshot{}
	for _, snap := range inv.Profiles {
		byName[snap.Name] = snap
	}
	if len(byName) != 2 {
		t.Fatalf("profiles = %d, want 2", len(byName))
	}

	work := byName["work"]
	if !work.Active {
		t.Error("work 应标记为 active")
	}
	if !work.HasConfig || !work.HasSecret || !work.HasToken {
		t.Errorf("work 文件状态不对: %+v", work)
	}
	if work.TokenStatus == "" || work.UserName != "张三" || work.UserOpenID != "ou_x" {
		t.Errorf("work token/用户信息不对: %+v", work)
	}
	if work.SelectWith != "--profile work" {
		t.Errorf("select_with = %q", work.SelectWith)
	}

	personal := byName["personal"]
	if personal.Active {
		t.Error("personal 不应标记为 active")
	}
	if !personal.HasConfig || personal.HasToken {
		t.Errorf("personal 未登录，不该有 token: %+v", personal)
	}
	if personal.TokenStatus != "" || personal.UserName != "" {
		t.Errorf("personal 不该有 token 状态/用户名: %+v", personal)
	}
}

// --config 替换的是「选中目录的 config.yaml」这一层，effective 必须跟着走，
// 否则 auth status 报的 app_id 会和 doctor / 真实请求用的对不上。
func TestInventoryEffectiveFollowsConfigFlag(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sw", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_alt\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfgFile = alt

	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Effective.AppID != "cli_alt" {
		t.Errorf("effective.app_id = %q, want cli_alt（--config 指定的那份）", inv.Effective.AppID)
	}
	if !inv.Effective.HasSecret {
		t.Error("--config 里有 app_secret，effective.has_secret 应为 true")
	}
	if len(inv.Profiles) != 1 || inv.Profiles[0].AppID != "cli_work" {
		t.Errorf("profiles[] 应保持磁盘原值，不受 --config 影响: %+v", inv.Profiles)
	}
	// token 归属不受 --config 影响
	if inv.TokenFrom != "work" {
		t.Errorf("token_from_profile = %q, want work", inv.TokenFrom)
	}
}

// --config 下的告警要点名实际读的那个文件，而不是笼统说 config.yaml。
func TestAppCredentialWarningNamesConfigFlagFile(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sw", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_alt\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfgFile = alt
	t.Setenv("FEISHU_APP_ID", "cli_env")
	t.Setenv("FEISHU_APP_SECRET", "se")

	got := appCredentialWarning()
	if !strings.Contains(got, "--config") || !strings.Contains(got, "cli_alt") {
		t.Errorf("告警应点名 --config 文件及其 app_id: %q", got)
	}
	if strings.Contains(got, "cli_work") {
		t.Errorf("--config 生效时不该拿 profile 目录的 app_id 作基准: %q", got)
	}
}
