package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// runCLIWithStdin 在 runCLI 基础上注入 stdin。
func runCLIWithStdin(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	rootCmd.SetIn(strings.NewReader(stdin))
	defer rootCmd.SetIn(nil)
	return runCLI(t, args...)
}

func stubProbe(t *testing.T, fn func(ctx context.Context, appID, appSecret, baseURL string) (*auth.TenantAccessToken, error)) *[]string {
	t.Helper()
	var seen []string
	orig := probeTenantTokenFn
	probeTenantTokenFn = func(ctx context.Context, appID, appSecret, baseURL string) (*auth.TenantAccessToken, error) {
		seen = append(seen, appID+"/"+appSecret)
		return fn(ctx, appID, appSecret, baseURL)
	}
	t.Cleanup(func() { probeTenantTokenFn = orig })
	return &seen
}

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	return home
}

func TestProfileAdd_AppSecretStdin(t *testing.T) {
	home := isolatedHome(t)
	stdout, stderr, err := runCLIWithStdin(t, "  sec_from_stdin  \nignored-second-line\n", "profile", "add", "work", "--app-id", "cli_work", "--app-secret-stdin", "--json")
	if err != nil {
		t.Fatalf("profile add: %v\n%s", err, stderr)
	}
	if strings.Contains(stdout+stderr, "sec_from_stdin") {
		t.Fatalf("输出不得回显 secret: %s %s", stdout, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".feishu-cli", "profiles", "work", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `app_secret: "sec_from_stdin"`) || !strings.Contains(string(raw), `app_id: "cli_work"`) {
		t.Fatalf("config.yaml 应写入 stdin 读取的 secret:\n%s", raw)
	}
}

func TestProfileAdd_SecretFlagUsageErrors(t *testing.T) {
	isolatedHome(t)
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"互斥", "x\n", []string{"profile", "add", "a1", "--app-secret", "s", "--app-secret-stdin"}},
		{"stdin 为空", "   \n", []string{"profile", "add", "a2", "--app-secret-stdin"}},
		{"probe 缺凭证", "", []string{"profile", "add", "a3", "--app-id", "cli_x", "--probe"}},
	}
	for _, c := range cases {
		_, _, err := runCLIWithStdin(t, c.stdin, c.args...)
		if err == nil || exitCodeFor(err) != 2 {
			t.Fatalf("%s: 应为用法错误 exit 2: %v", c.name, err)
		}
	}
}

// --probe：服务端明确拒绝凭证 → 退出码 3 且不创建 profile；网络错误只告警；通过时 probe=ok。
func TestProfileAdd_Probe(t *testing.T) {
	home := isolatedHome(t)

	seen := stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return nil, clierr.Auth(&auth.TATError{HTTPStatus: 400, Code: 20002, OAuthError: "invalid_client"})
	})
	_, _, err := runCLIWithStdin(t, "bad_secret\n", "profile", "add", "bad", "--app-id", "cli_bad", "--app-secret-stdin", "--probe")
	if err == nil || exitCodeFor(err) != 3 {
		t.Fatalf("凭证被拒应为鉴权类 exit 3: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".feishu-cli", "profiles", "bad")); !os.IsNotExist(statErr) {
		t.Fatalf("凭证被拒时不应创建 profile: %v", statErr)
	}
	if len(*seen) != 1 || (*seen)[0] != "cli_bad/bad_secret" {
		t.Fatalf("探测应使用 stdin 读到的凭证: %v", *seen)
	}

	stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return nil, errors.New("请求 tenant token 失败: dial tcp: i/o timeout")
	})
	stdout, stderr, err := runCLIWithStdin(t, "s\n", "profile", "add", "flaky", "--app-id", "cli_f", "--app-secret-stdin", "--probe", "--json")
	if err != nil {
		t.Fatalf("网络错误不应阻断: %v", err)
	}
	if !strings.Contains(stderr, "凭证探测未完成") || !strings.Contains(stdout, `"probe":"skipped"`) {
		t.Fatalf("应告警并标记 skipped: stdout=%s stderr=%s", stdout, stderr)
	}

	stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return &auth.TenantAccessToken{AccessToken: "t-ok", ExpiresIn: 7200}, nil
	})
	stdout, _, err = runCLIWithStdin(t, "s\n", "profile", "add", "good", "--app-id", "cli_g", "--app-secret-stdin", "--probe", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(stdout), &out)
	if out["probe"] != "ok" {
		t.Fatalf("probe 应为 ok: %s", stdout)
	}
}

func TestConfigInit_AppSecretStdinAndProbe(t *testing.T) {
	home := isolatedHome(t)
	cfgFile := filepath.Join(home, ".feishu-cli", "config.yaml")

	stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return nil, clierr.Auth(&auth.TATError{HTTPStatus: 400, Code: 20048, OAuthError: "invalid_client"})
	})
	_, _, err := runCLIWithStdin(t, "bad\n", "config", "init", "--app-id", "cli_missing", "--app-secret-stdin", "--probe")
	if err == nil || exitCodeFor(err) != 3 {
		t.Fatalf("凭证被拒应 exit 3: %v", err)
	}
	if _, statErr := os.Stat(cfgFile); !os.IsNotExist(statErr) {
		t.Fatalf("凭证被拒时不应写配置: %v", statErr)
	}

	stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return &auth.TenantAccessToken{AccessToken: "t-ok", ExpiresIn: 7200}, nil
	})
	if _, stderr, err := runCLIWithStdin(t, "good_secret\n", "config", "init", "--app-id", "cli_ok", "--app-secret-stdin", "--probe", "--base-url", "https://open.larksuite.com"); err != nil {
		t.Fatalf("config init: %v\n%s", err, stderr)
	}
	raw, _ := os.ReadFile(cfgFile)
	for _, want := range []string{`app_id: "cli_ok"`, `app_secret: "good_secret"`, `base_url: "https://open.larksuite.com"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("配置应包含 %s:\n%s", want, raw)
		}
	}
	info, _ := os.Stat(cfgFile)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("配置权限应为 0600: %o", info.Mode().Perm())
	}

	// 已存在时报错，且不读 stdin、不探测
	seen := stubProbe(t, func(context.Context, string, string, string) (*auth.TenantAccessToken, error) {
		return &auth.TenantAccessToken{AccessToken: "t", ExpiresIn: 7200}, nil
	})
	if _, _, err := runCLIWithStdin(t, "x\n", "config", "init", "--app-id", "cli_ok", "--app-secret-stdin", "--probe"); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("配置已存在应报错: %v", err)
	}
	if len(*seen) != 0 {
		t.Fatal("配置已存在时不应探测凭证")
	}

	// 无参数：行为与原先一致（空模板）
	if err := os.Remove(cfgFile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, "config", "init"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cfgFile)
	if !strings.Contains(string(raw), `app_id: ""`) || !strings.Contains(string(raw), `base_url: "https://open.feishu.cn"`) {
		t.Fatalf("默认模板应保持原样:\n%s", raw)
	}
}
