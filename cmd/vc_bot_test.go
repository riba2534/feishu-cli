package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// TestVCBotCmdRegistered 验证 bot 父组与三个子命令注册
func TestVCBotCmdRegistered(t *testing.T) {
	if vcBotCmd.Use != "bot" {
		t.Fatalf("Use = %q, want bot", vcBotCmd.Use)
	}
	found := false
	for _, sub := range vcCmd.Commands() {
		if sub == vcBotCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("vcBotCmd should be child of vcCmd")
	}

	got := map[string]bool{}
	for _, sub := range vcBotCmd.Commands() {
		got[sub.Use] = true
	}
	for _, use := range []string{"meeting-join", "meeting-leave", "meeting-events"} {
		if !got[use] {
			t.Errorf("missing bot subcommand %q", use)
		}
	}
}

func botSub(use string) *cobra.Command {
	for _, sub := range vcBotCmd.Commands() {
		if sub.Use == use {
			return sub
		}
	}
	return nil
}

// TestVCBotFlagsRequired 验证三个子命令的 flag 注册与必填项
func TestVCBotFlagsRequired(t *testing.T) {
	cases := []struct {
		name     string
		flags    []string
		required string
	}{
		{"meeting-join", []string{"meeting-number", "password", "call-id", "action", "dry-run", "output", "user-access-token"}, "meeting-number"},
		{"meeting-leave", []string{"meeting-id", "dry-run", "output", "user-access-token"}, "meeting-id"},
		{"meeting-events", []string{"meeting-id", "start", "end", "page-size", "page-token", "page-all", "dry-run", "output", "as", "user-access-token"}, "meeting-id"},
	}
	for _, tc := range cases {
		c := botSub(tc.name)
		if c == nil {
			t.Fatalf("subcommand %q not found", tc.name)
		}
		for _, f := range tc.flags {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("%s: --%s missing", tc.name, f)
			}
		}
		req := c.Flags().Lookup(tc.required)
		if req == nil {
			t.Fatalf("%s: required flag --%s missing", tc.name, tc.required)
		}
		ann := req.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("%s: --%s should be required, ann=%v", tc.name, tc.required, ann)
		}
		if out := c.Flags().Lookup("output"); out != nil && out.Shorthand != "o" {
			t.Errorf("%s: --output shorthand=%q, want o", tc.name, out.Shorthand)
		}
	}
}

// TestVCBotHelpDocumentsTenantDefault 验证 bot 命令帮助明确默认 Bot/Tenant 身份。
func TestVCBotHelpDocumentsTenantDefault(t *testing.T) {
	if !strings.Contains(vcBotCmd.Long, "默认使用 Bot/Tenant Access Token") {
		t.Fatalf("bot Long 应说明默认使用 Bot/Tenant Access Token，实际:\n%s", vcBotCmd.Long)
	}
	// meeting-join / meeting-leave 默认 Bot/Tenant 身份；meeting-events 用显式 --as。
	for _, c := range []*cobra.Command{vcBotJoinCmd, vcBotLeaveCmd} {
		if !strings.Contains(c.Long, "默认使用 Bot/Tenant 身份") {
			t.Errorf("%s Long 应说明默认 Bot/Tenant 身份", c.Use)
		}
		f := c.Flags().Lookup("user-access-token")
		if f == nil {
			t.Errorf("%s 缺少 --user-access-token", c.Use)
			continue
		}
		if !strings.Contains(f.Usage, "默认 Bot/Tenant 身份") {
			t.Errorf("%s --user-access-token help 应说明默认 Bot/Tenant 身份，实际 %q", c.Use, f.Usage)
		}
	}
	if vcBotEventsCmd.Flags().Lookup("as") == nil || vcBotEventsCmd.Flags().Lookup("as").DefValue != "auto" {
		t.Fatal("meeting-events 应注册 --as，默认 auto")
	}
	if !strings.Contains(vcBotEventsCmd.Long, "--as bot") || !strings.Contains(vcBotEventsCmd.Long, "--as user") {
		t.Errorf("meeting-events Long 应说明显式 --as bot|user|auto，实际:\n%s", vcBotEventsCmd.Long)
	}
	if !strings.Contains(vcBotEventsCmd.Long, "meeting_id 来源") {
		t.Errorf("meeting-events Long 应说明身份须与 meeting_id 来源一致，实际:\n%s", vcBotEventsCmd.Long)
	}
	if !strings.Contains(vcBotLeaveCmd.Long, "vc:meeting.bot.join:write") {
		t.Errorf("meeting-leave Long 应使用官方 join scope，实际:\n%s", vcBotLeaveCmd.Long)
	}
	if strings.Contains(vcBotLeaveCmd.Long, "vc:meeting.bot.leave:write") {
		t.Errorf("meeting-leave 不应再宣传不存在的 leave scope")
	}
	if !strings.Contains(vcBotCmd.Long, "vc:meeting.bot.join:write") || strings.Contains(vcBotCmd.Long, "vc:meeting.bot.leave:write") {
		t.Errorf("bot Long 的 leave scope 应与 join 相同，实际:\n%s", vcBotCmd.Long)
	}
}

// testVCMeetingID 测试用长数字 meeting_id（meeting-events 会校验正整数且拒绝 9 位会议号）
const testVCMeetingID = "6911188411932033028"

func newVCBotJoinTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("call-id", "", "")
	cmd.Flags().String("action", "join", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func newVCBotLeaveTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func newVCBotEventsTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("as", "auto", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func vcBotTokenModeCases() []struct {
	name     string
	path     string
	newCmd   func() *cobra.Command
	setup    func(*testing.T, *cobra.Command)
	run      func(*cobra.Command) error
	response string
} {
	return []struct {
		name     string
		path     string
		newCmd   func() *cobra.Command
		setup    func(*testing.T, *cobra.Command)
		run      func(*cobra.Command) error
		response string
	}{
		{
			name:   "meeting-join",
			path:   "/open-apis/vc/v1/bots/join",
			newCmd: newVCBotJoinTestCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				mustSetFlag(t, cmd, "meeting-number", "123456789")
			},
			run:      func(cmd *cobra.Command) error { return vcBotJoinCmd.RunE(cmd, nil) },
			response: `{"code":0,"msg":"success","data":{"meeting_id":"m1"}}`,
		},
		{
			name:   "meeting-leave",
			path:   "/open-apis/vc/v1/bots/leave",
			newCmd: newVCBotLeaveTestCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
			},
			run:      func(cmd *cobra.Command) error { return vcBotLeaveCmd.RunE(cmd, nil) },
			response: `{"code":0,"msg":"success","data":null}`,
		},
		{
			name:   "meeting-events",
			path:   "/open-apis/vc/v1/bots/events",
			newCmd: newVCBotEventsTestCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
			},
			run:      func(cmd *cobra.Command) error { return vcBotEventsCmd.RunE(cmd, nil) },
			response: `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`,
		},
	}
}

// TestVCBotCommandsDefaultToTenantToken 验证 vc bot 三个 API 默认走 Bot/Tenant Token；
// 即使环境变量存在 User Token，也不能被隐式切到用户身份。
func TestVCBotCommandsDefaultToTenantToken(t *testing.T) {
	for _, tc := range vcBotTokenModeCases() {
		// meeting-events 端点拒收 Tenant Token（99991663），改走 User 优先 + Tenant 兜底，
		// 默认 token 行为不同，单独由 TestVCBotEventsDefaultsToUserToken 覆盖。
		if tc.name == "meeting-events" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")

			var capturedAuth string
			cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
					return
				}
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.response)
			}))
			defer cleanup()

			cmd := tc.newCmd()
			tc.setup(t, cmd)
			if err := tc.run(cmd); err != nil {
				t.Fatalf("%s 返回错误: %v", tc.name, err)
			}
			if capturedAuth != testTenantAuth {
				t.Fatalf("Authorization = %q, want %q", capturedAuth, testTenantAuth)
			}
		})
	}
}

func TestVCBotEventsAsBotIgnoresUserToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")

	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/vc/v1/bots/events" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`)
	}))
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "bot")
	if err := vcBotEventsCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("meeting-events --as bot 返回错误: %v", err)
	}
	if capturedAuth != testTenantAuth {
		t.Fatalf("Authorization = %q, want %q（--as bot 即使已登录也走 Bot）", capturedAuth, testTenantAuth)
	}
}

func TestVCBotEventsAsUserFailClosed(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("app_id: cli_test\napp_secret: secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "user")
	if err := vcBotEventsCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("--as user 缺 Token 应失败，实际: %v", err)
	}
	mustSetFlag(t, cmd, "dry-run", "true")
	if err := vcBotEventsCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("dry-run 也应 fail-closed，实际: %v", err)
	}
}

func TestVCBotEventsInvalidAsFailClosed(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "nobody")
	mustSetFlag(t, cmd, "dry-run", "true")
	if err := vcBotEventsCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "bot|user|auto") {
		t.Fatalf("非法 --as 在 dry-run 也应失败，实际: %v", err)
	}
}

func TestVCBotEventsDryRunIncludesResolvedIdentity(t *testing.T) {
	isolateVCBotEventsIdentityEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "bot")
	mustSetFlag(t, cmd, "dry-run", "true")
	out, err := captureVCBotStdout(t, func() error { return vcBotEventsCmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run --as bot 不应请求网络: %v", err)
	}
	if !strings.Contains(out, `"as": "bot"`) {
		t.Fatalf("dry-run --as bot 预览应含 as=bot，实际:\n%s", out)
	}
}

func TestResolveVCBotEventsIdentity(t *testing.T) {
	isolateVCBotEventsIdentityEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "as", "bot")
	token, identity, err := resolveVCBotEventsIdentity(cmd)
	if err != nil || token != "" || identity != "bot" {
		t.Fatalf("--as bot 即使已登录也应走 Bot, token=%q identity=%q err=%v", token, identity, err)
	}
	mustSetFlag(t, cmd, "as", "auto")
	token, identity, err = resolveVCBotEventsIdentity(cmd)
	if err != nil || token != "u-env-token" || identity != "user" {
		t.Fatalf("--as auto 已登录应为 user, token=%q identity=%q err=%v", token, identity, err)
	}
	mustSetFlag(t, cmd, "as", "user")
	token, identity, err = resolveVCBotEventsIdentity(cmd)
	if err != nil || token != "u-env-token" || identity != "user" {
		t.Fatalf("--as user 有 Token 应为 user, token=%q identity=%q err=%v", token, identity, err)
	}
	mustSetFlag(t, cmd, "as", "nobody")
	if _, _, err = resolveVCBotEventsIdentity(cmd); err == nil || !strings.Contains(err.Error(), "bot|user|auto") {
		t.Fatalf("非法 --as 应失败，实际: %v", err)
	}
}

func isolateVCBotEventsIdentityEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_BASE_URL", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("app_id: cli_test\napp_secret: secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	return home
}

func writeExpiredUserTokenFile(t *testing.T, home, refreshToken string) string {
	t.Helper()
	tokenDir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(tokenDir, "token.json")
	store := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-1 * time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "vc:meeting.meetingevent:read",
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	return tokenFile
}

func captureVCBotStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("读取 stdout 失败: %v", readErr)
	}
	return string(out), runErr
}

func TestVCBotEventsAutoFailClosedOnRefreshError(t *testing.T) {
	home := isolateVCBotEventsIdentityEnv(t)
	writeExpiredUserTokenFile(t, home, "broken-refresh-token")

	var bizRequests int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/authen/v2/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token is invalid"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/open-apis/vc/v1/bots/events") {
			atomic.AddInt32(&bizRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Errorf("User refresh 失败时不应请求 tenant access token 尝试切 Bot")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "auto")
	err := vcBotEventsCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("--as auto 刷新失败应 fail-closed，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "刷新") && !strings.Contains(err.Error(), "refresh") && !strings.Contains(err.Error(), "token") {
		t.Fatalf("错误应说明刷新/token 失败，实际: %v", err)
	}
	if hits := atomic.LoadInt32(&bizRequests); hits != 0 {
		t.Fatalf("刷新失败不得以 Bot 身份打业务端点，实际 %d 次", hits)
	}
}

func TestVCBotEventsAutoFailClosedOnTokenFileError(t *testing.T) {
	home := isolateVCBotEventsIdentityEnv(t)
	tokenDir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tokenDir, "token.json"), []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}

	var bizRequests int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/vc/v1/bots/events") ||
			strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			atomic.AddInt32(&bizRequests, 1)
		}
		http.Error(w, "token 文件损坏时不应发请求", http.StatusInternalServerError)
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "auto")
	if err := vcBotEventsCmd.RunE(cmd, nil); err == nil {
		t.Fatal("token.json 损坏时应 fail-closed，不得静默切 Bot")
	}
	if hits := atomic.LoadInt32(&bizRequests); hits != 0 {
		t.Fatalf("token 文件错误不得发业务/Bot 请求，实际 %d 次", hits)
	}
}

func TestVCBotEventsAutoNoUserFallsBackToBot(t *testing.T) {
	isolateVCBotEventsIdentityEnv(t)

	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/vc/v1/bots/events" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`)
	}))
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "auto")
	token, identity, err := resolveVCBotEventsIdentity(cmd)
	if err != nil || token != "" || identity != "bot" {
		t.Fatalf("未配置 User 时 auto 应回落 Bot, token=%q identity=%q err=%v", token, identity, err)
	}
	if err := vcBotEventsCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("未登录 --as auto 应走 Bot 成功，实际: %v", err)
	}
	if capturedAuth != testTenantAuth {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, testTenantAuth)
	}
}

func TestVCBotEventsAsUserUsesUserToken(t *testing.T) {
	isolateVCBotEventsIdentityEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")

	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			http.Error(w, "不应请求 tenant_access_token", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/open-apis/vc/v1/bots/events" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`)
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "user")
	if err := vcBotEventsCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("meeting-events --as user 返回错误: %v", err)
	}
	if capturedAuth != "Bearer u-env-token" {
		t.Fatalf("Authorization = %q, want Bearer u-env-token", capturedAuth)
	}
}

func TestVCBotEventsDryRunStaticIdentityNoNetworkNoTokenWrite(t *testing.T) {
	home := isolateVCBotEventsIdentityEnv(t)
	tokenFile := writeExpiredUserTokenFile(t, home, "valid-refresh-token")
	hashBefore, err := fileSHA256(tokenFile)
	if err != nil {
		t.Fatalf("计算 token.json hash 失败: %v", err)
	}

	var serverHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverHits, 1)
		http.Error(w, "dry-run 不应调用任何服务端接口", http.StatusInternalServerError)
	})
	defer cleanup()

	cases := []struct {
		as   string
		want string
	}{
		{"auto", "user"},
		{"user", "user"},
		{"bot", "bot"},
	}
	for _, tc := range cases {
		t.Run("as="+tc.as, func(t *testing.T) {
			cmd := newVCBotEventsTestCmd()
			mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
			mustSetFlag(t, cmd, "as", tc.as)
			mustSetFlag(t, cmd, "dry-run", "true")
			out, runErr := captureVCBotStdout(t, func() error { return vcBotEventsCmd.RunE(cmd, nil) })
			if runErr != nil {
				t.Fatalf("dry-run --as %s 失败: %v", tc.as, runErr)
			}
			want := fmt.Sprintf(`"as": %q`, tc.want)
			if !strings.Contains(out, want) {
				t.Fatalf("dry-run --as %s 预览应含 %s，实际:\n%s", tc.as, want, out)
			}
			if hits := atomic.LoadInt32(&serverHits); hits != 0 {
				t.Errorf("--dry-run 不应触发网络请求，得到 %d 次", hits)
			}
			hashAfter, err := fileSHA256(tokenFile)
			if err != nil {
				t.Fatalf("计算 token.json hash 失败: %v", err)
			}
			if hashBefore != hashAfter {
				t.Errorf("token.json 被改写: before=%s after=%s", hashBefore, hashAfter)
			}
		})
	}

	t.Run("as=invalid", func(t *testing.T) {
		cmd := newVCBotEventsTestCmd()
		mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
		mustSetFlag(t, cmd, "as", "nobody")
		mustSetFlag(t, cmd, "dry-run", "true")
		if err := vcBotEventsCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "bot|user|auto") {
			t.Fatalf("非法 --as 在 dry-run 也应失败，实际: %v", err)
		}
		if hits := atomic.LoadInt32(&serverHits); hits != 0 {
			t.Errorf("非法 --as dry-run 不应联网，得到 %d 次", hits)
		}
	})
}

// TestVCBotEventsDefaultsToUserToken 验证 --as auto（默认）在已登录时走 User Token。
func TestVCBotEventsDefaultsToUserToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")

	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			http.Error(w, "不应请求 tenant_access_token（meeting-events 应优先 User Token）", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/open-apis/vc/v1/bots/events" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"events":[],"has_more":false,"page_token":""}}`)
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	if err := vcBotEventsCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("meeting-events 返回错误: %v", err)
	}
	if capturedAuth != "Bearer u-env-token" {
		t.Fatalf("Authorization = %q, want %q（meeting-events 应优先 User Token）", capturedAuth, "Bearer u-env-token")
	}
}

// TestVCBotJoinLeaveRejectUserToken meeting-join / meeting-leave 仅支持 Bot 身份（官方 #2570）：
// 显式传 --user-access-token 报用法错误且不发任何请求；meeting-events 仍可用显式 User Token。
func TestVCBotJoinLeaveRejectUserToken(t *testing.T) {
	for _, tc := range vcBotTokenModeCases() {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			var hits int32
			var capturedAuth string
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					http.Error(w, "不应请求 tenant_access_token", http.StatusInternalServerError)
					return
				}
				if r.URL.Path != tc.path {
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
					return
				}
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.response)
			})
			defer cleanup()

			cmd := tc.newCmd()
			tc.setup(t, cmd)
			mustSetFlag(t, cmd, "user-access-token", testUserToken)
			err := tc.run(cmd)
			if tc.name == "meeting-events" {
				if err != nil {
					t.Fatalf("%s 返回错误: %v", tc.name, err)
				}
				if capturedAuth != "Bearer "+testUserToken {
					t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer "+testUserToken)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "仅支持 Bot 身份") {
				t.Fatalf("%s 传 --user-access-token 应报错，实际: %v", tc.name, err)
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误（exit 2），kinds=%v", clierr.Kinds(err))
			}
			if n := atomic.LoadInt32(&hits); n != 0 {
				t.Fatalf("拒绝 User Token 时不应发请求，实际 %d 次", n)
			}
		})
	}
}

// TestVCBotEventsTextReadsEvents 锁住 #6：文本模式读 data.events（官方 vc_meeting_events.go 与服务端字段），
// 旧实现读 meeting_event_list，永远输出 0 条。
func TestVCBotEventsTextReadsEvents(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"has_more":true,"page_token":"p2","events":[`+
			`{"event_id":"e1","event_type":"participant_joined","event_time":"1790757108"},`+
			`{"event_id":"e2","event_type":"transcript_received","event_time":"1790757109"}]}}`)
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "user")
	out, err := captureVCBotStdout(t, func() error { return vcBotEventsCmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("meeting-events 返回错误: %v", err)
	}
	for _, want := range []string{"共 2 条", "participant_joined", "transcript_received", `"event_id":"e2"`, "--page-token p2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("文本输出缺少 %q:\n%s", want, out)
		}
	}
}

// TestVCBotEventsPageAll --page-all 合并多页 events，page_size 取 100，重复游标即停止。
func TestVCBotEventsPageAll(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	var calls int32
	var sizes []string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sizes = append(sizes, r.URL.Query().Get("page_size"))
		switch r.URL.Query().Get("page_token") {
		case "":
			atomic.AddInt32(&calls, 1)
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"page_token":"p2","events":[{"event_id":"e1"}]}}`)
		case "p2":
			atomic.AddInt32(&calls, 1)
			// 服务端异常地回同一个游标：必须停止，不能死循环
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"page_token":"p2","events":[{"event_id":"e2"}]}}`)
		default:
			http.Error(w, "unexpected token", http.StatusBadRequest)
		}
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", testVCMeetingID)
	mustSetFlag(t, cmd, "as", "user")
	mustSetFlag(t, cmd, "page-all", "true")
	mustSetFlag(t, cmd, "output", "json")
	out, err := captureVCBotStdout(t, func() error { return vcBotEventsCmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("--page-all 返回错误: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("应请求 2 页后因重复游标停止，实际 %d", n)
	}
	var parsed struct {
		Events    []map[string]any `json:"events"`
		HasMore   bool             `json:"has_more"`
		PageToken string           `json:"page_token"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if len(parsed.Events) != 2 || !parsed.HasMore || parsed.PageToken != "p2" {
		t.Fatalf("合并结果不符: %+v", parsed)
	}
	for _, sz := range sizes {
		if sz != "100" {
			t.Fatalf("--page-all 应使用 page_size=100，实际 %v", sizes)
		}
	}
}

// TestValidateVCEventsMeetingID meeting-events 拒绝 9 位会议号与非数字 meeting_id（用法错误）。
func TestValidateVCEventsMeetingID(t *testing.T) {
	cases := []struct {
		in      string
		wantErr string
	}{
		{testVCMeetingID, ""},
		{"", "必填"},
		{"123456789", "9 位会议号"},
		{"m1", "正整数"},
		{"-5", "正整数"},
	}
	for _, tc := range cases {
		err := validateVCEventsMeetingID(tc.in)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%q: 意外错误 %v", tc.in, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%q: err=%v, want 用法错误包含 %q", tc.in, err, tc.wantErr)
		}
	}
}

// TestVCBotJoinValidationAndDryRun 会议号必须 9 位数字；--call-id / --action start 进入请求体；dry-run 不联网。
func TestVCBotJoinValidationAndDryRun(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var hits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "不应联网", http.StatusInternalServerError)
	})
	defer cleanup()

	for _, bad := range []string{"12345678", "1234567890", "12345678a", ""} {
		cmd := newVCBotJoinTestCmd()
		mustSetFlag(t, cmd, "meeting-number", bad)
		err := vcBotJoinCmd.RunE(cmd, nil)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("会议号 %q 应为用法错误，实际 %v", bad, err)
		}
	}
	cmd := newVCBotJoinTestCmd()
	mustSetFlag(t, cmd, "meeting-number", "123456789")
	mustSetFlag(t, cmd, "action", "bogus")
	if err := vcBotJoinCmd.RunE(cmd, nil); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Errorf("非法 --action 应为用法错误，实际 %v", err)
	}

	cmd = newVCBotJoinTestCmd()
	mustSetFlag(t, cmd, "meeting-number", "123456789")
	mustSetFlag(t, cmd, "call-id", "call-1")
	mustSetFlag(t, cmd, "action", "start")
	mustSetFlag(t, cmd, "dry-run", "true")
	out, err := captureVCBotStdout(t, func() error { return vcBotJoinCmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	for _, want := range []string{`"call_id": "call-1"`, `"action": 2`, `"meeting_no": "123456789"`, `"as": "bot"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run 预览缺少 %s:\n%s", want, out)
		}
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("校验失败与 dry-run 都不应联网，实际 %d 次", n)
	}
}

// TestValidateVCPageSize 验证 page-size 校验放行 0（默认）与 20-100，拒绝 1-19 与 >100
func TestValidateVCPageSize(t *testing.T) {
	cases := []struct {
		pageSize int
		wantErr  bool
	}{
		{0, false},   // 未传，回落默认
		{1, true},    // 下界以下
		{5, true},    // lark/help 声明 20-100，1-19 应拒绝
		{19, true},   // 边界
		{20, false},  // 下界
		{50, false},  // 中间
		{100, false}, // 上界
		{101, true},  // 上界以上
		{-1, true},   // 负数
	}
	for _, tc := range cases {
		err := validateVCPageSize(tc.pageSize)
		if tc.wantErr && err == nil {
			t.Errorf("page-size=%d: expected error, got nil", tc.pageSize)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("page-size=%d: unexpected error: %v", tc.pageSize, err)
		}
	}
}

// TestVCStartAfterEnd 验证 start/end 用 int64 数值比较（而非字符串字典序）。
func TestVCStartAfterEnd(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		wantAfter  bool
		wantErr    bool
	}{
		// 位数不同：字典序 "999999999" > "1000000000"（误判 start 晚于 end），数值序则 start 早于 end
		{"位数不同数值序正确-start早于end", "999999999", "1000000000", false, false},
		// 位数不同反向：start 数值确实大于 end
		{"位数不同start确实晚", "1000000000", "999999999", true, false},
		{"同位数start晚", "200", "100", true, false},
		{"同位数start早", "100", "200", false, false},
		{"相等", "100", "100", false, false},
		{"start空跳过", "", "100", false, false},
		{"end空跳过", "100", "", false, false},
		{"非数值报错", "abc", "100", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after, err := vcStartAfterEnd(tc.start, tc.end)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got after=%v", after)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if after != tc.wantAfter {
				t.Fatalf("vcStartAfterEnd(%q,%q) = %v, want %v", tc.start, tc.end, after, tc.wantAfter)
			}
		})
	}
}

// TestVCParseTimeToUnixSec 验证时间字符串到 Unix 秒的转换
func TestVCParseTimeToUnixSec(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := vcParseTimeToUnixSec("", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("empty input should yield empty string, got %q", got)
		}
	})

	t.Run("rfc3339", func(t *testing.T) {
		in := "2026-03-01T00:00:00+08:00"
		got, err := vcParseTimeToUnixSec(in, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.FixedZone("", 8*3600)).Unix()
		if got != strconv.FormatInt(want, 10) {
			t.Fatalf("got %q, want %d", got, want)
		}
	})

	t.Run("date end aligns later than start", func(t *testing.T) {
		startSec, err := vcParseTimeToUnixSec("2026-03-01", false)
		if err != nil {
			t.Fatalf("start err: %v", err)
		}
		endSec, err := vcParseTimeToUnixSec("2026-03-01", true)
		if err != nil {
			t.Fatalf("end err: %v", err)
		}
		if startSec == "" || endSec == "" {
			t.Fatal("expected non-empty seconds")
		}
		s, _ := strconv.ParseInt(startSec, 10, 64)
		e, _ := strconv.ParseInt(endSec, 10, 64)
		if e-s != 86399 {
			t.Fatalf("end-start = %d, want 86399 (23:59:59 alignment)", e-s)
		}
	})

	t.Run("unix seconds passthrough", func(t *testing.T) {
		// help 宣传接受 Unix 秒，纯整数应原样透传，不被 parseVCTime 拒绝
		got, err := vcParseTimeToUnixSec("1709251200", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "1709251200" {
			t.Fatalf("got %q, want 1709251200", got)
		}
	})

	t.Run("non-positive unix seconds rejected", func(t *testing.T) {
		if _, err := vcParseTimeToUnixSec("0", false); err == nil {
			t.Fatal("expected error for zero unix seconds")
		}
		if _, err := vcParseTimeToUnixSec("-5", false); err == nil {
			t.Fatal("expected error for negative unix seconds")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := vcParseTimeToUnixSec("nonsense", false); err == nil {
			t.Fatal("expected error for invalid input")
		}
	})
}
