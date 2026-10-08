package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
)

// writeTestToken 在 tempdir 写入 token.json 并设置 HOME 使 auth.LoadToken 能找到它
func writeTestToken(t *testing.T, token *auth.TokenStore) {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dir := filepath.Join(tmpHome, ".feishu-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if token == nil {
		return
	}
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.json"), data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestPerformAuthCheck_NotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 空 HOME，无 token 文件

	result, ok := performAuthCheck([]string{"search:docs:read"})
	if ok {
		t.Errorf("ok should be false when not logged in")
	}
	if result["error"] != "not_logged_in" {
		t.Errorf("expected error=not_logged_in, got %v", result["error"])
	}
	if result["ok"] != false {
		t.Errorf("expected ok=false, got %v", result["ok"])
	}
	missing, _ := result["missing"].([]string)
	if !reflect.DeepEqual(missing, []string{"search:docs:read"}) {
		t.Errorf("expected missing=[search:docs:read], got %v", missing)
	}
}

func TestPerformAuthCheck_TokenExpired(t *testing.T) {
	past := time.Now().Add(-2 * time.Hour)
	writeTestToken(t, &auth.TokenStore{
		AccessToken:      "expired",
		RefreshToken:     "also-expired",
		ExpiresAt:        past,
		RefreshExpiresAt: past, // 两个都过期
		Scope:            "search:docs:read",
	})

	result, ok := performAuthCheck([]string{"search:docs:read"})
	if ok {
		t.Errorf("ok should be false when both tokens expired")
	}
	if result["error"] != "token_expired" {
		t.Errorf("expected error=token_expired, got %v", result["error"])
	}
}

func TestPerformAuthCheck_RefreshValid(t *testing.T) {
	// access_token 过期但 refresh_token 仍有效 → 应视为可用（后续会自动刷新）
	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(7 * 24 * time.Hour)
	writeTestToken(t, &auth.TokenStore{
		AccessToken:      "expired",
		RefreshToken:     "valid-refresh",
		ExpiresAt:        past,
		RefreshExpiresAt: future,
		Scope:            "search:docs:read im:message:readonly",
	})

	result, ok := performAuthCheck([]string{"search:docs:read"})
	if !ok {
		t.Errorf("ok should be true: access expired but refresh valid, scope matches")
	}
	if result["error"] != nil {
		t.Errorf("expected no error, got %v", result["error"])
	}
}

func TestPerformAuthCheck_AllGranted(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	writeTestToken(t, &auth.TokenStore{
		AccessToken:      "valid",
		RefreshToken:     "valid-refresh",
		ExpiresAt:        future,
		RefreshExpiresAt: future,
		Scope:            "search:docs:read im:message:readonly drive:drive.search:readonly",
	})

	result, ok := performAuthCheck([]string{"search:docs:read", "im:message:readonly"})
	if !ok {
		t.Errorf("ok should be true when all scopes granted")
	}
	missing, _ := result["missing"].([]string)
	if len(missing) != 0 {
		t.Errorf("missing should be empty, got %v", missing)
	}
	granted, _ := result["granted"].([]string)
	if !reflect.DeepEqual(granted, []string{"search:docs:read", "im:message:readonly"}) {
		t.Errorf("granted wrong: %v", granted)
	}
	if _, has := result["suggestion"]; has {
		t.Errorf("should not have suggestion when ok")
	}
}

func TestPerformAuthCheck_PartialMissing(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	writeTestToken(t, &auth.TokenStore{
		AccessToken:      "valid",
		RefreshToken:     "valid-refresh",
		ExpiresAt:        future,
		RefreshExpiresAt: future,
		Scope:            "search:docs:read",
	})

	result, ok := performAuthCheck([]string{"search:docs:read", "im:message:readonly"})
	if ok {
		t.Errorf("ok should be false when one scope is missing")
	}
	missing, _ := result["missing"].([]string)
	if !reflect.DeepEqual(missing, []string{"im:message:readonly"}) {
		t.Errorf("missing should be [im:message:readonly], got %v", missing)
	}
	granted, _ := result["granted"].([]string)
	if !reflect.DeepEqual(granted, []string{"search:docs:read"}) {
		t.Errorf("granted should be [search:docs:read], got %v", granted)
	}
	suggestion, _ := result["suggestion"].(string)
	if suggestion == "" {
		t.Errorf("should have suggestion when missing")
	}
}

// TestAuthCheck_CommaSeparatedScopes：--scope "a,b" 必须识别为两个 scope，
// suggestion 里的 auth login 命令统一空格分隔（服务端拒绝 "a,b" 形式）。
func TestAuthCheck_CommaSeparatedScopes(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	writeTestToken(t, &auth.TokenStore{
		AccessToken:      "valid",
		RefreshToken:     "valid-refresh",
		ExpiresAt:        future,
		RefreshExpiresAt: future,
		Scope:            "search:docs:read",
	})
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")

	stdout, _, err := runCLI(t, "auth", "check", "--scope", "search:docs:read,im:message:readonly, vc:note:read,search:docs:read")
	if err == nil {
		t.Fatal("缺 scope 时应返回非零")
	}
	if got := exitCodeFor(err); got != 3 {
		t.Fatalf("缺 scope 应为鉴权类退出码 3，得到 %d", got)
	}
	var result struct {
		OK         bool     `json:"ok"`
		Granted    []string `json:"granted"`
		Missing    []string `json:"missing"`
		Suggestion string   `json:"suggestion"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout 不是 JSON: %v\n%s", err, stdout)
	}
	if !reflect.DeepEqual(result.Granted, []string{"search:docs:read"}) {
		t.Fatalf("granted = %v", result.Granted)
	}
	if !reflect.DeepEqual(result.Missing, []string{"im:message:readonly", "vc:note:read"}) {
		t.Fatalf("missing = %v（逗号分隔应拆成多个 scope 并去重）", result.Missing)
	}
	if !strings.Contains(result.Suggestion, `--scope "im:message:readonly vc:note:read"`) {
		t.Fatalf("suggestion 应为空格分隔: %s", result.Suggestion)
	}
}

func TestAuthCheck_EmptyScopeIsUsageError(t *testing.T) {
	writeTestToken(t, nil)
	_, _, err := runCLI(t, "auth", "check", "--scope", " , ")
	if err == nil {
		t.Fatal("空 scope 应报错")
	}
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("空 scope 应为用法错误 exit 2，得到 %d", got)
	}
}
