package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
)

// 登录未下发 refresh_token：只新增 no_refresh_token，既有字段（refresh_token_present / warnings / hint）保留，
// hint 不再把原因归到 offline_access。
func TestAuthLoginEventNoRefreshToken(t *testing.T) {
	expires := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	event := buildAuthorizationCompleteEvent(&auth.TokenStore{AccessToken: "u", ExpiresAt: expires},
		buildLoginScopeSummary("", ""), "cli_abc", "https://open.feishu.cn")

	if event["refresh_token_present"] != false || event["expires_at"] != expires.Format(time.RFC3339) {
		t.Fatalf("既有字段变化: %v", event)
	}
	info, ok := event["no_refresh_token"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 no_refresh_token: %v", event)
	}
	if info["settings_url"] != "https://open.feishu.cn/app/cli_abc/safe" ||
		info["cause"] != auth.NoRefreshTokenCause || !strings.Contains(info["fix"].(string), "发布") {
		t.Fatalf("no_refresh_token 内容不符: %v", info)
	}
	hint, _ := event["hint"].(string)
	if strings.Contains(hint, "开通 offline_access") || !strings.Contains(hint, "刷新 user_access_token") {
		t.Fatalf("hint 仍归因 offline_access: %q", hint)
	}
	if _, ok := event["warnings"]; !ok {
		t.Fatal("warnings 字段应保留")
	}

	// 有 refresh_token 时不输出该对象
	ok2 := buildAuthorizationCompleteEvent(&auth.TokenStore{AccessToken: "u", RefreshToken: "r"},
		buildLoginScopeSummary("", ""), "cli_abc", "https://open.feishu.cn")
	if _, has := ok2["no_refresh_token"]; has {
		t.Fatal("有 refresh_token 时不应输出 no_refresh_token")
	}

	// Lark 品牌指向 open.larksuite.com；app_id 未知时省略 settings_url
	lark := buildAuthorizationCompleteEvent(&auth.TokenStore{AccessToken: "u"}, buildLoginScopeSummary("", ""), "cli_x", "https://open.larksuite.com")
	if lark["no_refresh_token"].(map[string]any)["settings_url"] != "https://open.larksuite.com/app/cli_x/safe" {
		t.Fatalf("Lark 链接不符: %v", lark["no_refresh_token"])
	}
	none := buildAuthorizationCompleteEvent(&auth.TokenStore{AccessToken: "u"}, buildLoginScopeSummary("", ""), "", "")
	if _, has := none["no_refresh_token"].(map[string]any)["settings_url"]; has {
		t.Fatal("app_id 未知时应省略 settings_url")
	}
}

// auth status：health 值不变；note 改为真因；新增 no_refresh_token（app_id 取当前配置）。
func TestAuthStatusNoRefreshToken(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expiresAt time.Time
		status    string
	}{
		{"未过期", time.Now().Add(time.Hour), "valid"},
		{"已过期", time.Now().Add(-time.Hour), "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeVerifyToken(t, &auth.TokenStore{AccessToken: "u-x", ExpiresAt: tc.expiresAt, AppID: "cli_app"})
			t.Setenv("FEISHU_APP_ID", "cli_app")
			t.Setenv("FEISHU_APP_SECRET", "secret_x")

			stdout, stderr, err := runCLI(t, "auth", "status", "-o", "json")
			if err != nil {
				t.Fatalf("auth status: %v\n%s", err, stderr)
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("stdout 非 JSON: %v", err)
			}
			if out["health"] != "missing_refresh_token" || out["refresh_token_present"] != false || out["token_status"] != tc.status {
				t.Fatalf("health/字段变化: %v", out)
			}
			note, _ := out["note"].(string)
			if tc.status == "valid" && (strings.Contains(note, "未开通 offline_access") || !strings.Contains(note, "刷新 user_access_token")) {
				t.Fatalf("note 归因不符: %q", note)
			}
			info, _ := out["no_refresh_token"].(map[string]any)
			if info == nil || info["settings_url"] != "https://open.feishu.cn/app/cli_app/safe" {
				t.Fatalf("no_refresh_token 不符: %v", out["no_refresh_token"])
			}

			human, _, err := runCLI(t, "auth", "status")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(human, "offline_access）") || !strings.Contains(human, "https://open.feishu.cn/app/cli_app/safe") {
				t.Fatalf("文本输出不符:\n%s", human)
			}
		})
	}
}

func TestDoctorUserTokenExpiredWithoutRefresh(t *testing.T) {
	writeVerifyToken(t, &auth.TokenStore{AccessToken: "u-x", ExpiresAt: time.Now().Add(-time.Hour), AppID: "cli_app"})
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	for _, r := range []checkResult{checkUserToken(), checkUserIdentity()} {
		raw, _ := json.Marshal(r)
		s := string(raw)
		if strings.Contains(s, "均已过期") || !strings.Contains(s, "未下发 refresh_token") || !strings.Contains(s, "/app/cli_app/safe") {
			t.Fatalf("doctor 文案不符: %s", s)
		}
	}
}
