package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// withAppsNoCredentials 清空 App 凭证（模拟无配置 HOME），返回记录请求的 mock。
func withAppsNoCredentials(t *testing.T) *slidesMockServer {
	t.Helper()
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	cfg := config.Get()
	origID, origSecret := cfg.AppID, cfg.AppSecret
	cfg.AppID, cfg.AppSecret = "", ""
	t.Cleanup(func() { cfg.AppID, cfg.AppSecret = origID, origSecret })
	return m
}

func writeAppsIconFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(p, []byte("\x89PNG\r\n\x1a\n0000000000000000"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestApps_DryRunWithoutCredentials 所有 apps 写命令（及只读命令）的 --dry-run 在无凭据时 exit 0 且不联网；
// 去掉 --dry-run 后仍是鉴权类错误（exit 3）。
func TestApps_DryRunWithoutCredentials(t *testing.T) {
	m := withAppsNoCredentials(t)
	site := writeAppsIndexFixture(t, "<h1>hi</h1>")
	icon := writeAppsIconFixture(t)
	cases := []struct {
		name  string
		cmd   *cobra.Command
		flags map[string]string
	}{
		{"update", appsUpdateCmd, map[string]string{"app-id": "app_x", "name": "n"}},
		{"update-icon-file", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-file": icon}},
		{"access-scope-set", appsAccessScopeSetCmd, map[string]string{"app-id": "app_x", "scope": "tenant"}},
		{"access-scope-get", appsAccessScopeGetCmd, map[string]string{"app-id": "app_x"}},
		{"html-publish", appsHTMLPublishCmd, map[string]string{"app-id": "app_x", "path": site}},
		{"create", appsCreateCmd, map[string]string{"name": "n", "app-type": "html"}},
		{"delete", appsDeleteCmd, map[string]string{"app-id": "app_x", "confirm-name": "n"}},
		{"release-create", appsReleaseCreateCmd, map[string]string{"app-id": "app_x", "apply-reason": "r"}},
		{"features", appsFeaturesCmd, map[string]string{"app-id": "app_x"}},
		{"list", appsListCmd, map[string]string{"owner-id": "ou_x"}},
	}
	for _, tc := range cases {
		dry := map[string]string{"dry-run": "true"}
		for k, v := range tc.flags {
			dry[k] = v
		}
		out, err := runSlidesCmd(t, tc.cmd, nil, dry)
		if err != nil {
			t.Fatalf("%s --dry-run 无凭据应成功: %v", tc.name, err)
		}
		if !strings.Contains(out, `"dry_run": true`) {
			t.Fatalf("%s --dry-run 输出缺 dry_run:\n%s", tc.name, out)
		}
		_, err = runSlidesCmd(t, tc.cmd, nil, tc.flags)
		if err == nil || exitCodeFor(err) != clierr.ExitAuth {
			t.Fatalf("%s 去掉 --dry-run 无凭据应 exit 3，got %d: %v", tc.name, exitCodeFor(err), err)
		}
	}
	if len(m.requests) != 0 {
		t.Fatalf("无凭据不应发请求，实际 %d 次", len(m.requests))
	}
}

// TestApps_UsageErrorsExit2 参数错误一律 exit 2（即使无凭据也先报参数错误）。
func TestApps_UsageErrorsExit2(t *testing.T) {
	m := withAppsNoCredentials(t)
	icon := writeAppsIconFixture(t)
	notImage := writeAppsIndexFixture(t, "<h1>hi</h1>")
	cases := []struct {
		name  string
		cmd   *cobra.Command
		flags map[string]string
	}{
		{"update-no-field", appsUpdateCmd, map[string]string{"app-id": "app_x"}},
		{"update-no-app-id", appsUpdateCmd, map[string]string{"name": "n"}},
		{"update-icon-both", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-url": "https://example.com/a.png", "icon-file": icon}},
		{"update-icon-url-scheme", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-url": "ftp://example.com/a.png"}},
		{"update-icon-url-userinfo", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-url": "https://u:p@example.com/a.png"}},
		{"update-icon-not-image", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-file": filepath.Join(notImage, "index.html")}},
		{"update-icon-missing", appsUpdateCmd, map[string]string{"app-id": "app_x", "icon-file": filepath.Join(notImage, "nope.png")}},
		{"access-scope-set-bad-scope", appsAccessScopeSetCmd, map[string]string{"app-id": "app_x", "scope": "bad"}},
		{"access-scope-set-no-app", appsAccessScopeSetCmd, map[string]string{"scope": "tenant"}},
		{"access-scope-set-empty-targets", appsAccessScopeSetCmd, map[string]string{"app-id": "app_x", "scope": "specific", "targets": "[]"}},
		{"access-scope-get-no-app", appsAccessScopeGetCmd, map[string]string{}},
		{"html-publish-no-path", appsHTMLPublishCmd, map[string]string{"app-id": "app_x"}},
		{"list-owner-blank", appsListCmd, map[string]string{"owner-id": ""}},
		{"list-owner-bad", appsListCmd, map[string]string{"owner-id": "abc"}},
		{"delete-no-confirm", appsDeleteCmd, map[string]string{"app-id": "app_x"}},
		{"delete-no-app", appsDeleteCmd, map[string]string{"confirm-name": "n"}},
		{"release-create-meta", appsReleaseCreateCmd, map[string]string{"app-id": "meta_x"}},
		{"release-create-blank-reason", appsReleaseCreateCmd, map[string]string{"app-id": "app_x", "apply-reason": "  "}},
		{"release-create-long-reason", appsReleaseCreateCmd, map[string]string{"app-id": "app_x", "apply-reason": strings.Repeat("理", 1001)}},
		{"release-create-bidi", appsReleaseCreateCmd, map[string]string{"app-id": "app_x", "apply-reason": "a\u202eb"}},
		{"features-meta", appsFeaturesCmd, map[string]string{"app-id": "meta_x"}},
	}
	for _, tc := range cases {
		for _, dry := range []string{"false", "true"} {
			flags := map[string]string{"dry-run": dry}
			for k, v := range tc.flags {
				flags[k] = v
			}
			_, err := runSlidesCmd(t, tc.cmd, nil, flags)
			if err == nil || exitCodeFor(err) != clierr.ExitUsage {
				t.Fatalf("%s (dry-run=%s) 应 exit 2，got %d: %v", tc.name, dry, exitCodeFor(err), err)
			}
		}
	}
	if len(m.requests) != 0 {
		t.Fatalf("参数错误不应发请求，实际 %d 次", len(m.requests))
	}
	// 缺 index.html：dry-run 以 would_block 预览（exit 0），实跑才是用法错误
	if _, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": t.TempDir()}); exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("缺 index.html 实跑应 exit 2: %v", err)
	}
	// 1000 字符整（按 rune 计）应通过
	if err := validateAppsReleaseApplyReason(strings.Repeat("理", 1000)); err != nil {
		t.Fatalf("1000 字符应通过: %v", err)
	}
}

// TestAppsWithHint_RateLimitIsBackoff 限流（99991400，HTTP 200 / 429 两种）时不追加参数类提示，改为退避提示；
// 鉴权错误照旧保留命令提示。
func TestAppsWithHint_RateLimitIsBackoff(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	for _, status := range []int{200, 429} {
		status := status
		m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
			return status, `{"code":99991400,"msg":"request trigger frequency limit"}`
		})
		_, err := runSlidesCmd(t, appsGetCmd, nil, map[string]string{"app-id": "app_x"})
		if err == nil {
			t.Fatalf("HTTP %d 限流应报错", status)
		}
		msg := err.Error()
		if strings.Contains(msg, "确认 app_id") || strings.Contains(msg, "apps list --keyword") {
			t.Fatalf("HTTP %d 限流不应追加参数类提示:\n%s", status, msg)
		}
		if !strings.Contains(msg, appsRateLimitHint) {
			t.Fatalf("HTTP %d 限流应给退避提示:\n%s", status, msg)
		}
	}
	m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 400, `{"code":99991679,"msg":"Unauthorized"}`
	})
	_, err := runSlidesCmd(t, appsGetCmd, nil, map[string]string{"app-id": "app_x"})
	if err == nil || !strings.Contains(err.Error(), "确认 app_id") || strings.Contains(err.Error(), appsRateLimitHint) {
		t.Fatalf("鉴权错误应保留命令提示: %v", err)
	}
}

// setupAppsWaitBackoff 把退避参数调小，便于测试。
func setupAppsWaitBackoff(t *testing.T) {
	t.Helper()
	origBase, origMax, origN := appsWaitRetryBase, appsWaitRetryMax, appsWaitMaxTransientFailures
	appsWaitRetryBase, appsWaitRetryMax = time.Millisecond, 4*time.Millisecond
	t.Cleanup(func() { appsWaitRetryBase, appsWaitRetryMax, appsWaitMaxTransientFailures = origBase, origMax, origN })
}

// TestAppsHTMLPublish_WaitBacksOffOnRateLimit --wait 轮询连续两次 99991400（HTTP 200 与 429）后成功，应继续轮询并成功；
// 发布提交（POST releases）只发一次。
func TestAppsHTMLPublish_WaitBacksOffOnRateLimit(t *testing.T) {
	m, _ := setupAppsPublishMock(t, nil)
	setupAppsWaitBackoff(t)
	polls := 0
	m.on("GET", "/apps/app_x/releases/rel_1", func(*http.Request, map[string]any) (int, string) {
		polls++
		switch polls {
		case 1:
			return 200, `{"code":99991400,"msg":"request trigger frequency limit"}`
		case 2:
			return 429, `{"code":99991400,"msg":"request trigger frequency limit"}`
		default:
			return 200, `{"code":0,"data":{"release":{"release_id":"rel_1","status":"finished","online_url":"https://miaoda.feishu.cn/app/app_x"}}}`
		}
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	out, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"})
	if err != nil {
		t.Fatalf("限流后应退避重试并成功: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if got["status"] != "finished" || polls != 3 {
		t.Fatalf("polls=%d 输出=%v", polls, got)
	}
	if n := len(m.find("POST", "/apps/app_x/releases")); n != 1 {
		t.Fatalf("发布提交不得重放，实际 %d 次", n)
	}
}

// TestAppsHTMLPublish_WaitGivesUpAfterTransientLimit 连续瞬时失败超过上限后报错，保留「不要重新发布」提示。
func TestAppsHTMLPublish_WaitGivesUpAfterTransientLimit(t *testing.T) {
	m, _ := setupAppsPublishMock(t, nil)
	setupAppsWaitBackoff(t)
	appsWaitMaxTransientFailures = 2
	polls := 0
	m.on("GET", "/apps/app_x/releases/rel_1", func(*http.Request, map[string]any) (int, string) {
		polls++
		return 429, `{"code":99991400,"msg":"request trigger frequency limit"}`
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	_, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"})
	if err == nil || !strings.Contains(err.Error(), "不要重新发布") || strings.Contains(err.Error(), appsRateLimitHint) {
		t.Fatalf("超过上限应报错并保留不要重新发布提示: %v", err)
	}
	if polls != 3 {
		t.Fatalf("上限 2 次应共查询 3 次，实际 %d", polls)
	}
	if n := len(m.find("POST", "/apps/app_x/releases")); n != 1 {
		t.Fatalf("发布提交不得重放，实际 %d 次", n)
	}
}

// TestAppsHTMLPublish_WaitNonTransientFailsFast 非瞬时错误（如鉴权）不重试。
func TestAppsHTMLPublish_WaitNonTransientFailsFast(t *testing.T) {
	m, _ := setupAppsPublishMock(t, nil)
	setupAppsWaitBackoff(t)
	polls := 0
	m.on("GET", "/apps/app_x/releases/rel_1", func(*http.Request, map[string]any) (int, string) {
		polls++
		return 400, `{"code":99991679,"msg":"Unauthorized"}`
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	if _, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"}); err == nil {
		t.Fatal("鉴权错误应报错")
	}
	if polls != 1 {
		t.Fatalf("非瞬时错误不应重试，实际 %d 次", polls)
	}
}

func TestAppsWaitBackoff(t *testing.T) {
	setupAppsWaitBackoff(t)
	appsWaitRetryBase, appsWaitRetryMax = time.Second, 5*time.Second
	for n, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 5 * time.Second, 10: 5 * time.Second} {
		if got := appsWaitBackoff(n); got != want {
			t.Errorf("appsWaitBackoff(%d) = %s, want %s", n, got, want)
		}
	}
}

// TestAppsList_OwnerID 显式 --owner-id 作为 owner_id 发送。
func TestAppsList_OwnerID(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/spark/v1/apps", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"items":[],"has_more":false}}`
	})
	if _, err := runSlidesCmd(t, appsListCmd, nil, map[string]string{"owner-id": " ou_abc ", "ownership": "mine"}); err != nil {
		t.Fatal(err)
	}
	q := m.find("GET", "/spark/v1/apps")[0].Query
	if q.Get("owner_id") != "ou_abc" || q.Get("ownership") != "mine" {
		t.Fatalf("query = %v", q)
	}
	if _, err := runSlidesCmd(t, appsListCmd, nil, nil); err != nil {
		t.Fatal(err)
	}
	if q := m.find("GET", "/spark/v1/apps")[1].Query; q.Has("owner_id") {
		t.Fatalf("未传 --owner-id 不应带 owner_id: %v", q)
	}
}

// TestAppsUpdate_IconFlow --icon-file：GET 预检 → multipart 上传 → PATCH 写入返回的 icon_url；
// --icon-url：GET 预检 → PATCH；40002 给出类型不支持提示。
func TestAppsUpdate_IconFlow(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app":{"app_id":"app_x","app_type":"HTML"}}}`
	})
	var uploadCT string
	m.on("POST", "/spark/v1/icon", func(r *http.Request, _ map[string]any) (int, string) {
		uploadCT = r.Header.Get("Content-Type")
		return 200, `{"code":0,"data":{"icon_url":"https://cdn.example.com/icon.png"}}`
	})
	m.on("PATCH", "/spark/v1/apps/app_x", func(_ *http.Request, body map[string]any) (int, string) {
		icon, _ := body["icon_url"].(string)
		return 200, `{"code":0,"data":{"app":{"app_id":"app_x","icon_url":"` + icon + `"}}}`
	})
	icon := writeAppsIconFixture(t)
	if _, err := runSlidesCmd(t, appsUpdateCmd, nil, map[string]string{"app-id": "app_x", "icon-file": icon}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uploadCT, "multipart/form-data") {
		t.Fatalf("上传应为 multipart，Content-Type=%q", uploadCT)
	}
	patches := m.find("PATCH", "/spark/v1/apps/app_x")
	if len(patches) != 1 || patches[0].Body["icon_url"] != "https://cdn.example.com/icon.png" {
		t.Fatalf("PATCH 应写入上传返回的 icon_url: %v", patches)
	}
	if len(m.find("GET", "/spark/v1/apps/app_x")) != 1 {
		t.Fatal("改图标前应 GET 预检一次")
	}

	if _, err := runSlidesCmd(t, appsUpdateCmd, nil, map[string]string{"app-id": "app_x", "icon-url": "https://example.com/b.png"}); err != nil {
		t.Fatal(err)
	}
	if n := len(m.find("POST", "/spark/v1/icon")); n != 1 {
		t.Fatalf("--icon-url 不应上传，上传次数 %d", n)
	}
	if got := m.find("PATCH", "/spark/v1/apps/app_x")[1].Body["icon_url"]; got != "https://example.com/b.png" {
		t.Fatalf("--icon-url PATCH body icon_url=%v", got)
	}

	m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 400, `{"code":40002,"msg":"unsupported app type"}`
	})
	_, err := runSlidesCmd(t, appsUpdateCmd, nil, map[string]string{"app-id": "app_x", "icon-file": icon})
	if err == nil || !strings.Contains(err.Error(), "妙搭编辑器") {
		t.Fatalf("40002 应提示在编辑器中修改: %v", err)
	}
	if n := len(m.find("POST", "/spark/v1/icon")); n != 1 {
		t.Fatalf("预检失败不应上传，上传次数 %d", n)
	}

	// 只改名称时不预检、不上传
	before := len(m.find("GET", "/spark/v1/apps/app_x"))
	if _, err := runSlidesCmd(t, appsUpdateCmd, nil, map[string]string{"app-id": "app_x", "name": "n"}); err != nil {
		t.Fatal(err)
	}
	if len(m.find("GET", "/spark/v1/apps/app_x")) != before {
		t.Fatal("只改名称不应 GET 预检")
	}
}

func TestAppsUpdate_IconFileTooLarge(t *testing.T) {
	withAppsNoCredentials(t)
	orig := appsIconMaxBytes
	appsIconMaxBytes = 8
	t.Cleanup(func() { appsIconMaxBytes = orig })
	_, err := runSlidesCmd(t, appsUpdateCmd, nil, map[string]string{"app-id": "app_x", "icon-file": writeAppsIconFixture(t), "dry-run": "true"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "2MB") {
		t.Fatalf("超限应用法错误: %v", err)
	}
}

// setupAppsDeleteMock GET 返回 name，DELETE 返回 in_trash。
func setupAppsDeleteMock(t *testing.T, name string) *slidesMockServer {
	t.Helper()
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/spark/v1/apps/meta_x", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app":{"app_id":"app_x","name":"` + name + `"}}}`
	})
	m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app":{"app_id":"app_x","name":"` + name + `"}}}`
	})
	m.on("DELETE", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"in_trash":true}}`
	})
	return m
}

// TestAppsDelete_ConfirmationGate 非交互且未 --yes：exit 10，不发任何请求。
func TestAppsDelete_ConfirmationGate(t *testing.T) {
	m := setupAppsDeleteMock(t, "n")
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	t.Cleanup(func() { confirmIsInteractive = origInteractive })
	origYes := assumeYes
	assumeYes = false
	t.Cleanup(func() { assumeYes = origYes })
	_, err := runSlidesCmd(t, appsDeleteCmd, nil, map[string]string{"app-id": "app_x", "confirm-name": "n"})
	if exitCodeFor(err) != clierr.ExitConfirmationRequired {
		t.Fatalf("应 exit 10，got %d: %v", exitCodeFor(err), err)
	}
	if len(m.requests) != 0 {
		t.Fatalf("未确认不应发请求，实际 %d 次", len(m.requests))
	}
}

// TestAppsDelete_NameCheck 名称一致才 DELETE（meta_token 输入按解析出的 app_id 删除）；不一致 exit 2 且不 DELETE。
func TestAppsDelete_NameCheck(t *testing.T) {
	m := setupAppsDeleteMock(t, "我的页面")
	origYes := assumeYes
	assumeYes = true
	t.Cleanup(func() { assumeYes = origYes })

	_, err := runSlidesCmd(t, appsDeleteCmd, nil, map[string]string{"app-id": "app_x", "confirm-name": "别的名字"})
	if exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), "apps get") {
		t.Fatalf("名称不一致应 exit 2 并提示 apps get，got %d: %v", exitCodeFor(err), err)
	}
	if n := len(m.find("DELETE", "/spark/v1/apps/app_x")); n != 0 {
		t.Fatalf("名称不一致不得 DELETE，实际 %d 次", n)
	}

	out, err := runSlidesCmd(t, appsDeleteCmd, nil, map[string]string{"app-id": "meta_x", "confirm-name": "我的页面"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if got["app_id"] != "app_x" || got["deleted"] != true || got["in_trash"] != true || got["name"] != "我的页面" {
		t.Fatalf("输出 = %v", got)
	}
	if n := len(m.find("DELETE", "/spark/v1/apps/app_x")); n != 1 {
		t.Fatalf("应按解析出的 app_id DELETE 一次，实际 %d", n)
	}
}

func TestAppsResolveForDelete_Inconsistent(t *testing.T) {
	for _, data := range []map[string]any{
		{"app": map[string]any{"app_id": "app_y", "name": "n"}},
		{"app": map[string]any{"app_id": "app_x"}},
		{"app": map[string]any{"app_id": "meta_x", "name": "n"}},
		{},
	} {
		if _, _, err := appsResolveForDelete(data, "app_x"); err == nil {
			t.Fatalf("不一致响应应拒绝: %v", data)
		}
	}
}

// TestAppsReleaseCreate_BodyAndWait body 只含提供的字段；--wait 复用轮询；POST 只发一次。
func TestAppsReleaseCreate_BodyAndWait(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("POST", "/apps/app_x/releases", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"release_id":"rel_9","status":"publishing","sync":false,"extra":1}}`
	})
	out, err := runSlidesCmd(t, appsReleaseCreateCmd, nil, map[string]string{"app-id": "app_x", "branch": " b ", "apply-reason": "修复"})
	if err != nil {
		t.Fatal(err)
	}
	body := m.find("POST", "/apps/app_x/releases")[0].Body
	if body["branch"] != "b" || body["apply_reason"] != "修复" || len(body) != 2 {
		t.Fatalf("body = %v", body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["release_id"] != "rel_9" || got["status"] != "publishing" || got["sync"] != false || got["extra"] != nil {
		t.Fatalf("输出 = %v (%v)", got, err)
	}

	orig := appsReleasePollInterval
	appsReleasePollInterval = time.Millisecond
	t.Cleanup(func() { appsReleasePollInterval = orig })
	m.on("GET", "/apps/app_x/releases/rel_9", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"release":{"release_id":"rel_9","status":"finished","online_url":"https://miaoda.feishu.cn/app/app_x"}}}`
	})
	out, err = runSlidesCmd(t, appsReleaseCreateCmd, nil, map[string]string{"app-id": "app_x", "wait": "true"})
	if err != nil || !strings.Contains(out, `"outcome": "finished"`) {
		t.Fatalf("--wait 应轮询到 finished: %v\n%s", err, out)
	}
	if n := len(m.find("POST", "/apps/app_x/releases")); n != 2 {
		t.Fatalf("两次调用各 POST 一次，实际 %d", n)
	}
	if b := m.find("POST", "/apps/app_x/releases")[1].Body; len(b) != 0 {
		t.Fatalf("未传 branch/apply-reason 时 body 应为空: %v", b)
	}
}

// TestAppsFeatures_OperationRefLowercased --operation-ref 规范化为小写并作为 operation_ref 发送。
func TestAppsFeatures_OperationRefLowercased(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/apps/app_x/features", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app_id":"app_x","features":[{"name":"database","enabled":true}]}}`
	})
	out, err := runSlidesCmd(t, appsFeaturesCmd, nil, map[string]string{"app-id": "app_x", "operation-ref": " ABC-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if q := m.find("GET", "/apps/app_x/features")[0].Query; q.Get("operation_ref") != "abc-1" {
		t.Fatalf("query = %v", q)
	}
	if !strings.Contains(out, `"database"`) {
		t.Fatalf("输出应透传 features:\n%s", out)
	}
}
