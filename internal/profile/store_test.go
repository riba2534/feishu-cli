package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome 把 homeFunc 重定向到 t.TempDir()，并在测试结束时还原。
// 同时清空 FEISHU_PROFILE 环境变量，避免污染其它子测试。
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore := SetHomeFunc(func() (string, error) {
		return dir, nil
	})
	prevEnv := os.Getenv(EnvVar)
	_ = os.Unsetenv(EnvVar)
	t.Cleanup(func() {
		restore()
		if prevEnv != "" {
			_ = os.Setenv(EnvVar, prevEnv)
		}
	})
	return dir
}

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"work", false},
		{"work-1", false},
		{"work_personal", false},
		{"A_b-9", false},
		{"", true},
		{".", true},
		{"..", true},
		{"with space", true},
		{"with/slash", true},
		{"with.dot", true},
		{"profiles", true}, // 保留名
		{"cache", true},    // 保留名
		{strings.Repeat("a", 65), true},
	}
	for _, c := range cases {
		err := ValidateName(c.name)
		if c.wantErr && err == nil {
			t.Errorf("ValidateName(%q) want error, got nil", c.name)
		}
		if !c.wantErr && err != nil {
			t.Errorf("ValidateName(%q) want nil, got %v", c.name, err)
		}
	}
}

func TestRootDirAndProfileDir(t *testing.T) {
	home := withTempHome(t)

	root, err := RootDir()
	if err != nil {
		t.Fatalf("RootDir: %v", err)
	}
	wantRoot := filepath.Join(home, ".feishu-cli")
	if root != wantRoot {
		t.Errorf("RootDir = %q, want %q", root, wantRoot)
	}

	pd, err := ProfileDir("work")
	if err != nil {
		t.Fatalf("ProfileDir: %v", err)
	}
	want := filepath.Join(home, ".feishu-cli", "profiles", "work")
	if pd != want {
		t.Errorf("ProfileDir = %q, want %q", pd, want)
	}

	// 非法名应当报错
	if _, err := ProfileDir(".."); err == nil {
		t.Errorf("ProfileDir('..') want error, got nil")
	}
}

func TestHasProfilesEmpty(t *testing.T) {
	withTempHome(t)
	ok, err := HasProfiles()
	if err != nil {
		t.Fatalf("HasProfiles: %v", err)
	}
	if ok {
		t.Errorf("HasProfiles on empty home should be false")
	}
}

func TestCreateAndExists(t *testing.T) {
	withTempHome(t)

	if err := Create("work", CreateOpts{AppID: "cli_xxx", AppSecret: "secret"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ok, err := Exists("work")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Errorf("Exists('work') = false after Create")
	}

	cfgPath := filepath.Join(getProfileDirT(t, "work"), "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if !strings.Contains(string(data), `app_id: "cli_xxx"`) {
		t.Errorf("config.yaml missing app_id: %s", data)
	}
	if !strings.Contains(string(data), `app_secret: "secret"`) {
		t.Errorf("config.yaml missing app_secret: %s", data)
	}
	if !strings.Contains(string(data), `base_url: "https://open.feishu.cn"`) {
		t.Errorf("config.yaml missing default base_url: %s", data)
	}
}

func TestCreateDuplicateErrors(t *testing.T) {
	withTempHome(t)
	if err := Create("work", CreateOpts{}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := Create("work", CreateOpts{})
	if err == nil {
		t.Fatalf("second Create should error")
	}
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists wrapped", err)
	}
}

func TestCreateInvalidName(t *testing.T) {
	withTempHome(t)
	err := Create("bad name", CreateOpts{})
	if err == nil {
		t.Fatalf("Create('bad name') should error")
	}
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName wrapped", err)
	}
}

func TestListSorted(t *testing.T) {
	withTempHome(t)
	for _, name := range []string{"work", "personal", "test-1"} {
		if err := Create(name, CreateOpts{}); err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
	}
	got, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"personal", "test-1", "work"} // 字典序
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d, got=%v", len(got), len(want), got)
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("List[%d] = %q, want %q", i, got[i], n)
		}
	}
}

func TestListEmptyDir(t *testing.T) {
	withTempHome(t)
	got, err := List()
	if err != nil {
		t.Fatalf("List on empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List on empty home = %v, want []", got)
	}
}

func TestUseAndActiveName(t *testing.T) {
	withTempHome(t)
	if err := Create("work", CreateOpts{}); err != nil {
		t.Fatalf("Create work: %v", err)
	}
	if err := Create("personal", CreateOpts{}); err != nil {
		t.Fatalf("Create personal: %v", err)
	}

	// 未设置指针时 ActiveName 应当返回字典序第一个
	active, err := ActiveName()
	if err != nil {
		t.Fatalf("ActiveName: %v", err)
	}
	if active != "personal" {
		t.Errorf("default ActiveName = %q, want 'personal' (first by sort)", active)
	}

	if _, err := Use("work"); err != nil {
		t.Fatalf("Use work: %v", err)
	}
	active, err = ActiveName()
	if err != nil {
		t.Fatalf("ActiveName after Use: %v", err)
	}
	if active != "work" {
		t.Errorf("ActiveName after Use('work') = %q, want 'work'", active)
	}

	// 切换 personal，previous 应该 = work
	if _, err := Use("personal"); err != nil {
		t.Fatalf("Use personal: %v", err)
	}
	prev, err := ReadPrevious()
	if err != nil {
		t.Fatalf("ReadPrevious: %v", err)
	}
	if prev != "work" {
		t.Errorf("previous after switch = %q, want 'work'", prev)
	}

	// Use("-") 切回 work
	got, err := Use("-")
	if err != nil {
		t.Fatalf("Use('-'): %v", err)
	}
	if got != "work" {
		t.Errorf("Use('-') = %q, want 'work'", got)
	}
}

func TestUseNonExistent(t *testing.T) {
	withTempHome(t)
	_, err := Use("ghost")
	if err == nil {
		t.Fatalf("Use('ghost') should error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound wrapped", err)
	}
}

func TestUseDashWithoutPrevious(t *testing.T) {
	withTempHome(t)
	if err := Create("work", CreateOpts{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := Use("-")
	if err == nil {
		t.Fatalf("Use('-') without previous should error")
	}
	if !strings.Contains(err.Error(), "上一个") {
		t.Errorf("err msg = %q, want hint about previous profile", err.Error())
	}
}

func TestRemove(t *testing.T) {
	withTempHome(t)
	if err := Create("temp", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Remove("temp"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	ok, err := Exists("temp")
	if err != nil {
		t.Fatalf("Exists after Remove: %v", err)
	}
	if ok {
		t.Errorf("Exists('temp') = true after Remove")
	}
	// active 指针应被清空
	active, _ := ReadActive()
	if active != "" {
		t.Errorf("active = %q after removing active profile, want empty", active)
	}
}

func TestRemoveNonExistent(t *testing.T) {
	withTempHome(t)
	err := Remove("ghost")
	if err == nil {
		t.Fatalf("Remove('ghost') should error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound wrapped", err)
	}
}

func TestRename(t *testing.T) {
	withTempHome(t)
	if err := Create("old", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Rename("old", "new"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	oldOk, _ := Exists("old")
	newOk, _ := Exists("new")
	if oldOk {
		t.Errorf("old still exists")
	}
	if !newOk {
		t.Errorf("new doesn't exist")
	}
	active, _ := ReadActive()
	if active != "new" {
		t.Errorf("active = %q after rename, want 'new'", active)
	}
}

func TestRenameDuplicate(t *testing.T) {
	withTempHome(t)
	if err := Create("a", CreateOpts{}); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if err := Create("b", CreateOpts{}); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	err := Rename("a", "b")
	if err == nil {
		t.Fatalf("Rename a->b (b exists) should error")
	}
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists wrapped", err)
	}
}

func TestActiveDirFallback(t *testing.T) {
	home := withTempHome(t)
	// 未启用 profile 时 ActiveDir 应该返回旧布局 ~/.feishu-cli/
	dir, err := ActiveDir()
	if err != nil {
		t.Fatalf("ActiveDir on empty: %v", err)
	}
	want := filepath.Join(home, ".feishu-cli")
	if dir != want {
		t.Errorf("ActiveDir empty = %q, want %q (legacy layout)", dir, want)
	}

	// 启用 profile 后 ActiveDir 应该返回 profiles/<name>/
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dir, err = ActiveDir()
	if err != nil {
		t.Fatalf("ActiveDir: %v", err)
	}
	want = filepath.Join(home, ".feishu-cli", "profiles", "work")
	if dir != want {
		t.Errorf("ActiveDir = %q, want %q", dir, want)
	}
}

func TestEnvVarOverride(t *testing.T) {
	withTempHome(t)
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create work: %v", err)
	}
	if err := Create("personal", CreateOpts{}); err != nil {
		t.Fatalf("Create personal: %v", err)
	}

	t.Setenv(EnvVar, "personal")
	active, err := ActiveName()
	if err != nil {
		t.Fatalf("ActiveName: %v", err)
	}
	if active != "personal" {
		t.Errorf("ActiveName with env override = %q, want 'personal'", active)
	}

	t.Setenv(EnvVar, "ghost")
	_, err = ActiveName()
	if err == nil {
		t.Fatalf("ActiveName with FEISHU_PROFILE=ghost should error")
	}
}

func TestDescribe(t *testing.T) {
	withTempHome(t)
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create work: %v", err)
	}
	if err := Create("personal", CreateOpts{}); err != nil {
		t.Fatalf("Create personal: %v", err)
	}

	infos, err := Describe()
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("Describe len = %d, want 2", len(infos))
	}
	for _, info := range infos {
		if info.Name == "work" && !info.Active {
			t.Errorf("work should be active")
		}
		if info.Name == "personal" && info.Active {
			t.Errorf("personal should not be active")
		}
		if !info.HasConfig {
			t.Errorf("%s should have config.yaml", info.Name)
		}
		if info.HasToken {
			t.Errorf("%s should not have token.json (not logged in)", info.Name)
		}
	}
}

func TestMigrateLegacy(t *testing.T) {
	home := withTempHome(t)
	legacyDir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	legacyContent := "app_id: \"cli_legacy\"\napp_secret: \"old_secret\"\n"
	if err := os.WriteFile(filepath.Join(legacyDir, "config.yaml"), []byte(legacyContent), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	tokenContent := `{"access_token":"u-xxx","expires_at":0}`
	if err := os.WriteFile(filepath.Join(legacyDir, "token.json"), []byte(tokenContent), 0600); err != nil {
		t.Fatalf("write legacy token: %v", err)
	}

	target, err := MigrateLegacy(MigrateLegacyOpts{})
	if err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}
	if target != "default" {
		t.Errorf("target = %q, want 'default'", target)
	}

	gotCfg, err := os.ReadFile(filepath.Join(legacyDir, "profiles", "default", "config.yaml"))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if string(gotCfg) != legacyContent {
		t.Errorf("migrated config content mismatch:\n got=%q\nwant=%q", gotCfg, legacyContent)
	}

	gotTok, err := os.ReadFile(filepath.Join(legacyDir, "profiles", "default", "token.json"))
	if err != nil {
		t.Fatalf("read migrated token: %v", err)
	}
	if string(gotTok) != tokenContent {
		t.Errorf("migrated token content mismatch")
	}

	active, _ := ReadActive()
	if active != "default" {
		t.Errorf("active after migrate = %q, want 'default'", active)
	}

	// 第二次 migrate 应当报错（target 已存在）
	_, err = MigrateLegacy(MigrateLegacyOpts{})
	if err == nil {
		t.Fatalf("second MigrateLegacy should error")
	}

	// --force 覆盖
	_, err = MigrateLegacy(MigrateLegacyOpts{Force: true})
	if err != nil {
		t.Fatalf("MigrateLegacy --force: %v", err)
	}
}

func TestMigrateLegacyRejectsEmptyLegacyLayout(t *testing.T) {
	home := withTempHome(t)
	legacyDir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}

	_, err := MigrateLegacy(MigrateLegacyOpts{})
	if err == nil {
		t.Fatal("MigrateLegacy should reject an empty legacy layout")
	}
	if !strings.Contains(err.Error(), "未找到可迁移的旧布局文件") {
		t.Fatalf("error should explain missing legacy files, got: %v", err)
	}
	if fileExists(filepath.Join(legacyDir, "profiles", "default", "config.yaml")) {
		t.Fatal("empty migrate should not create a target profile config")
	}
	active, _ := ReadActive()
	if active != "" {
		t.Fatalf("empty migrate should not update active profile, got %q", active)
	}
}

func TestMigrateLegacyForceWithoutLegacyDoesNotClearExistingProfile(t *testing.T) {
	home := withTempHome(t)
	if err := Create("work", CreateOpts{
		AppID:     "cli_existing",
		AppSecret: "existing_secret",
		SwitchTo:  true,
	}); err != nil {
		t.Fatalf("Create work: %v", err)
	}

	_, err := MigrateLegacy(MigrateLegacyOpts{TargetName: "work", Force: true})
	if err == nil {
		t.Fatal("MigrateLegacy --force should reject when no legacy files exist")
	}

	cfgPath := filepath.Join(home, ".feishu-cli", "profiles", "work", "config.yaml")
	data, readErr := os.ReadFile(cfgPath)
	if readErr != nil {
		t.Fatalf("existing profile config should remain readable: %v", readErr)
	}
	if !strings.Contains(string(data), "cli_existing") {
		t.Fatalf("existing profile config was unexpectedly changed:\n%s", data)
	}
	active, _ := ReadActive()
	if active != "work" {
		t.Fatalf("active profile should remain work, got %q", active)
	}
}

func TestConfigFilePath(t *testing.T) {
	home := withTempHome(t)

	// 未启用 profile：回退旧布局
	got, err := ConfigFilePath()
	if err != nil {
		t.Fatalf("ConfigFilePath: %v", err)
	}
	want := filepath.Join(home, ".feishu-cli", "config.yaml")
	if got != want {
		t.Errorf("ConfigFilePath legacy = %q, want %q", got, want)
	}

	// 启用 profile
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err = ConfigFilePath()
	if err != nil {
		t.Fatalf("ConfigFilePath: %v", err)
	}
	want = filepath.Join(home, ".feishu-cli", "profiles", "work", "config.yaml")
	if got != want {
		t.Errorf("ConfigFilePath = %q, want %q", got, want)
	}
}

func TestTokenFilePath(t *testing.T) {
	home := withTempHome(t)
	if err := Create("work", CreateOpts{SwitchTo: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := TokenFilePath()
	if err != nil {
		t.Fatalf("TokenFilePath: %v", err)
	}
	want := filepath.Join(home, ".feishu-cli", "profiles", "work", "token.json")
	if got != want {
		t.Errorf("TokenFilePath = %q, want %q", got, want)
	}
}

// getProfileDirT 内部测试 helper：减少 boilerplate。
func getProfileDirT(t *testing.T, name string) string {
	t.Helper()
	dir, err := ProfileDir(name)
	if err != nil {
		t.Fatalf("ProfileDir(%q): %v", name, err)
	}
	return dir
}
