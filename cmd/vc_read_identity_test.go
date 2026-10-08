package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// vcReadIdentityCases 会议 / 纪要 / 妙记读命令（#10：新增 --as，默认 user）
func vcReadIdentityCases() [][]string {
	return [][]string{
		{"vc", "search", "--query", "周会", "-o", "json"},
		{"vc", "detail", testVCMeetingID, "-o", "json"},
		{"vc", "recording", "--meeting-ids", testVCMeetingID, "-o", "json"},
		{"vc", "notes", "--meeting-ids", testVCMeetingID, "-o", "json"},
		{"vc", "note", "detail", testNoteID},
		{"minutes", "search", "--query", "周会", "-o", "json"},
		{"minutes", "get", "obcnxxxxxx", "-o", "json"},
		{"minutes", "download", "--minute-tokens", "obcnxxxxxx", "--url-only"},
		{"minutes", "apply-permission", "--minute-token", "obcnxxxxxx", "--perm", "view", "-o", "json"},
	}
}

// startVCIdentityServer 起一个记录业务请求 Authorization 的假飞书服务，返回配置文件路径与采集结果。
func startVCIdentityServer(t *testing.T) (cfgPath string, auths func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		mu.Lock()
		got = append(got, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false,`+
			`"meeting":{"id":"`+testVCMeetingID+`","topic":"t","start_time":"1790000000","end_time":"1790003600"},`+
			`"recording":{"url":"https://example.feishu.cn/minutes/obcnxxxxxx"},`+
			`"minute":{"title":"t","token":"obcnxxxxxx"},"note":{"note_display_type":1},`+
			`"download_url":"https://example.com/media.mp4"}}`)
	}))
	t.Cleanup(srv.Close)
	cfgPath = filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// TestVCReadCommandsAsIdentity --as bot 走 Tenant Token（即使已登录）；默认（user）走 User Token。
func TestVCReadCommandsAsIdentity(t *testing.T) {
	for _, base := range vcReadIdentityCases() {
		name := strings.Join(base[:2], " ")
		t.Run(name+" --as bot", func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
			cfg, auths := startVCIdentityServer(t)
			args := append(append([]string{}, base...), "--as", "bot", "--config", cfg)
			stdout, stderr, err := runCLI(t, args...)
			if err != nil {
				t.Fatalf("%v 失败: %v\nstdout=%s\nstderr=%s", args, err, stdout, stderr)
			}
			got := auths()
			if len(got) == 0 {
				t.Fatalf("未发出业务请求")
			}
			for _, a := range got {
				if !strings.HasSuffix(a, " "+testTenantAuth) {
					t.Fatalf("--as bot 应使用 Tenant Token，实际 %v", got)
				}
			}
		})
		t.Run(name+" 默认 user", func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
			cfg, auths := startVCIdentityServer(t)
			args := append(append([]string{}, base...), "--config", cfg)
			if _, stderr, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v 失败: %v\nstderr=%s", args, err, stderr)
			}
			for _, a := range auths() {
				if !strings.HasSuffix(a, " Bearer u-env-token") {
					t.Fatalf("默认应使用 User Token，实际 %v", auths())
				}
			}
		})
	}
}

// TestVCReadCommandsDefaultUserRequiresToken 默认 --as user 未登录时报错且不发业务请求（不静默切 Bot）。
func TestVCReadCommandsDefaultUserRequiresToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cfg, auths := startVCIdentityServer(t)
	_, _, err := runCLI(t, "vc", "search", "--query", "周会", "--config", cfg)
	if err == nil || !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("未登录默认 user 应报错并提示 --as，实际: %v", err)
	}
	if got := auths(); len(got) != 0 {
		t.Fatalf("不应发业务请求: %v", got)
	}
}
