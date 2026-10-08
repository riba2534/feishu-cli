package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
)

// writeVerifyToken 在临时 HOME 的旧布局目录写入 token.json，返回文件路径。
func writeVerifyToken(t *testing.T, store *auth.TokenStore) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	dir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "token.json")
	raw, _ := json.Marshal(store)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubVerifyUserInfo(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := verifyUserInfoFn
	verifyUserInfoFn = fn
	t.Cleanup(func() { verifyUserInfoFn = orig })
}

// 未绑定 app_id 的旧 token：--verify 不得发起刷新，也不得把它静默绑定到当前应用。
func TestVerifyStoredUserToken_UnboundTokenFailsClosed(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "u-new", "refresh_token": "r-new", "expires_in": 7200})
	}))
	t.Cleanup(srv.Close)
	var userInfoCalls int32
	stubVerifyUserInfo(t, func(string) error { atomic.AddInt32(&userInfoCalls, 1); return nil })

	for _, accessValid := range []bool{false, true} {
		expires := time.Now().Add(-time.Hour)
		if accessValid {
			expires = time.Now().Add(time.Hour)
		}
		snapshot := &auth.TokenStore{
			AccessToken:      "u-legacy",
			RefreshToken:     "r-legacy",
			ExpiresAt:        expires,
			RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		}
		path := writeVerifyToken(t, snapshot)

		fresh, ok, msg := verifyStoredUserToken(snapshot, "cli_app", "sec", srv.URL)
		if ok || fresh != nil {
			t.Fatalf("accessValid=%v: 未绑定 token 校验应失败", accessValid)
		}
		if !strings.Contains(msg, "未绑定") || !strings.Contains(msg, "--bind-legacy-app") {
			t.Fatalf("accessValid=%v: 应提示绑定: %s", accessValid, msg)
		}
		loaded, err := auth.LoadTokenFrom(path)
		if err != nil || loaded == nil {
			t.Fatalf("读 token 失败: %v", err)
		}
		if loaded.AppID != "" || loaded.AccessToken != "u-legacy" {
			t.Fatalf("accessValid=%v: token.json 不得被静默绑定或改写: %+v", accessValid, loaded)
		}
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("未绑定 token 不得发起刷新请求，实际 %d 次", hits)
	}
	if atomic.LoadInt32(&userInfoCalls) != 0 {
		t.Fatalf("未绑定 token 不得用于在线校验，实际 %d 次", userInfoCalls)
	}
}

// 并发 --verify（同一过期快照）只允许一次刷新请求：走加锁 + 代际校验路径，不重复消耗 refresh_token。
func TestVerifyStoredUserToken_ConcurrentRefreshOnce(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(80 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":             "u-fresh",
			"refresh_token":            "r-fresh",
			"expires_in":               7200,
			"refresh_token_expires_in": 86400,
		})
	}))
	t.Cleanup(srv.Close)
	var seen sync.Map
	stubVerifyUserInfo(t, func(tok string) error { seen.Store(tok, true); return nil })

	snapshot := auth.TokenStore{
		AccessToken:      "u-stale",
		RefreshToken:     "r-shared",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            "cli_app",
	}
	path := writeVerifyToken(t, &snapshot)

	const n = 8
	var wg sync.WaitGroup
	oks := make([]bool, n)
	msgs := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snap := snapshot // 每个"进程"各自持有加锁前读到的同一份旧快照
			_, oks[i], msgs[i] = verifyStoredUserToken(&snap, "cli_app", "sec", srv.URL)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("并发 --verify 应只刷新一次，实际 %d 次", got)
	}
	for i := 0; i < n; i++ {
		if !oks[i] {
			t.Fatalf("goroutine %d 校验失败: %s", i, msgs[i])
		}
	}
	if _, ok := seen.Load("u-fresh"); !ok {
		t.Fatal("在线校验应使用刷新后的 access_token")
	}
	if _, ok := seen.Load("u-stale"); ok {
		t.Fatal("不得用过期 access_token 在线校验")
	}
	loaded, _ := auth.LoadTokenFrom(path)
	if loaded == nil || loaded.AccessToken != "u-fresh" || loaded.RefreshToken != "r-fresh" || loaded.AppID != "cli_app" {
		t.Fatalf("落盘 token 不正确: %+v", loaded)
	}
}

// 端到端：auth status --verify -o json 展示刷新后的 token 状态，并与业务命令共用同一刷新路径。
func TestAuthStatusVerify_ShowsRefreshedToken(t *testing.T) {
	var refreshHits, userInfoHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch {
		case strings.HasSuffix(r.URL.Path, "/authen/v2/oauth/token"):
			atomic.AddInt32(&refreshHits, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "u-fresh-e2e-token", "refresh_token": "r-fresh", "expires_in": 7200})
		case strings.HasSuffix(r.URL.Path, "/authen/v1/user_info"):
			atomic.AddInt32(&userInfoHits, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer u-fresh-e2e-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":99991668,"msg":"Invalid access token"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"open_id":"ou_x","name":"tester"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	path := writeVerifyToken(t, &auth.TokenStore{
		AccessToken:      "u-stale-e2e-token",
		RefreshToken:     "r-old",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            "cli_app",
	})
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	t.Setenv("FEISHU_BASE_URL", srv.URL)
	config.SetBotFlagCredentials("", "")

	stdout, stderr, err := runCLI(t, "auth", "status", "--verify", "-o", "json")
	if err != nil {
		t.Fatalf("auth status --verify: %v\n%s", err, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 非 JSON: %v\n%s", err, stdout)
	}
	if out["verified"] != true {
		t.Fatalf("verified 应为 true: %v (verify_error=%v)", out["verified"], out["verify_error"])
	}
	if out["token_status"] != "valid" || out["access_token_valid"] != true {
		t.Fatalf("应展示刷新后的状态: token_status=%v access_token_valid=%v", out["token_status"], out["access_token_valid"])
	}
	if refreshHits != 1 || userInfoHits != 1 {
		t.Fatalf("refresh=%d user_info=%d", refreshHits, userInfoHits)
	}
	loaded, _ := auth.LoadTokenFrom(path)
	if loaded == nil || loaded.AccessToken != "u-fresh-e2e-token" || loaded.AppID != "cli_app" {
		t.Fatalf("落盘 token 不正确: %+v", loaded)
	}
}

// 时间字段必须是带真实时区的 RFC3339，不能把本地钟面时间硬标成 +08:00（TZ=UTC 时会偏 8 小时）。
func TestAuthTimeFieldsAreRFC3339(t *testing.T) {
	expires := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	refreshExpires := expires.Add(30 * 24 * time.Hour)
	writeVerifyToken(t, &auth.TokenStore{
		AccessToken:      "u-time-check-token",
		RefreshToken:     "r-time",
		ExpiresAt:        expires,
		RefreshExpiresAt: refreshExpires,
		AppID:            "cli_app",
	})
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")

	stdout, stderr, err := runCLI(t, "auth", "status", "-o", "json")
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 非 JSON: %v\n%s", err, stdout)
	}
	for field, want := range map[string]time.Time{"expires_at": expires, "refresh_expires_at": refreshExpires} {
		raw, _ := out[field].(string)
		got, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatalf("%s=%q 不是 RFC3339: %v", field, raw, err)
		}
		if !got.Equal(want) {
			t.Fatalf("%s=%q 表示的时刻与 token 不一致（期望 %s）", field, raw, want.Format(time.RFC3339))
		}
	}

	event := buildAuthorizationCompleteEvent(&auth.TokenStore{
		AccessToken:      "u",
		RefreshToken:     "r",
		ExpiresAt:        expires,
		RefreshExpiresAt: refreshExpires,
	}, buildLoginScopeSummary("", ""))
	for field, want := range map[string]time.Time{"expires_at": expires, "refresh_expires_at": refreshExpires} {
		got, err := time.Parse(time.RFC3339, event[field].(string))
		if err != nil || !got.Equal(want) {
			t.Fatalf("authorization_complete.%s=%v 与 token 时刻不一致: %v", field, event[field], err)
		}
	}
}

// 终态刷新失败标记要在 auth status / auth check 中体现为"需要重新登录"。
func TestAuthStatusShowsRefreshFailureMarker(t *testing.T) {
	writeVerifyToken(t, &auth.TokenStore{
		AccessToken:      "u-expired-marker",
		RefreshToken:     "r-revoked",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            "cli_app",
		RefreshFailure:   &auth.RefreshFailure{Code: 20064, Error: "invalid_grant", Description: "revoked", At: time.Now()},
	})
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
	if out["health"] != "needs_relogin" || out["token_status"] != "expired" || out["refresh_token_valid"] != false {
		t.Fatalf("终态标记应判为需重新登录: health=%v token_status=%v refresh_token_valid=%v", out["health"], out["token_status"], out["refresh_token_valid"])
	}
	failure, _ := out["refresh_failure"].(map[string]any)
	if failure == nil || failure["code"] != float64(20064) {
		t.Fatalf("应输出 refresh_failure: %v", out["refresh_failure"])
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "auth login") {
		t.Fatalf("note 应提示重新登录: %v", out["note"])
	}

	result, ok := performAuthCheck([]string{"search:docs:read"})
	if ok || result["error"] != "token_expired" {
		t.Fatalf("auth check 应报 token_expired: %v", result)
	}
}
