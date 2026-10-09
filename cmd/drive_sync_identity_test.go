package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
)

// 复现：token.json 存在且 access_token 仍有效，但没有 app_id（旧版布局），
// 当前 App 为 cli_test_app。ResolveUserAccessToken 会 fail-closed 返回 ErrUnboundToken，
// resolveOptionalUserTokenWithFallback 吞掉后返回 ""，drive pull 以 Bot 身份列举，
// 列举到空 → --delete-local 直接删掉本地文件。
// TestDrivePull_DeleteLocal_FailsClosedOnUnusableUserToken 验证 --delete-local 在
// 「已配置 User Token 但不可用」时 fail-closed，不得降级为 Bot 身份执行。
//
// 回归防护：身份决定远端文件视图。降级到 Bot 后远端条目更少，差集变大，
// --delete-local 会把用户本地文件当作"远端已不存在"而删除（实测 keep.txt 被删且 err=nil）。
func TestDrivePull_DeleteLocal_FailsClosedOnUnusableUserToken(t *testing.T) {
	var mu sync.Mutex
	var authHeaders []string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()

		if strings.Contains(r.URL.Path, "/tenant_access_token") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "msg": "ok",
				"tenant_access_token": "t-bot",
				"expire":              7200,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			// Bot 身份对该文件夹无可见条目 → 空列表
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "msg": "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files":           []map[string]any{},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	// 1. 隔离 HOME，写入「旧版」token.json（无 app_id，access_token 仍有效）
	tempHome := t.TempDir()
	restoreHome := profile.SetHomeFunc(func() (string, error) { return tempHome, nil })
	defer restoreHome()

	root := filepath.Join(tempHome, ".feishu-cli")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"access_token":       "u-legacy-user-token",
		"refresh_token":      "ur-legacy",
		"token_type":         "Bearer",
		"expires_at":         time.Now().Add(2 * time.Hour).Format(time.RFC3339),
		"refresh_expires_at": time.Now().Add(240 * time.Hour).Format(time.RFC3339),
		"scope":              "drive:drive",
		// 注意：故意不写 app_id
	}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "token.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	config.SetBotFlagCredentials("cli_test_app", "test_secret_123")
	config.ApplyBotFlagCredentials()
	cfg := config.Get()
	origBase, origUAT := cfg.BaseURL, cfg.UserAccessToken
	cfg.BaseURL = mockServer.URL
	cfg.UserAccessToken = ""
	defer func() {
		cfg.BaseURL, cfg.UserAccessToken = origBase, origUAT
		config.SetBotFlagCredentials("", "")
	}()

	// 前置断言：这份 token.json 确实被 fail-closed 拒绝
	if _, err := auth.ResolveUserAccessToken("", "", cfg.AppID, cfg.AppSecret, cfg.BaseURL); err == nil {
		t.Fatalf("预期 ResolveUserAccessToken fail-closed，但成功了")
	} else {
		t.Logf("ResolveUserAccessToken 拒绝: %v", err)
		if auth.IsNoUserTokenConfigured(err) {
			t.Fatalf("不应被判定为「未配置」")
		}
	}

	// 2. cwd 子树内的本地镜像目录，放一个用户文件
	cwd, _ := os.Getwd()
	mirror := filepath.Join(cwd, "zz_repro_mirror")
	if err := os.MkdirAll(mirror, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mirror)
	keep := filepath.Join(mirror, "keep.txt")
	if err := os.WriteFile(keep, []byte("我的重要文件"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. 跑 drive pull --delete-local --yes
	cmd := drivePullCmd
	_ = cmd.Flags().Set("folder-token", "fld_root")
	_ = cmd.Flags().Set("local-dir", mirror)
	_ = cmd.Flags().Set("delete-local", "true")
	_ = cmd.Flags().Set("yes", "true")
	_ = cmd.Flags().Set("output", "json")
	_ = cmd.Flags().Set("user-access-token", "")
	defer func() {
		_ = cmd.Flags().Set("delete-local", "false")
		_ = cmd.Flags().Set("yes", "false")
		_ = cmd.Flags().Set("output", "")
	}()

	err := cmd.RunE(cmd, []string{})
	t.Logf("RunE err = %v", err)

	mu.Lock()
	hdrs := append([]string(nil), authHeaders...)
	mu.Unlock()
	t.Logf("Authorization headers = %v", hdrs)

	if err == nil {
		t.Error("已配置 User Token 不可用时 --delete-local 应报错，不得降级为 Bot 执行")
	}
	if len(hdrs) != 0 {
		t.Errorf("fail-closed 时不应发出任何请求，实际 Authorization headers = %v", hdrs)
	}
	if _, statErr := os.Stat(keep); os.IsNotExist(statErr) {
		t.Errorf("本地文件被误删（静默 Bot 身份 + --delete-local），err=%v", err)
	}
}
