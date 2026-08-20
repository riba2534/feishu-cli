package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
)

func writeLegacyConfig(t *testing.T, home, appID, appSecret string) {
	t.Helper()
	root := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	if appID != "" {
		body.WriteString("app_id: " + appID + "\n")
	}
	if appSecret != "" {
		body.WriteString("app_secret: " + appSecret + "\n")
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

// 单应用是文档推荐用法，不该每条命令刷一行警告。
func TestAppCredentialWarningSilentOnSingleApp(t *testing.T) {
	t.Run("旧布局无配置文件，仅环境变量", func(t *testing.T) {
		withCmdHome(t)
		t.Setenv("FEISHU_APP_ID", "cli_env")
		t.Setenv("FEISHU_APP_SECRET", "sec_env")
		if got := appCredentialWarning(); got != "" {
			t.Errorf("单应用 env 不该告警，得到: %s", got)
		}
	})

	t.Run("环境变量与旧布局配置同一个应用", func(t *testing.T) {
		home := withCmdHome(t)
		writeLegacyConfig(t, home, "cli_same", "sec_file")
		t.Setenv("FEISHU_APP_ID", "cli_same")
		t.Setenv("FEISHU_APP_SECRET", "sec_env")
		if got := appCredentialWarning(); got != "" {
			t.Errorf("同一应用不该告警，得到: %s", got)
		}
	})

	t.Run("只 export app_id 且与配置文件同值，secret 走文件", func(t *testing.T) {
		home := withCmdHome(t)
		writeLegacyConfig(t, home, "cli_same", "sec_file")
		t.Setenv("FEISHU_APP_ID", "cli_same")
		if got := appCredentialWarning(); got != "" {
			t.Errorf("app_id 同值时 secret 走文件是安全的，得到: %s", got)
		}
	})

	t.Run("--bot-app-id 与所选 profile 同值", func(t *testing.T) {
		withCmdHome(t)
		if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		config.SetBotFlagCredentials("cli_work", "")
		if got := appCredentialWarning(); got != "" {
			t.Errorf("flag 与 profile 同一应用不该告警，得到: %s", got)
		}
	})

	t.Run("完全没有覆盖", func(t *testing.T) {
		withCmdHome(t)
		if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		if got := appCredentialWarning(); got != "" {
			t.Errorf("无覆盖不该告警，得到: %s", got)
		}
	})
}

// 覆盖来的 app_id 与所选目录不是同一个应用时，User Token 归属必须说清楚。
func TestAppCredentialWarningOnMismatch(t *testing.T) {
	t.Run("环境变量覆盖 profile 的 app_id", func(t *testing.T) {
		withCmdHome(t)
		if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		t.Setenv("FEISHU_APP_ID", "cli_other")
		t.Setenv("FEISHU_APP_SECRET", "sec_other")
		got := appCredentialWarning()
		if !strings.Contains(got, "不是同一个应用") || !strings.Contains(got, `profile "work"`) {
			t.Errorf("缺少错配与 token 归属说明: %q", got)
		}
		if strings.Contains(got, "sec_other") || strings.Contains(got, "sec_work") {
			t.Errorf("app_secret 泄漏进告警: %q", got)
		}
	})

	t.Run("环境变量覆盖旧布局的 app_id", func(t *testing.T) {
		home := withCmdHome(t)
		writeLegacyConfig(t, home, "cli_legacy", "sec_file")
		t.Setenv("FEISHU_APP_ID", "cli_other")
		t.Setenv("FEISHU_APP_SECRET", "sec_other")
		got := appCredentialWarning()
		if !strings.Contains(got, "不是同一个应用") || !strings.Contains(got, "旧布局") {
			t.Errorf("旧布局错配文案不对: %q", got)
		}
	})

	t.Run("--bot-app-id 指向另一个应用", func(t *testing.T) {
		withCmdHome(t)
		if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		config.SetBotFlagCredentials("cli_flag", "sec_flag")
		if got := appCredentialWarning(); !strings.Contains(got, "--bot-app-id") || !strings.Contains(got, "cli_flag") {
			t.Errorf("flag 错配文案不对: %q", got)
		}
	})
}

// app_id 与 app_secret 分处不同层时提示拼接风险。
func TestAppCredentialWarningOnSplitSources(t *testing.T) {
	t.Run("只传 --bot-app-secret，app_id 走 profile", func(t *testing.T) {
		withCmdHome(t)
		if err := profile.Create("work", profile.CreateOpts{AppID: "cli_work", AppSecret: "sec_work", SwitchTo: true}); err != nil {
			t.Fatal(err)
		}
		config.SetBotFlagCredentials("", "sec_flag")
		got := appCredentialWarning()
		if !strings.Contains(got, "app_secret 来自") {
			t.Errorf("缺少拼接风险提示: %q", got)
		}
		if strings.Contains(got, "sec_flag") {
			t.Errorf("app_secret 泄漏进告警: %q", got)
		}
	})

	t.Run("只 export app_secret 且没有任何 app_id", func(t *testing.T) {
		withCmdHome(t)
		t.Setenv("FEISHU_APP_SECRET", "sec_env")
		if got := appCredentialWarning(); got != "" {
			t.Errorf("缺 app_id 由 Validate 报错即可，不重复告警: %q", got)
		}
	})
}

// `--profile X profile use Y` 时，告警必须讲切换后的 Y，不能一行 X 一行 Y。
func TestAppCredentialWarningForTargetsGivenProfile(t *testing.T) {
	withCmdHome(t)
	if err := profile.Create("alert", profile.CreateOpts{AppID: "cli_alert", AppSecret: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := profile.Create("notify", profile.CreateOpts{AppID: "cli_notify", AppSecret: "s2", SwitchTo: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.SetCommandOverride("alert"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_APP_ID", "cli_third")
	t.Setenv("FEISHU_APP_SECRET", "sec")

	dir, err := profile.ProfileDir("notify")
	if err != nil {
		t.Fatal(err)
	}
	got := appCredentialWarningForProfile("notify", dir)
	if !strings.Contains(got, `profile "notify"`) || !strings.Contains(got, "cli_notify") {
		t.Errorf("显式入口应针对目标 profile: %q", got)
	}
	if strings.Contains(got, "alert") {
		t.Errorf("显式入口不该混入 --profile 指向的 profile: %q", got)
	}

	// 不带参数的入口仍跟随 --profile，供普通命令使用
	if got := appCredentialWarning(); !strings.Contains(got, `profile "alert"`) {
		t.Errorf("默认入口应跟随 --profile: %q", got)
	}
}

func TestConfigInitFallbackScope(t *testing.T) {
	if !configInitOptional(authStatusCmd) || !configInitOptional(authLogoutCmd) {
		t.Error("auth status/logout 应在配置损坏时降级继续")
	}
	if configInitOptional(authLoginCmd) {
		t.Error("auth login 需要 App 凭证，不能降级")
	}
	if !isProfileUseCmd(profileUseCmd) {
		t.Error("profile use 应由自己在切换后打印凭证告警")
	}
	if isProfileUseCmd(profileCurrentCmd) || isProfileUseCmd(authStatusCmd) {
		t.Error("isProfileUseCmd 误判了其他命令")
	}
}
