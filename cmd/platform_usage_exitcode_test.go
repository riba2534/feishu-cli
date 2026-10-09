package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// isolatePlatformCLIEnv 用临时 HOME + 伪造 App 凭证隔离测试，并把 base_url 指向计数桩：
// 用法错误必须在发任何请求之前返回，桩被命中即视为失败。绝不触碰真实 ~/.feishu-cli。
func isolatePlatformCLIEnv(t *testing.T) *int32 {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_APP_ID", "cli_fake_app")
	t.Setenv("FEISHU_APP_SECRET", "fake_secret")
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "用法错误不应发起网络请求", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FEISHU_BASE_URL", srv.URL)
	return &hits
}

func assertCLIExit(t *testing.T, wantExit int, wantMsg string, args ...string) {
	t.Helper()
	_, stderr, err := runCLI(t, args...)
	if err == nil {
		t.Fatalf("%v 应返回错误", args)
	}
	if got := exitCodeFor(err); got != wantExit {
		t.Fatalf("%v 退出码 = %d，want %d（err=%v）", args, got, wantExit, err)
	}
	if wantMsg != "" && !strings.Contains(err.Error()+stderr, wantMsg) {
		t.Fatalf("%v 错误信息应包含 %q，得到: %v", args, wantMsg, err)
	}
}

// ① doctor --only 拼错的检查名属于用法错误（退出码 2），不是"检查失败"（退出码 1）。
func TestDoctorOnlyUnknownCheckIsUsageError(t *testing.T) {
	hits := isolatePlatformCLIEnv(t)
	for _, only := range []string{"bogus-check", "user_tokn", " , "} {
		_, err := parseOnly(only)
		if !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("parseOnly(%q) 应返回用法错误，得到 %v", only, err)
		}
	}
	assertCLIExit(t, clierr.ExitUsage, "--only 包含未知 check 名", "doctor", "--only", "bogus-check", "--offline")
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Fatalf("doctor --only 用法错误不应发请求，命中 %d 次", n)
	}
}

// ② auth token 的参数冲突与非法 --as 属于用法错误（退出码 2）。
func TestAuthTokenArgConflictsAreUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"auth", "token", "--as", "bot", "--user-access-token", "u-fake"}, "不能同时使用 --as bot 与 --user-access-token"},
		{[]string{"auth", "token", "--bind-legacy-app", "--as", "bot"}, "不能同时使用 --bind-legacy-app 与 --as bot"},
		{[]string{"auth", "token", "--bind-legacy-app", "--user-access-token", "u-fake"}, "不能同时使用 --bind-legacy-app 与 --user-access-token"},
		{[]string{"auth", "token", "--as", "bogus"}, "--as 仅支持 user|bot|auto"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args[2:], " "), func(t *testing.T) {
			hits := isolatePlatformCLIEnv(t)
			assertCLIExit(t, clierr.ExitUsage, tc.msg, tc.args...)
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Fatalf("参数冲突不应发请求，命中 %d 次", n)
			}
		})
	}
}

// ③ search messages / msg search-chats 的 --page-limit 越界属于用法错误（退出码 2）。
func TestSearchPageLimitOutOfRangeIsUsageError(t *testing.T) {
	cases := [][]string{
		{"search", "messages", "fp-test-query", "--page-limit", "-1"},
		{"search", "messages", "fp-test-query", "--page-limit", "41"},
		{"msg", "search-chats", "fp-test-query", "--page-limit", "-1"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			hits := isolatePlatformCLIEnv(t)
			assertCLIExit(t, clierr.ExitUsage, "--page-limit", args...)
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Fatalf("--page-limit 用法错误不应发请求，命中 %d 次", n)
			}
		})
	}
}

// ④ chat member / msg history / wiki space-list 共用 resolveChatToken：非法 --as 为用法错误（退出码 2）。
func TestChatTokenInvalidAsIsUsageError(t *testing.T) {
	cases := [][]string{
		{"chat", "member", "list", "oc_fp_test_fake", "--as", "bogus"},
		{"chat", "member", "add", "oc_fp_test_fake", "--id-list", "ou_fp_test_fake", "--as", "bogus"},
		{"chat", "member", "remove", "oc_fp_test_fake", "--id-list", "ou_fp_test_fake", "--as", "bogus"},
		{"msg", "history", "--container-id", "oc_fp_test_fake", "--as", "bogus"},
		{"wiki", "space-list", "--as", "bogus"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			hits := isolatePlatformCLIEnv(t)
			assertCLIExit(t, clierr.ExitUsage, `--as 仅支持 bot|user|auto，得到 "bogus"`, args...)
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Fatalf("非法 --as 不应发请求，命中 %d 次", n)
			}
		})
	}
}

// ④ 语义不变：auto 未配置 User Token 时回退 Bot（返回空 token、不报错）；bot 恒为空 token。
func TestChatTokenAutoFallbackSemanticsUnchanged(t *testing.T) {
	isolatePlatformCLIEnv(t)
	if err := config.Init(""); err != nil {
		t.Fatal(err)
	}
	c := &cobra.Command{Use: "fake"}
	c.Flags().String("user-access-token", "", "")
	for _, as := range []string{"", "auto", "AUTO", "bot", "tenant", "app"} {
		token, err := resolveChatToken(c, as)
		if err != nil || token != "" {
			t.Errorf("resolveChatToken(%q) 未配置 User Token 时应回退 Bot（空 token），得到 (%q, %v)", as, token, err)
		}
	}
	if _, err := resolveChatToken(c, "user"); err == nil || exitCodeFor(err) != clierr.ExitAuth {
		t.Errorf("--as user 缺 User Token 应为鉴权类错误（退出码 3），得到 %v", err)
	}
	_ = c.Flags().Set("user-access-token", "u-fp-test-explicit")
	if token, err := resolveChatToken(c, "auto"); err != nil || token != "u-fp-test-explicit" {
		t.Errorf("auto 有显式 User Token 时应使用之，得到 (%q, %v)", token, err)
	}
}

// ⑤ api --as user --dry-run 缺 User Token 与真实调用一致，均为鉴权类错误（退出码 3）。
func TestRunAPI_AsUserWithoutTokenDryRunMatchesRealExitCode(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	var hits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "缺 User Token 时不应发请求", http.StatusInternalServerError)
	})
	defer cleanup()

	codes := map[bool]int{}
	for _, dryRun := range []bool{true, false} {
		resetAPIFlags()
		cmd := newTestAPICmd()
		apiDryRun = dryRun
		apiAs = "user"
		err := cmd.RunE(cmd, []string{"GET", "/open-apis/contact/v3/users/me"})
		if err == nil {
			t.Fatalf("dry-run=%v: --as user 缺 User Token 应报错", dryRun)
		}
		if !strings.Contains(err.Error(), "--as user 需要 User Access Token") || !strings.Contains(err.Error(), "feishu-cli auth login") {
			t.Errorf("dry-run=%v: 错误信息应提示登录，得到 %v", dryRun, err)
		}
		codes[dryRun] = exitCodeFor(err)
	}
	if codes[true] != clierr.ExitAuth || codes[false] != clierr.ExitAuth {
		t.Fatalf("退出码 dry-run=%d real=%d，均应为 %d", codes[true], codes[false], clierr.ExitAuth)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("缺 User Token 时不应发请求，命中 %d 次", n)
	}
}
