package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/client"
)

// stubPermissionGrant 替换当前用户解析与授权调用，并捕获 stderr 告警。
func stubPermissionGrant(t *testing.T, openID, reason string, grantErr error) (*[]string, *bytes.Buffer) {
	t.Helper()
	var calls []string
	var stderr bytes.Buffer
	oldUser, oldGrant, oldErr := currentCLIUserOpenIDFunc, grantFullAccessFunc, permissionGrantStderr
	currentCLIUserOpenIDFunc = func() (string, string) {
		calls = append(calls, "whoami")
		return openID, reason
	}
	grantFullAccessFunc = func(token, resourceType, uid string) error {
		calls = append(calls, fmt.Sprintf("grant %s %s %s", token, resourceType, uid))
		return grantErr
	}
	permissionGrantStderr = &stderr
	t.Cleanup(func() {
		currentCLIUserOpenIDFunc, grantFullAccessFunc, permissionGrantStderr = oldUser, oldGrant, oldErr
	})
	return &calls, &stderr
}

func TestAutoGrantCurrentUser_UserIdentityNeverTriggers(t *testing.T) {
	calls, _ := stubPermissionGrant(t, "ou_me", "", nil)
	if got := autoGrantCurrentUser("u-token", "DocA", "docx"); got != nil {
		t.Fatalf("User 身份创建不应触发自动授权，得到 %+v", got)
	}
	if len(*calls) != 0 {
		t.Fatalf("User 身份不应解析当前用户或发起授权: %v", *calls)
	}
}

func TestAutoGrantCurrentUser_BotGranted(t *testing.T) {
	calls, stderr := stubPermissionGrant(t, "ou_me", "", nil)
	got := autoGrantCurrentUser("", "WikA", "wiki")
	if got == nil || got.Status != client.PermissionGrantGranted || got.UserOpenID != "ou_me" || got.MemberType != "openid" || got.Perm != "full_access" {
		t.Fatalf("结果不符: %+v", got)
	}
	if strings.Join(*calls, "|") != "whoami|grant WikA wiki ou_me" {
		t.Fatalf("调用序列不符: %v", *calls)
	}
	if stderr.Len() != 0 {
		t.Fatalf("授权成功不应告警: %s", stderr.String())
	}
}

func TestAutoGrantCurrentUser_NoLoginSkipped(t *testing.T) {
	calls, stderr := stubPermissionGrant(t, "", "未找到当前 CLI 登录用户（未登录）", nil)
	got := autoGrantCurrentUser("", "DocA", "docx")
	if got == nil || got.Status != client.PermissionGrantSkipped || got.UserOpenID != "" || !strings.Contains(got.Hint, "auth login") {
		t.Fatalf("结果不符: %+v", got)
	}
	if strings.Join(*calls, "|") != "whoami" {
		t.Fatalf("无登录用户时不应发起授权: %v", *calls)
	}
	if !strings.Contains(stderr.String(), "未自动授予 full_access") {
		t.Fatalf("应在 stderr 告警: %q", stderr.String())
	}
}

func TestAutoGrantCurrentUser_FailureOnlyWarns(t *testing.T) {
	_, stderr := stubPermissionGrant(t, "ou_me", "", &client.PermissionGrantError{Code: 1063002, Msg: "permission denied"})
	got := autoGrantCurrentUser("", "DocA", "docx")
	if got == nil || got.Status != client.PermissionGrantFailed || got.LarkCode != 1063002 || got.UserOpenID != "ou_me" {
		t.Fatalf("结果不符: %+v", got)
	}
	if !strings.Contains(stderr.String(), "授予 full_access 失败") {
		t.Fatalf("应在 stderr 告警: %q", stderr.String())
	}
	// 非业务错误同样只告警
	_, _ = stubPermissionGrant(t, "ou_me", "", errors.New("network down"))
	if got := autoGrantCurrentUser("", "DocA", "docx"); got == nil || got.Status != client.PermissionGrantFailed || got.LarkCode != 0 {
		t.Fatalf("结果不符: %+v", got)
	}
}

func TestAutoGrantCurrentUser_MissingTargetSkipped(t *testing.T) {
	calls, _ := stubPermissionGrant(t, "ou_me", "", nil)
	if got := autoGrantCurrentUser("", "", "docx"); got == nil || got.Status != client.PermissionGrantSkipped {
		t.Fatalf("缺少 token 应 skipped，得到 %+v", got)
	}
	if len(*calls) != 0 {
		t.Fatalf("缺少 token 时不应解析用户或授权: %v", *calls)
	}
}

// TestCurrentCLIUserOpenID 不强制登录：未登录跳过；已登录且缓存匹配时不发网络请求。
func TestCurrentCLIUserOpenID(t *testing.T) {
	initWikiNodeDeleteTestConfig(t, "http://127.0.0.1:1")
	if id, reason := currentCLIUserOpenID(); id != "" || !strings.Contains(reason, "未登录") {
		t.Fatalf("未登录时应返回空 open_id 与原因，得到 %q %q", id, reason)
	}

	tok := &auth.TokenStore{
		AccessToken:      "u-cached-token",
		RefreshToken:     "r-token",
		ExpiresAt:        time.Now().Add(2 * time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            "cli_test",
	}
	if err := auth.SaveToken(tok); err != nil {
		t.Fatalf("写 token.json 失败: %v", err)
	}
	if err := auth.SaveCurrentUserCache(&auth.CurrentUserCache{
		OpenID: "ou_cached", TokenFingerprint: auth.UserTokenFingerprint("u-cached-token"),
	}); err != nil {
		t.Fatalf("写用户缓存失败: %v", err)
	}
	if id, reason := currentCLIUserOpenID(); id != "ou_cached" {
		t.Fatalf("应命中缓存 open_id（base_url 指向不可达地址，任何网络请求都会失败），得到 %q %q", id, reason)
	}

	// token 绑定其他应用：open_id 按应用隔离，不能拿来授权
	tok.AppID = "cli_other"
	if err := auth.SaveToken(tok); err != nil {
		t.Fatal(err)
	}
	if id, reason := currentCLIUserOpenID(); id != "" || !strings.Contains(reason, "无法使用") {
		t.Fatalf("token 绑定其他应用时应跳过，得到 %q %q", id, reason)
	}
}

// TestDocCreate_BotAutoGrantsCurrentUser 端到端：Bot 建文档后向权限接口发起授权，JSON 带 permission_grant。
func TestDocCreate_BotAutoGrantsCurrentUser(t *testing.T) {
	var grantPath, grantType string
	var grantBody map[string]any
	server, _ := newResourceTestServer(t, "docx", "x", func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/docx/v1/documents":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"DocNew1","title":"T","revision_id":1}}}`)
			return true
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/permissions/"):
			grantPath, grantType = r.URL.Path, r.URL.Query().Get("type")
			_ = json.NewDecoder(r.Body).Decode(&grantBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)
	oldUser := currentCLIUserOpenIDFunc
	currentCLIUserOpenIDFunc = func() (string, string) { return "ou_me", "" }
	t.Cleanup(func() { currentCLIUserOpenIDFunc = oldUser })

	_ = createDocumentCmd.Flags().Set("title", "T")
	_ = createDocumentCmd.Flags().Set("output", "json")
	defer resetCmdFlag(createDocumentCmd, "title", "output")
	out, err := captureCmdStdout(t, func() error { return createDocumentCmd.RunE(createDocumentCmd, nil) })
	if err != nil {
		t.Fatalf("doc create 失败: %v", err)
	}
	if grantPath != "/open-apis/drive/v1/permissions/DocNew1/members" || grantType != "docx" || grantBody["member_id"] != "ou_me" {
		t.Fatalf("授权请求不符: path=%q type=%q body=%v", grantPath, grantType, grantBody)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("输出不是 JSON: %s", out)
	}
	pg, _ := result["permission_grant"].(map[string]any)
	if pg["status"] != "granted" || pg["user_open_id"] != "ou_me" || result["url"] != "https://www.feishu.cn/docx/DocNew1" {
		t.Fatalf("JSON 输出不符: %s", out)
	}
}

// TestDocCreate_UserTokenSkipsGrant 显式 User 身份创建：资源属于用户本人，不发授权请求、不输出 permission_grant。
func TestDocCreate_UserTokenSkipsGrant(t *testing.T) {
	granted := false
	server, _ := newResourceTestServer(t, "docx", "x", func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/docx/v1/documents":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"DocNew2","title":"T","revision_id":1}}}`)
			return true
		case strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/permissions/"):
			granted = true
			_, _ = fmt.Fprint(w, `{"code":0}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = createDocumentCmd.Flags().Set("title", "T")
	_ = createDocumentCmd.Flags().Set("output", "json")
	_ = createDocumentCmd.Flags().Set("user-access-token", "u-explicit")
	defer resetCmdFlag(createDocumentCmd, "title", "output", "user-access-token")
	out, err := captureCmdStdout(t, func() error { return createDocumentCmd.RunE(createDocumentCmd, nil) })
	if err != nil {
		t.Fatalf("doc create 失败: %v", err)
	}
	if granted || strings.Contains(out, "permission_grant") {
		t.Fatalf("User 身份创建不应自动授权: granted=%v out=%s", granted, out)
	}
}

// TestDriveImportDryRun_NoGrant dry-run 不解析身份、不触发授权。
func TestDriveImportDryRun_NoGrant(t *testing.T) {
	calls, _ := stubPermissionGrant(t, "ou_me", "", nil)
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = driveImportCmd.Flags().Set("file", path)
	_ = driveImportCmd.Flags().Set("type", "docx")
	_ = driveImportCmd.Flags().Set("as", "bot")
	_ = driveImportCmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(driveImportCmd, "file", "type", "as", "dry-run")
	if _, err := captureCmdStdout(t, func() error { return driveImportCmd.RunE(driveImportCmd, nil) }); err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("dry-run 不应触发自动授权: %v", *calls)
	}
}
