package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

func withCmdHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore := profile.SetHomeFunc(func() (string, error) { return dir, nil })
	oldCfgFile := cfgFile
	t.Cleanup(func() {
		restore()
		_ = profile.SetCommandOverride("")
		config.SetBotFlagCredentials("", "")
		cfgFile = oldCfgFile
	})
	_ = profile.SetCommandOverride("")
	config.SetBotFlagCredentials("", "")
	cfgFile = ""
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	return dir
}

func TestBuildInventoryLegacyEnv(t *testing.T) {
	withCmdHome(t)
	t.Setenv("FEISHU_APP_ID", "cli_env_only")
	t.Setenv("FEISHU_APP_SECRET", "sec")
	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Mode != "legacy" {
		t.Errorf("mode=%s", inv.Mode)
	}
	if inv.Effective.AppID != "cli_env_only" {
		t.Errorf("effective app_id=%s", inv.Effective.AppID)
	}
	if !inv.EnvOverrides.AppID {
		t.Errorf("expected env override")
	}
	if inv.Hint == "" {
		t.Errorf("expected hint for no profiles")
	}
}

func TestBuildInventoryTwoProfilesAndFlag(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("alert", profile.CreateOpts{AppID: "cli_aaa", AppSecret: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("notify", profile.CreateOpts{AppID: "cli_bbb", AppSecret: "s2", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Active != "notify" || len(inv.Profiles) != 2 {
		t.Fatalf("active=%s n=%d", inv.Active, len(inv.Profiles))
	}
	if inv.Effective.AppID != "cli_bbb" {
		t.Errorf("effective=%s", inv.Effective.AppID)
	}
	if err := profile.SetCommandOverride("alert"); err != nil {
		t.Fatal(err)
	}
	inv, err = buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Active != "alert" || inv.ActiveSource != profile.SourceFlag {
		t.Errorf("flag override active=%s source=%s", inv.Active, inv.ActiveSource)
	}
	if !inv.FlagOverrides.Profile {
		t.Error("expected flag_overrides.profile=true")
	}
	if inv.TokenFrom != "alert" {
		t.Errorf("token_from_profile=%q, want alert", inv.TokenFrom)
	}
	if inv.Effective.AppID != "cli_aaa" {
		t.Errorf("flag effective=%s", inv.Effective.AppID)
	}
	t.Setenv("FEISHU_APP_ID", "cli_env")
	inv, err = buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Effective.AppID != "cli_env" {
		t.Errorf("env should win effective=%s", inv.Effective.AppID)
	}
	if !strings.Contains(inv.Hint, "覆盖所选 profile") || !strings.Contains(inv.Hint, "token.json") {
		t.Errorf("missing env/token-source hint: %q", inv.Hint)
	}
	raw, _ := json.Marshal(inv)
	if bytes.Contains(raw, []byte("s1")) || bytes.Contains(raw, []byte("s2")) {
		t.Errorf("secret leaked in JSON: %s", raw)
	}
}

func TestBuildInventoryBotFlagContext(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	config.SetBotFlagCredentials("cli_flag", "sec_flag")
	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	if inv.Effective.AppID != "cli_flag" || !inv.FlagOverrides.AppID || !inv.FlagOverrides.AppSecret {
		t.Fatalf("unexpected flag inventory: %+v", inv)
	}
	if inv.TokenFrom != "work" || !strings.Contains(inv.Hint, "token.json") {
		t.Fatalf("missing token source context: %+v", inv)
	}
	ctx := map[string]any{}
	attachProfileContext(ctx)
	if ctx["token_from_profile"] != "work" || ctx["hint"] == "" {
		t.Fatalf("attached context missing token source/hint: %+v", ctx)
	}
}

func TestGlobalBotCredentialFlagsDoNotCollide(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("bot-app-id") == nil || rootCmd.PersistentFlags().Lookup("bot-app-secret") == nil {
		t.Fatal("missing global --bot-app-id/--bot-app-secret")
	}
	if rootCmd.PersistentFlags().Lookup("app-id") != nil || rootCmd.PersistentFlags().Lookup("app-secret") != nil {
		t.Fatal("root must not register global --app-id/--app-secret")
	}
	if profileAddCmd.Flags().Lookup("app-id") == nil || profileAddCmd.Flags().Lookup("app-secret") == nil {
		t.Fatal("profile add must retain local --app-id/--app-secret")
	}
	if appsUpdateCmd.Flags().Lookup("app-id") == nil {
		t.Fatal("apps update must retain local Miaoda --app-id")
	}
}

func TestBotCredentialOverridesDoNotPersistIntoProfileAdd(t *testing.T) {
	withCmdHome(t)
	oldID, oldSecret, oldJSON := profileAddAppID, profileAddAppSecret, profileAddJSON
	oldOut := profileAddCmd.OutOrStdout()
	t.Cleanup(func() {
		profileAddAppID, profileAddAppSecret, profileAddJSON = oldID, oldSecret, oldJSON
		profileAddCmd.SetOut(oldOut)
	})
	profileAddAppID, profileAddAppSecret, profileAddJSON = "", "", false
	profileAddCmd.SetOut(&bytes.Buffer{})
	config.SetBotFlagCredentials("cli_transient", "sec_transient")

	if err := profileAddCmd.RunE(profileAddCmd, []string{"foo"}); err != nil {
		t.Fatal(err)
	}
	dir, err := profile.ProfileDir("foo")
	if err != nil {
		t.Fatal(err)
	}
	fields, err := profile.ReadConfigFields(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fields.AppID != "" || fields.AppSecret != "" {
		t.Fatalf("one-shot Bot credentials leaked into profile config: app_id=%q has_secret=%t", fields.AppID, fields.AppSecret != "")
	}
}

func TestAuthStatusAndLogoutLoadSelectedConfig(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{
		"auth status": authStatusCmd,
		"auth logout": authLogoutCmd,
	} {
		if shouldSkipConfigInit(cmd) {
			t.Errorf("%s must run config.Init for refresh/revoke credentials", name)
		}
	}
}

func TestProfileListJSONNoEmptyTrap(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: cli_legacy\napp_secret: sec\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inv, err := buildProfileInventory()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if strings.Contains(out, "尚未创建任何 profile") {
		t.Fatalf("empty trap: %s", out)
	}
	if inv.Effective.AppID != "cli_legacy" {
		t.Fatalf("missing effective app_id: %s", out)
	}
	if len(inv.Profiles) != 0 {
		t.Fatalf("legacy should have empty profiles: %s", out)
	}
}
