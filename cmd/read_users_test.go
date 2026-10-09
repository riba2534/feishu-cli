package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// startReadUsersStub 假 read_users 服务：记录 Authorization 与 query，返回一条已读记录。
func startReadUsersStub(t *testing.T) (cfg string, seen func() (auth, query string)) {
	t.Helper()
	var mu sync.Mutex
	var lastAuth, lastQuery string
	srv := httptest.NewServer(tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/messages/om_fp_test/read_users" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		mu.Lock()
		lastAuth, lastQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[{"user_id_type":"open_id","user_id":"ou_reader","timestamp":"1790000000000","tenant_key":"tk"}],"has_more":true,"page_token":"next"}}`)
	}))
	t.Cleanup(srv.Close)
	return writeStubConfig(t, srv.URL), func() (string, string) {
		mu.Lock()
		defer mu.Unlock()
		return lastAuth, lastQuery
	}
}

// TestMsgReadUsersIdentity read_users 接口同时接受 User/Tenant：--as user/auto（已登录）带 User Token
// 发起请求（以前 SDK 本地报 "tenant token type not match user access token"），--as bot 用 App Token。
func TestMsgReadUsersIdentity(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		envToken string
		wantAuth string
	}{
		{"auto 已登录走 User", nil, "u-env-token", "Bearer u-env-token"},
		{"显式 user", []string{"--as", "user"}, "u-env-token", "Bearer u-env-token"},
		{"显式 bot 即使已登录也走 App Token", []string{"--as", "bot"}, "u-env-token", "Bearer t-fake"},
		{"auto 未登录回退 Bot", nil, "", "Bearer t-fake"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			if tc.envToken != "" {
				t.Setenv("FEISHU_USER_ACCESS_TOKEN", tc.envToken)
			}
			cfg, seen := startReadUsersStub(t)
			args := append([]string{"msg", "read-users", "om_fp_test", "--page-size", "50", "--page-token", "p1", "-o", "json", "--config", cfg}, tc.args...)
			stdout, stderr, err := runCLI(t, args...)
			if err != nil {
				t.Fatalf("read-users 失败: %v\nstderr=%s", err, stderr)
			}
			auth, query := seen()
			if auth != tc.wantAuth {
				t.Fatalf("Authorization = %q, want %q", auth, tc.wantAuth)
			}
			for _, want := range []string{"user_id_type=open_id", "page_size=50", "page_token=p1"} {
				if !strings.Contains(query, want) {
					t.Fatalf("query = %q, 缺少 %s", query, want)
				}
			}
			var out struct {
				Items []struct {
					UserID string `json:"UserID"`
				} `json:"items"`
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
			}
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
			}
			if len(out.Items) != 1 || out.Items[0].UserID != "ou_reader" || !out.HasMore || out.PageToken != "next" {
				t.Fatalf("输出 = %s", stdout)
			}
		})
	}
}

func TestMsgReadUsersInvalidAsAndUserWithoutToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cfg, seen := startReadUsersStub(t)
	_, _, err := runCLI(t, "msg", "read-users", "om_fp_test", "--as", "nobody", "--config", cfg)
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("非法 --as 应为用法错误 exit 2，实际 %v", err)
	}
	_, _, err = runCLI(t, "msg", "read-users", "om_fp_test", "--as", "user", "--config", cfg)
	if err == nil || exitCodeFor(err) != 3 {
		t.Fatalf("--as user 缺 User Token 应为鉴权错误 exit 3，实际 %v", err)
	}
	if auth, _ := seen(); auth != "" {
		t.Fatalf("身份解析失败时不应发请求，实际 Authorization=%q", auth)
	}
	if !strings.Contains(readUsersCmd.Long, "7 天内") || !strings.Contains(readUsersCmd.Long, "--as bot") {
		t.Fatalf("help 应写明按发送者选身份与 7 天限制:\n%s", readUsersCmd.Long)
	}
}
