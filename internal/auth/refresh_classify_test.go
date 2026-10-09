package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// scriptedTokenServer 按顺序返回预设响应；step.hangup=true 时读完请求后直接断开连接（请求已发出、无响应）。
type tokenStep struct {
	status int
	body   string
	hangup bool
}

func newScriptedTokenServer(t *testing.T, steps []tokenStep) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1))
		_, _ = io.ReadAll(r.Body)
		step := steps[len(steps)-1]
		if n <= len(steps) {
			step = steps[n-1]
		}
		if step.hangup {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server 不支持 hijack")
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if step.status != 0 {
			w.WriteHeader(step.status)
		}
		_, _ = w.Write([]byte(step.body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fastRefreshRetry(t *testing.T) {
	t.Helper()
	orig := refreshRetryDelay
	refreshRetryDelay = time.Millisecond
	t.Cleanup(func() { refreshRetryDelay = orig })
}

const okRefreshBody = `{"code":0,"access_token":"u-new","refresh_token":"r-new","expires_in":7200,"refresh_token_expires_in":86400}`

func TestRefreshAccessToken_Classification(t *testing.T) {
	fastRefreshRetry(t)
	cases := []struct {
		name          string
		steps         []tokenStep
		wantCalls     int32
		wantOK        bool
		wantTerminal  bool
		wantRetryable bool
		wantUncertain bool
		wantCode      int
		wantText      []string
	}{
		{
			name:      "20050 重试一次后成功",
			steps:     []tokenStep{{status: 400, body: `{"code":20050,"error_description":"retry"}`}, {body: okRefreshBody}},
			wantCalls: 2, wantOK: true,
		},
		{
			name:          "20050 两次后放弃，可重试且非终态",
			steps:         []tokenStep{{status: 400, body: `{"code":20050,"error_description":"retry"}`}},
			wantCalls:     2,
			wantRetryable: true, wantCode: 20050,
			wantText: []string{"code=20050"},
		},
		{
			name:         "20037 随 HTTP 400 下发：先解析业务码，终态不重试",
			steps:        []tokenStep{{status: 400, body: `{"code":20037,"error":"invalid_grant","error_description":"The refresh token passed is invalid or expired."}`}},
			wantCalls:    1,
			wantTerminal: true, wantCode: 20037,
			wantText: []string{"code=20037", "已过期", "auth login"},
		},
		{
			name:         "20064 已吊销",
			steps:        []tokenStep{{status: 400, body: `{"code":20064,"error_description":"revoked"}`}},
			wantCalls:    1,
			wantTerminal: true, wantCode: 20064,
			wantText: []string{"code=20064", "已被吊销"},
		},
		{
			name:         "20073 已被使用",
			steps:        []tokenStep{{status: 400, body: `{"code":20073,"error_description":"reused"}`}},
			wantCalls:    1,
			wantTerminal: true, wantCode: 20073,
			wantText: []string{"code=20073", "已被使用"},
		},
		{
			name:         "20026 旧版格式",
			steps:        []tokenStep{{status: 400, body: `{"code":20026,"error_description":"legacy"}`}},
			wantCalls:    1,
			wantTerminal: true, wantCode: 20026,
		},
		{
			name:      "未知业务码：不重试、非终态（保留 token，交给上层报错）",
			steps:     []tokenStep{{status: 400, body: `{"code":99991543,"error":"invalid_client","error_description":"bad secret"}`}},
			wantCalls: 1, wantCode: 99991543,
			wantText: []string{"code=99991543", "invalid_client"},
		},
		{
			name:          "请求已发出后断开：重试一次，提示 refresh_token 可能已被消耗",
			steps:         []tokenStep{{hangup: true}},
			wantCalls:     2,
			wantRetryable: true, wantUncertain: true,
			wantText: []string{"可能已被服务端消耗"},
		},
		{
			name:         "断开后重试得到 20073：判为终态",
			steps:        []tokenStep{{hangup: true}, {status: 400, body: `{"code":20073,"error_description":"reused"}`}},
			wantCalls:    2,
			wantTerminal: true, wantCode: 20073,
		},
		{
			name:          "5xx 非 JSON：可重试且可能已消耗",
			steps:         []tokenStep{{status: 502, body: `<html>bad gateway</html>`}},
			wantCalls:     2,
			wantRetryable: true, wantUncertain: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := newScriptedTokenServer(t, tc.steps)
			store, err := RefreshAccessToken(&TokenStore{RefreshToken: "r-old", AppID: "aid"}, "aid", "sec", srv.URL)
			if got := atomic.LoadInt32(calls); got != tc.wantCalls {
				t.Fatalf("请求次数 = %d, want %d", got, tc.wantCalls)
			}
			if tc.wantOK {
				if err != nil || store == nil || store.AccessToken != "u-new" {
					t.Fatalf("应刷新成功: store=%+v err=%v", store, err)
				}
				return
			}
			var re *RefreshError
			if !errors.As(err, &re) {
				t.Fatalf("应返回 *RefreshError，得到 %T %v", err, err)
			}
			if re.Terminal != tc.wantTerminal || re.Retryable != tc.wantRetryable || re.Uncertain != tc.wantUncertain || re.Code != tc.wantCode {
				t.Fatalf("分类错误: %+v", re)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("错误文本应包含 %q: %v", want, err)
				}
			}
		})
	}
}

// 请求已发出后断网属于网络类错误（退出码 4），不应被当成需要重新登录。
func TestRefreshAccessToken_TransportErrorTaggedNetwork(t *testing.T) {
	fastRefreshRetry(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // 端口关闭：连接被拒绝（请求未发出）
	_, err = RefreshAccessToken(&TokenStore{RefreshToken: "r"}, "aid", "sec", "http://"+addr)
	if err == nil {
		t.Fatal("应失败")
	}
	if !clierr.HasKind(err, clierr.KindNetwork) {
		t.Fatalf("传输错误应打网络标签: %v", err)
	}
	var re *RefreshError
	if !errors.As(err, &re) || re.Uncertain {
		t.Fatalf("连接被拒绝时请求未发出，不应标记可能已消耗: %+v", re)
	}
}

func TestRefreshAccessToken_StatusMessagePassthrough(t *testing.T) {
	srv, _ := newScriptedTokenServer(t, []tokenStep{{body: `{"code":0,"access_token":"u-new","expires_in":7200,"status_message":"Some scopes were silently trimmed"}`}})
	r, w, _ := os.Pipe()
	orig := os.Stderr
	os.Stderr = w
	_, err := RefreshAccessToken(&TokenStore{RefreshToken: "r"}, "aid", "sec", srv.URL)
	os.Stderr = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Some scopes were silently trimmed") {
		t.Fatalf("stderr 应透传 status_message: %q", out)
	}
}

func TestRefreshAccessToken_DefaultsExpiresIn(t *testing.T) {
	srv, _ := newScriptedTokenServer(t, []tokenStep{{body: `{"code":0,"access_token":"u-new"}`}})
	store, err := RefreshAccessToken(&TokenStore{RefreshToken: "r"}, "aid", "sec", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsAccessTokenValid() {
		t.Fatalf("缺 expires_in 时应按 2 小时默认有效期，而不是立即过期: %v", store.ExpiresAt)
	}
}

// 终态失败写入 token.json 标记：后续命令不再发起刷新请求，直接给出重新登录提示；重新登录后标记消失。
func TestTerminalRefreshFailureMarkerStopsRepeatedRefresh(t *testing.T) {
	fastRefreshRetry(t)
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "token.json")
	tokenPathFunc = func() (string, error) { return tokenFile, nil }
	t.Cleanup(func() { tokenPathFunc = originalTokenPath })
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	srv, calls := newScriptedTokenServer(t, []tokenStep{{status: 400, body: `{"code":20064,"error":"invalid_grant","error_description":"refresh token revoked"}`}})
	if err := SaveToken(&TokenStore{
		AccessToken:      "u-stale",
		RefreshToken:     "r-revoked",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            "aid",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveUserAccessToken("", "", "aid", "sec", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "code=20064") {
		t.Fatalf("首次刷新应失败并带业务码: %v", err)
	}
	if !clierr.HasKind(err, clierr.KindAuth) {
		t.Fatalf("终态失败应为鉴权类错误: %v", err)
	}
	loaded, _ := LoadTokenFrom(tokenFile)
	if loaded == nil || loaded.RefreshFailure == nil || loaded.RefreshFailure.Code != 20064 {
		t.Fatalf("token.json 应记录终态标记: %+v", loaded)
	}
	if loaded.RefreshToken != "r-revoked" || loaded.AccessToken != "u-stale" {
		t.Fatalf("fail-closed：不得删除或改写 token 本体: %+v", loaded)
	}
	if loaded.IsRefreshTokenValid() || loaded.TokenStatus() != "expired" {
		t.Fatalf("有终态标记时 refresh 应视为不可用: valid=%v status=%s", loaded.IsRefreshTokenValid(), loaded.TokenStatus())
	}

	// 第二条命令：不再发起刷新
	for i := 0; i < 3; i++ {
		_, err = ResolveUserAccessToken("", "", "aid", "sec", srv.URL)
		if err == nil || !strings.Contains(err.Error(), "code=20064") || !strings.Contains(err.Error(), "auth login") {
			t.Fatalf("后续命令应直接报终态错误: %v", err)
		}
	}
	if _, err := ForceRefreshLocalToken("aid", "sec", srv.URL); err == nil || !strings.Contains(err.Error(), "code=20064") {
		t.Fatalf("auth refresh 也应直接报终态错误: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("终态后不应重复刷新，实际请求 %d 次", got)
	}

	// 重新登录写入新 token：标记消失
	if err := SaveToken(&TokenStore{AccessToken: "u-relogin", RefreshToken: "r-relogin", ExpiresAt: time.Now().Add(time.Hour), AppID: "aid"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(tokenFile)
	if strings.Contains(string(raw), "refresh_failure") {
		t.Fatalf("重新登录后不应残留终态标记: %s", raw)
	}
	tok, err := ResolveUserAccessToken("", "", "aid", "sec", srv.URL)
	if err != nil || tok != "u-relogin" {
		t.Fatalf("重新登录后应可用: %q %v", tok, err)
	}
}

// 非终态失败（20050 / 断网）不写标记，下次命令仍会尝试刷新。
func TestNonTerminalRefreshFailureKeepsRetrying(t *testing.T) {
	fastRefreshRetry(t)
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "token.json")
	tokenPathFunc = func() (string, error) { return tokenFile, nil }
	t.Cleanup(func() { tokenPathFunc = originalTokenPath })
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	srv, calls := newScriptedTokenServer(t, []tokenStep{{status: 500, body: `{"code":20050,"error_description":"busy"}`}})
	if err := SaveToken(&TokenStore{AccessToken: "u", RefreshToken: "r", ExpiresAt: time.Now().Add(-time.Hour), AppID: "aid"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := ResolveUserAccessToken("", "", "aid", "sec", srv.URL); err == nil {
			t.Fatal("应失败")
		}
	}
	if got := atomic.LoadInt32(calls); got != 4 {
		t.Fatalf("两次命令 × (1 + 1 次重试) = 4 次请求，实际 %d", got)
	}
	loaded, _ := LoadTokenFrom(tokenFile)
	if loaded.RefreshFailure != nil {
		t.Fatalf("非终态失败不应写标记: %+v", loaded.RefreshFailure)
	}
	var raw map[string]any
	data, _ := os.ReadFile(tokenFile)
	_ = json.Unmarshal(data, &raw)
	if _, ok := raw["refresh_failure"]; ok {
		t.Fatal("token.json 不应出现 refresh_failure 字段")
	}
}
