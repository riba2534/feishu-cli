package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
)

func TestAuthToken_AsBotConflictsWithUserAccessToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	t.Setenv("FEISHU_BASE_URL", "https://open.feishu.cn")
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0700); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runCLI(t, "auth", "token", "--as", "bot", "--user-access-token", "u-explicit")
	if err == nil {
		t.Fatal("应拒绝 --as bot 与 --user-access-token 同时使用")
	}
	msg := err.Error() + stderr
	if !strings.Contains(msg, "--as bot") || !strings.Contains(msg, "--user-access-token") {
		t.Fatalf("错误应说明冲突: %s", msg)
	}
}

func TestAuthToken_AsBotUsesOAuthV3ClientCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	t.Setenv("FEISHU_BASE_URL", "https://open.feishu.cn")
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0700); err != nil {
		t.Fatal(err)
	}

	var (
		gotMethod string
		gotCT     string
		gotBody   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-bot-token","token_type":"Bearer","expires_in":7200}`))
	}))
	t.Cleanup(srv.Close)

	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return srv.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	stdout, _, err := runCLI(t, "auth", "token", "--as", "bot")
	if err != nil {
		t.Fatalf("auth token --as bot: %v", err)
	}
	if strings.TrimSpace(stdout) != "t-bot-token" {
		t.Fatalf("stdout = %q", stdout)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	for _, want := range []string{"grant_type=client_credentials", "client_id=cli_app", "client_secret=secret_x"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q: %s", want, gotBody)
		}
	}
}

func TestAuthToken_BindLegacyApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	config.SetBotFlagCredentials("", "")

	dir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store := auth.TokenStore{
		AccessToken:      "u-legacy-token",
		RefreshToken:     "r-legacy",
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	}
	raw, _ := json.MarshalIndent(store, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "token.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runCLI(t, "auth", "token", "--bind-legacy-app", "--as", "user")
	if err != nil {
		t.Fatalf("bind-legacy-app: %v / %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "u-legacy-token" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "cli_app") {
		t.Fatalf("stderr 应提示绑定 app_id: %s", stderr)
	}
	loaded, err := auth.LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AppID != "cli_app" || loaded.AccessToken != "u-legacy-token" {
		t.Fatalf("绑定结果不正确: %+v", loaded)
	}
}

func TestAuthToken_BindLegacyConflictsWithBotAndExplicitUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0700); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "auth", "token", "--bind-legacy-app", "--as", "bot")
	if err == nil || !strings.Contains(err.Error(), "--bind-legacy-app") || !strings.Contains(err.Error(), "--as bot") {
		t.Fatalf("应拒绝 bind + --as bot: %v", err)
	}
	_, _, err = runCLI(t, "auth", "token", "--bind-legacy-app", "--user-access-token", "u-x", "--as", "user")
	if err == nil || !strings.Contains(err.Error(), "--user-access-token") {
		t.Fatalf("应拒绝 bind + --user-access-token: %v", err)
	}
}

func TestAuthToken_AutoFailClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	config.SetBotFlagCredentials("", "")
	dir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	var tatHits int
	tat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tatHits++
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-should-not-fallback","expires_in":7200}`))
	}))
	t.Cleanup(tat.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return tat.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	t.Run("mismatch", func(t *testing.T) {
		tatHits = 0
		raw, _ := json.Marshal(auth.TokenStore{
			AccessToken: "u-other",
			ExpiresAt:   time.Now().Add(time.Hour),
			AppID:       "cli_other",
		})
		if err := os.WriteFile(filepath.Join(dir, "token.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := runCLI(t, "auth", "token", "--as", "auto")
		if err == nil || !strings.Contains(err.Error(), "不一致") {
			t.Fatalf("App mismatch 应 fail closed: %v", err)
		}
		if tatHits != 0 {
			t.Fatalf("不得静默回退 Bot，TAT hits=%d", tatHits)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		tatHits = 0
		if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte("{not json"), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := runCLI(t, "auth", "token", "--as", "auto")
		if err == nil || !strings.Contains(err.Error(), "解析") {
			t.Fatalf("损坏 token.json 应 fail closed: %v", err)
		}
		if tatHits != 0 {
			t.Fatalf("不得静默回退 Bot，TAT hits=%d", tatHits)
		}
	})

	t.Run("refresh 4xx", func(t *testing.T) {
		tatHits = 0
		refresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh_token expired"}`))
		}))
		t.Cleanup(refresh.Close)
		t.Setenv("FEISHU_BASE_URL", refresh.URL)
		raw, _ := json.Marshal(auth.TokenStore{
			AccessToken:      "u-stale",
			RefreshToken:     "r-stale",
			ExpiresAt:        time.Now().Add(-time.Hour),
			RefreshExpiresAt: time.Now().Add(24 * time.Hour),
			AppID:            "cli_app",
		})
		if err := os.WriteFile(filepath.Join(dir, "token.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := runCLI(t, "auth", "token", "--as", "auto")
		if err == nil {
			t.Fatal("刷新 4xx 应 fail closed")
		}
		if tatHits != 0 {
			t.Fatalf("不得静默回退 Bot，TAT hits=%d", tatHits)
		}
	})

	t.Run("natural absence", func(t *testing.T) {
		tatHits = 0
		_ = os.Remove(filepath.Join(dir, "token.json"))
		t.Setenv("FEISHU_BASE_URL", "https://open.feishu.cn")
		stdout, _, err := runCLI(t, "auth", "token", "--as", "auto")
		if err != nil {
			t.Fatalf("自然缺失应回退 Bot: %v", err)
		}
		if strings.TrimSpace(stdout) != "t-should-not-fallback" {
			t.Fatalf("stdout=%q", stdout)
		}
		if tatHits != 1 {
			t.Fatalf("TAT hits=%d", tatHits)
		}
	})
}
