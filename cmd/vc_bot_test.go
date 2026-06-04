package cmd

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

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
		{"meeting-join", []string{"meeting-number", "password", "dry-run", "output", "user-access-token"}, "meeting-number"},
		{"meeting-leave", []string{"meeting-id", "dry-run", "output", "user-access-token"}, "meeting-id"},
		{"meeting-events", []string{"meeting-id", "start", "end", "page-size", "page-token", "dry-run", "output", "user-access-token"}, "meeting-id"},
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
	// meeting-join / meeting-leave 默认 Bot/Tenant 身份；meeting-events 例外——该端点拒收
	// Tenant Token（99991663），走 User 优先 + Tenant 兜底，故单独断言（见下）。
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
	// meeting-events 端点不接受 Tenant Token，help 应说明走 User 身份。
	if !strings.Contains(vcBotEventsCmd.Long, "User Token") && !strings.Contains(vcBotEventsCmd.Long, "User 身份") {
		t.Errorf("meeting-events Long 应说明走 User Token，实际:\n%s", vcBotEventsCmd.Long)
	}
	if f := vcBotEventsCmd.Flags().Lookup("user-access-token"); f == nil || !strings.Contains(f.Usage, "User 身份") {
		t.Errorf("meeting-events --user-access-token help 应说明 User 身份")
	}
}

func newVCBotJoinTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
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
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
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
				mustSetFlag(t, cmd, "meeting-id", "m1")
			},
			run:      func(cmd *cobra.Command) error { return vcBotLeaveCmd.RunE(cmd, nil) },
			response: `{"code":0,"msg":"success","data":null}`,
		},
		{
			name:   "meeting-events",
			path:   "/open-apis/vc/v1/bots/events",
			newCmd: newVCBotEventsTestCmd,
			setup: func(t *testing.T, cmd *cobra.Command) {
				mustSetFlag(t, cmd, "meeting-id", "m1")
			},
			run:      func(cmd *cobra.Command) error { return vcBotEventsCmd.RunE(cmd, nil) },
			response: `{"code":0,"msg":"success","data":{"meeting_event_list":[],"has_more":false,"page_token":""}}`,
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

// TestVCBotEventsDefaultsToUserToken 验证 meeting-events 走「User 优先 + Tenant 兜底」：
// 该端点不接受 Tenant Token（飞书网关 99991663），故已登录（env/token.json 存在 User Token）时
// 默认就用 User 身份，而非像 join/leave 那样回落 Tenant。这是 BUG #10 修复后的正确行为。
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
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"meeting_event_list":[],"has_more":false,"page_token":""}}`)
	})
	defer cleanup()

	cmd := newVCBotEventsTestCmd()
	mustSetFlag(t, cmd, "meeting-id", "m1")
	if err := vcBotEventsCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("meeting-events 返回错误: %v", err)
	}
	if capturedAuth != "Bearer u-env-token" {
		t.Fatalf("Authorization = %q, want %q（meeting-events 应优先 User Token）", capturedAuth, "Bearer u-env-token")
	}
}

// TestVCBotCommandsUseFlagUserToken 验证只有显式 --user-access-token 才切到 User Token。
func TestVCBotCommandsUseFlagUserToken(t *testing.T) {
	for _, tc := range vcBotTokenModeCases() {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)

			var capturedAuth string
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
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
			if err := tc.run(cmd); err != nil {
				t.Fatalf("%s 返回错误: %v", tc.name, err)
			}
			if capturedAuth != "Bearer "+testUserToken {
				t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer "+testUserToken)
			}
		})
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
