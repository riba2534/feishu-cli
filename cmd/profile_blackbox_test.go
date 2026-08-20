package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// runCLI 走真实 rootCmd 执行一条命令，同时捕获 stdout 与 stderr。
//
// 必须捕获 stderr：身份 warning 全部写在那里，只看 stdout 的 harness 会让
// 「两条 warning 自相矛盾」这类问题在全绿测试下存活。
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	// rootCmd 是包级单例：全局 flag 变量和各子命令的 flag（如 profileListJSON）
	// 都会跨用例残留，必须把整棵命令树复位，否则上一条 --json 会让下一条也输出 JSON。
	profileFlag, botAppIDFlag, botAppSecretFlag, cfgFile = "", "", "", ""
	resetCommandTreeFlags(rootCmd)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		profileFlag, botAppIDFlag, botAppSecretFlag, cfgFile = "", "", "", ""
		_ = profile.SetCommandOverride("")
	})

	// printJSON 与 warning 分别直写 os.Stdout / os.Stderr，SetOut 捕获不到
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	errR, errW, perr := os.Pipe()
	if perr != nil {
		t.Fatal(perr)
	}
	os.Stdout, os.Stderr = outW, errW
	// defer 恢复：即使被测路径 panic 也不污染后续用例
	defer func() {
		os.Stdout, os.Stderr = origOut, origErr
	}()

	err = rootCmd.Execute()
	_ = outW.Close()
	_ = errW.Close()
	ob, _ := io.ReadAll(outR)
	eb, _ := io.ReadAll(errR)
	return string(ob), string(eb), err
}

// resetCommandTreeFlags 把整棵命令树的 flag 复位到默认值并清除 Changed 标记。
func resetCommandTreeFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetCommandTreeFlags(sub)
	}
}

func decodeInventory(t *testing.T, out string) profileInventory {
	t.Helper()
	var inv profileInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("解析 inventory 失败: %v\n原始输出: %s", err, out)
	}
	return inv
}

// 三个全局 flag 必须经由 PersistentPreRunE 真正生效，而不只是注册成功。
func TestCLIGlobalFlagsPropagate(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("alpha", profile.CreateOpts{AppID: "cli_alpha", AppSecret: "sa"}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("beta", profile.CreateOpts{AppID: "cli_beta", AppSecret: "sb", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if inv := decodeInventory(t, out); inv.Active != "beta" || inv.ActiveSource != profile.SourcePointer {
		t.Errorf("默认应走指针: active=%s source=%s", inv.Active, inv.ActiveSource)
	}

	out, _, err = runCLI(t, "--profile", "alpha", "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.Active != "alpha" || inv.ActiveSource != profile.SourceFlag {
		t.Errorf("--profile 未经命令入口生效: active=%s source=%s", inv.Active, inv.ActiveSource)
	}
	if inv.Effective.AppID != "cli_alpha" {
		t.Errorf("effective 未跟随 --profile: %s", inv.Effective.AppID)
	}

	out, _, err = runCLI(t, "--bot-app-id", "cli_oneshot", "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv = decodeInventory(t, out)
	if inv.Effective.AppID != "cli_oneshot" || !inv.FlagOverrides.AppID {
		t.Errorf("--bot-app-id 未经命令入口生效: %+v", inv.Effective)
	}
	if inv.TokenFrom != "beta" {
		t.Errorf("App 凭证覆盖不应改变 token 归属: %s", inv.TokenFrom)
	}

	// --bot-app-secret 单独传：app_id 仍走 profile，has_secret 由 flag 提供
	out, _, err = runCLI(t, "--bot-app-secret", "sec_oneshot", "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv = decodeInventory(t, out)
	if !inv.FlagOverrides.AppSecret || inv.FlagOverrides.AppID {
		t.Errorf("--bot-app-secret 未经命令入口生效: %+v", inv.FlagOverrides)
	}
	if inv.Effective.AppID != "cli_beta" || !inv.Effective.HasSecret {
		t.Errorf("只传 secret 时 app_id 应仍来自 profile: %+v", inv.Effective)
	}
	if strings.Contains(out, "sec_oneshot") {
		t.Error("app_secret 明文泄漏进 JSON")
	}
}

// 一个损坏的 inactive profile 不能让整张清单打不开。
func TestCLIBrokenInactiveProfileDoesNotBreakList(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("good", profile.CreateOpts{AppID: "cli_good", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("broken", profile.CreateOpts{AppID: "cli_b", AppSecret: "s"}); err != nil {
		t.Fatal(err)
	}
	dir, err := profile.ProfileDir("broken")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("app_id: x\n  bad: [unclosed\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatalf("损坏的 inactive profile 不应让命令失败: %v", err)
	}
	inv := decodeInventory(t, out)
	if len(inv.Profiles) != 2 {
		t.Fatalf("两条都应保留: %+v", inv.Profiles)
	}
	var brokenSnap profileSnapshot
	for _, p := range inv.Profiles {
		if p.Name == "broken" {
			brokenSnap = p
		}
	}
	if brokenSnap.ConfigError == "" {
		t.Error("损坏条目必须带 config_error 说明，不能静默吞掉")
	}
	if brokenSnap.problems() == "" {
		t.Error("problems() 应汇总出该条目的错误")
	}
	if inv.Effective.AppID != "cli_good" {
		t.Errorf("active profile 不受影响: %s", inv.Effective.AppID)
	}
}

// legacy 与 profiles 共存、无指针时，生效的那套必须出现在 effective 里。
func TestCLILegacyCoexistsWithProfiles(t *testing.T) {
	home := withCmdHome(t)
	if err := profile.Create("spare", profile.CreateOpts{AppID: "cli_spare", AppSecret: "s"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".feishu-cli")
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: cli_legacy\napp_secret: s\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.ActiveSource != profile.SourceLegacy {
		t.Fatalf("应优先旧布局: source=%s", inv.ActiveSource)
	}
	if inv.Effective.AppID != "cli_legacy" {
		t.Errorf("effective 应是旧布局那套: %s", inv.Effective.AppID)
	}
	if len(inv.Profiles) != 1 || inv.Profiles[0].AppID != "cli_spare" {
		t.Errorf("profiles[] 仍应列出磁盘上的 profile: %+v", inv.Profiles)
	}
}

// active 字段必须始终存在（基线是 bool，omitempty 会让 inactive 条目丢字段）。
func TestCLIInventoryJSONShapeStable(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("one", profile.CreateOpts{AppID: "cli_one", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("two", profile.CreateOpts{AppID: "cli_two", AppSecret: "s"}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	for _, p := range raw.Profiles {
		for _, key := range []string{"name", "active", "has_config", "has_token"} {
			if _, ok := p[key]; !ok {
				t.Errorf("profile %v 缺少契约字段 %q", p["name"], key)
			}
		}
	}
	for _, key := range []string{`"user_token_override"`, `"has_config_user_token"`} {
		if !strings.Contains(out, key) {
			t.Errorf("缺少契约字段 %s", key)
		}
	}
}

// 显式 User Token 存在时，不能把 profile 目录说成实际来源。
func TestCLIUserTokenSourceReflectsEnv(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeInventory(t, out).UserTokenOverride; got != "" {
		t.Errorf("没有显式设置时不应报告任何 override，得到 %q", got)
	}

	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	t.Setenv("FEISHU_APP_ID", "cli_other")
	t.Setenv("FEISHU_APP_SECRET", "sec_other")
	out, _, err = runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.UserTokenOverride != "FEISHU_USER_ACCESS_TOKEN" {
		t.Errorf("user_token_override = %q, want FEISHU_USER_ACCESS_TOKEN", inv.UserTokenOverride)
	}
	if !strings.Contains(inv.Hint, "FEISHU_USER_ACCESS_TOKEN") {
		t.Errorf("hint 应点名覆盖来源: %q", inv.Hint)
	}
	// 措辞必须限定在「使用 User Token 的命令」，不能断言本次命令的实际身份
	if strings.Contains(inv.Hint, "本次 User Token 来自") {
		t.Errorf("不得断言本次命令的实际 token 来源: %q", inv.Hint)
	}
}

// config.yaml 坏掉时 auth status 必须还能跑——它正是用来排查这种情况的。
func TestCLIAuthStatusSurvivesMalformedConfig(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: x\n  bad: [unclosed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token := `{"access_token":"u-local","expires_at":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(root, "token.json"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "auth", "status", "-o", "json")
	if err != nil {
		t.Fatalf("配置损坏不应堵死 auth status: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n%s", err, out)
	}
	if got["logged_in"] != true {
		t.Errorf("本地 token 有效，应报告已登录: %v", got["logged_in"])
	}
	if _, ok := got["profile_source"]; !ok {
		t.Error("降级路径仍须保留身份线索 profile_source")
	}

	// 对照：业务命令必须继续 fail-fast，不能被降级路径带跑偏
	if _, _, err := runCLI(t, "chat", "list"); err == nil {
		t.Error("业务命令遇到损坏配置应直接报错")
	}
}

// 显式空 flag 不构成覆盖：resolver 会跳过空值继续走 env/file。
func TestCLIUserTokenEmptyFlagIsNotOverride(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env")

	// api 命令带 local 的 --user-access-token；--dry-run 不发请求
	if _, _, err := runCLI(t, "api", "GET", "/open-apis/im/v1/chats", "--user-access-token", "", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if got := userTokenOverride(); got != "FEISHU_USER_ACCESS_TOKEN" {
		t.Errorf("空 flag 不该被当成覆盖来源，应仍是环境变量，得到 %q", got)
	}

	// 非空 flag 才构成覆盖
	if _, _, err := runCLI(t, "api", "GET", "/open-apis/im/v1/chats", "--user-access-token", "u-flag", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if got := userTokenOverride(); got != "--user-access-token" {
		t.Errorf("非空 flag 应构成覆盖，得到 %q", got)
	}
}

// config.yaml 里的静态 user_access_token 是解析链最后一级，必须能被观察到且不泄漏明文。
func TestCLIConfigUserTokenIsReportedWithoutLeaking(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	body := "app_id: cli_legacy\napp_secret: s\nuser_access_token: u-static-secret\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if !inv.HasConfigUserToken {
		t.Error("config.yaml 有静态 user_access_token，应报告 has_config_user_token=true")
	}
	if strings.Contains(out, "u-static-secret") {
		t.Error("静态 user_access_token 明文泄漏进 JSON")
	}
}

// 当前生效那套的损坏原因必须能从 stdout 观察到，不能只留在 stderr。
func TestCLIEffectiveErrorReachesStdout(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: x\n  bad: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "token.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.EffectiveError == "" {
		t.Fatal("生效那套坏了，effective_error 不能为空")
	}
	if inv.Effective.ConfigError == "" || inv.Effective.TokenError == "" {
		t.Errorf("config 与 token 的错误应分别记录: %+v", inv.Effective)
	}
}

// --config 只替换 config 那一层，token/cache 的错误必须保留。
func TestCLIConfigFlagKeepsTokenErrors(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: x\n  bad: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "token.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_alt\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "--config", alt, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.Effective.ConfigError != "" {
		t.Errorf("--config 提供了有效配置，config 层错误应被替换掉: %q", inv.Effective.ConfigError)
	}
	if inv.Effective.AppID != "cli_alt" {
		t.Errorf("effective 应用 --config 的 app_id: %q", inv.Effective.AppID)
	}
	if inv.Effective.TokenError == "" {
		t.Error("--config 不替换 token.json，它的解析错误必须保留")
	}
}

// 人类可读输出也要验证，这轮修的正是表格与提示行。
func TestCLIHumanOutputShowsLegacyAndBrokenEntries(t *testing.T) {
	home := withCmdHome(t)
	if err := profile.Create("spare", profile.CreateOpts{AppID: "cli_spare", AppSecret: "s"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".feishu-cli")
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("app_id: cli_legacy\napp_secret: s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := profile.ProfileDir("spare")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "(legacy)") || !strings.Contains(out, "cli_legacy") {
		t.Errorf("生效的旧布局必须出现在表格里:\n%s", out)
	}
	if !strings.Contains(out, "* ") {
		t.Errorf("生效行必须有 * 标记:\n%s", out)
	}
	if !strings.Contains(out, "spare") {
		t.Errorf("磁盘上的 profile 仍应列出:\n%s", out)
	}
	if !strings.Contains(out, "token.json 解析失败") {
		t.Errorf("损坏条目必须被点名:\n%s", out)
	}
}

// 身份 warning 写在 stderr。这组断言正是前几轮「测试全绿但文案自相矛盾」的补漏。
func TestCLIWarningsAreConsistentOnStderr(t *testing.T) {
	home := withCmdHome(t)
	if err := profile.Create("beta", profile.CreateOpts{AppID: "cli_beta", AppSecret: "sb", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_alt\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = home

	t.Setenv("FEISHU_APP_ID", "cli_env")
	t.Setenv("FEISHU_APP_SECRET", "se")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env")

	_, stderr, err := runCLI(t, "--config", alt, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}

	// 旧 --config warning 不得声称 token 实际读自 profile——那与显式 token 优先的说明冲突
	if strings.Contains(stderr, "token 仍读自当前 profile") {
		t.Errorf("--config warning 与显式 User Token 说明自相矛盾:\n%s", stderr)
	}
	// 不得断言本次命令的实际身份（本项目按命令分四类 token helper，root 层无权断言）
	for _, forbidden := range []string{"本次 User Token 来自", "本次实际"} {
		if strings.Contains(stderr, forbidden) {
			t.Errorf("stderr 不得断言本次命令实际身份（含 %q）:\n%s", forbidden, stderr)
		}
	}
	// token.json 仍可能被 refreshIfStaleLocalToken 读取，不能说"不读"
	if strings.Contains(stderr, "不读 profile") || strings.Contains(stderr, "不读 旧布局") {
		t.Errorf("token.json 仍可能被读取用于自动刷新，措辞不应是「不读」:\n%s", stderr)
	}
	if !strings.Contains(stderr, "FEISHU_USER_ACCESS_TOKEN") {
		t.Errorf("应说明存在 User Token override:\n%s", stderr)
	}
}

// 单应用推荐用法必须完全静默——这是收窄告警的初衷。
func TestCLINoWarningOnSingleAppSetup(t *testing.T) {
	home := withCmdHome(t)
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_APP_ID", "cli_only")
	t.Setenv("FEISHU_APP_SECRET", "s")

	_, stderr, err := runCLI(t, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "⚠") {
		t.Errorf("单应用 env 配置不该产生任何告警:\n%s", stderr)
	}
}

// profile use 的告警必须指向切换后的目标，且不被本次 --profile / --config 带偏。
func TestCLIProfileUseWarningTargetsNewProfile(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("alpha", profile.CreateOpts{AppID: "cli_alpha", AppSecret: "sa", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("beta", profile.CreateOpts{AppID: "cli_beta", AppSecret: "sb"}); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_alt\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_APP_ID", "cli_env")
	t.Setenv("FEISHU_APP_SECRET", "se")

	_, stderr, err := runCLI(t, "--profile", "alpha", "--config", alt, "profile", "use", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, `profile "beta"`) {
		t.Errorf("告警应指向切换后的 beta:\n%s", stderr)
	}
	if !strings.Contains(stderr, "cli_beta") {
		t.Errorf("应以 beta 自己的 config.yaml 为基准，不被 --config 劫持:\n%s", stderr)
	}
	if strings.Contains(stderr, "cli_alt") {
		t.Errorf("profile use 的目标检查不应读 --config 的文件:\n%s", stderr)
	}
	if strings.Contains(stderr, `profile "alpha"`) {
		t.Errorf("不应混入 --profile 指向的 alpha:\n%s", stderr)
	}
}

// --config 指向不存在的文件：诊断命令必须说出来，业务命令必须失败。
func TestCLIMissingConfigFileIsReported(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "nope.yaml")

	out, _, err := runCLI(t, "--config", missing, "profile", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	inv := decodeInventory(t, out)
	if inv.EffectiveError == "" || !strings.Contains(inv.EffectiveError, "不存在") {
		t.Errorf("显式 --config 文件缺失必须报告: %q", inv.EffectiveError)
	}

	if _, _, err := runCLI(t, "--config", missing, "chat", "list"); err == nil {
		t.Error("业务命令遇到不存在的 --config 应直接失败")
	}
}

// profile current 的人类分支同样要暴露当前这套的问题。
func TestCLIProfileCurrentHumanShowsError(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("only", profile.CreateOpts{AppID: "cli_only", AppSecret: "s", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	dir, err := profile.ProfileDir("only")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("app_id: x\n  bad: [\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "profile", "current")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "解析失败") {
		t.Errorf("profile current 人类输出必须提示配置损坏:\n%s", out)
	}
}

// --config 生效时人类表格的 * 行是磁盘 YAML，必须补一行点明真正生效的 app_id。
func TestCLIListShowsEffectiveAppIDUnderConfigFlag(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("alpha", profile.CreateOpts{AppID: "cli_alpha", AppSecret: "sa", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(t.TempDir(), "alt.yaml")
	if err := os.WriteFile(alt, []byte("app_id: cli_explicit\napp_secret: sa\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "--config", alt, "profile", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cli_explicit") {
		t.Errorf("必须点明本次生效的 app_id，否则 * 行会被误读:\n%s", out)
	}
	if !strings.Contains(out, "--config") {
		t.Errorf("应说明生效凭证来自 --config:\n%s", out)
	}
}
