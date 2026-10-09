package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/client"
)

func stubAppScopes(t *testing.T, scopes []client.AppScope) {
	t.Helper()
	orig := getApplicationScopesFn
	getApplicationScopesFn = func(appID string) (*client.ApplicationScopes, error) {
		return &client.ApplicationScopes{AppID: appID, AppName: "测试应用", Scopes: scopes}, nil
	}
	t.Cleanup(func() { getApplicationScopesFn = orig })
}

// auth scopes 区分"应用没开通"与"用户没授权"。
func TestAuthScopes_DiagnosesAppVsUser(t *testing.T) {
	writeVerifyToken(t, &auth.TokenStore{
		AccessToken:      "u-scopes-check",
		RefreshToken:     "r",
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "search:docs:read",
		AppID:            "cli_app",
	})
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	stubAppScopes(t, []client.AppScope{
		{Scope: "search:docs:read", TokenTypes: []string{"user"}},
		{Scope: "search:message", TokenTypes: []string{"user", "tenant"}},
		{Scope: "im:message:send_as_bot", TokenTypes: []string{"tenant"}},
	})

	stdout, stderr, err := runCLI(t, "auth", "scopes", "--scope", "search:docs:read,search:message im:message:send_as_bot okr:okr.period:readonly", "-o", "json")
	if err != nil {
		t.Fatalf("auth scopes: %v\n%s", err, stderr)
	}
	var out struct {
		UserScopes     []string         `json:"user_scopes"`
		TenantScopes   []string         `json:"tenant_scopes"`
		Checks         []map[string]any `json:"checks"`
		AppNotEnabled  []string         `json:"app_not_enabled"`
		UserNotGranted []string         `json:"user_not_granted"`
		Suggestion     string           `json:"suggestion"`
		ConsoleURL     string           `json:"console_url"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 非 JSON: %v\n%s", err, stdout)
	}
	if strings.Join(out.UserScopes, " ") != "search:docs:read search:message" || strings.Join(out.TenantScopes, " ") != "im:message:send_as_bot search:message" {
		t.Fatalf("user=%v tenant=%v", out.UserScopes, out.TenantScopes)
	}
	want := map[string]string{
		"search:docs:read":        "ok",
		"search:message":          "user_not_granted",
		"im:message:send_as_bot":  "tenant_only",
		"okr:okr.period:readonly": "app_not_enabled",
	}
	if len(out.Checks) != len(want) {
		t.Fatalf("checks = %v", out.Checks)
	}
	for _, c := range out.Checks {
		if want[c["scope"].(string)] != c["diagnosis"] {
			t.Fatalf("%v 诊断为 %v，期望 %s", c["scope"], c["diagnosis"], want[c["scope"].(string)])
		}
	}
	if strings.Join(out.UserNotGranted, " ") != "search:message" || out.Suggestion != `feishu-cli auth login --scope "search:message"` {
		t.Fatalf("user_not_granted=%v suggestion=%q", out.UserNotGranted, out.Suggestion)
	}
	if strings.Join(out.AppNotEnabled, " ") != "im:message:send_as_bot okr:okr.period:readonly" || !strings.Contains(out.ConsoleURL, "/app/cli_app/auth?q=") {
		t.Fatalf("app_not_enabled=%v console_url=%q", out.AppNotEnabled, out.ConsoleURL)
	}
}

func TestAuthScopes_NotLoggedIn(t *testing.T) {
	writeVerifyToken(t, &auth.TokenStore{}) // 空 token：无可用 User Token
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	stubAppScopes(t, []client.AppScope{{Scope: "search:docs:read", TokenTypes: []string{"user"}}})

	stdout, _, err := runCLI(t, "auth", "scopes", "--scope", "search:docs:read", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"diagnosis": "not_logged_in"`) {
		t.Fatalf("应诊断为 not_logged_in: %s", stdout)
	}
}
