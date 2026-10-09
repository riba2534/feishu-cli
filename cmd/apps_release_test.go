package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// setupAppsPublishMock 模拟 html-publish 三段协议 + release get；releaseResponses 依次作为每次 release get 的响应。
func setupAppsPublishMock(t *testing.T, releaseResponses []string) (*slidesMockServer, *int) {
	t.Helper()
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/spark/v1/apps/app_x", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app":{"app_id":"app_x","app_type":"HTML"}}}`
	})
	m.on("GET", "/apps/app_x/pre_release", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"kvs":[{"key":"upload_url","value":"` + srv.URL + `/tos/upload"},{"key":"tos_path","value":"tos/abc"}]}}`
	})
	m.on("PUT", "/tos/upload", func(*http.Request, map[string]any) (int, string) { return 200, `` })
	m.on("POST", "/apps/app_x/releases", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"release_id":"rel_1"}}`
	})
	polls := 0
	m.on("GET", "/apps/app_x/releases/rel_1", func(*http.Request, map[string]any) (int, string) {
		i := polls
		if i >= len(releaseResponses) {
			i = len(releaseResponses) - 1
		}
		polls++
		return 200, releaseResponses[i]
	})
	orig := appsReleasePollInterval
	appsReleasePollInterval = time.Millisecond
	t.Cleanup(func() { appsReleasePollInterval = orig })
	return m, &polls
}

// TestAppsHTMLPublish_WaitFinished 验证 --wait 轮询到 finished 并输出 online_url（P0：发布后拿得到访问链接）。
func TestAppsHTMLPublish_WaitFinished(t *testing.T) {
	_, polls := setupAppsPublishMock(t, []string{
		`{"code":0,"data":{"release":{"release_id":"rel_1","status":"publishing"}}}`,
		`{"code":0,"data":{"release":{"release_id":"rel_1","status":"finished","online_url":"https://miaoda.feishu.cn/app/app_x"}}}`,
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	out, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"})
	if err != nil {
		t.Fatalf("--wait finished 不应报错: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if got["status"] != "finished" || got["online_url"] != "https://miaoda.feishu.cn/app/app_x" || got["release_id"] != "rel_1" {
		t.Fatalf("输出 = %v", got)
	}
	if *polls != 2 {
		t.Fatalf("应轮询到终态为止，实际 %d 次", *polls)
	}
}

func TestAppsHTMLPublish_WaitFailedExitsNonZero(t *testing.T) {
	setupAppsPublishMock(t, []string{
		`{"code":0,"data":{"release":{"release_id":"rel_1","status":"failed"},"error_logs":[{"step":"build","error_log":"index.html missing"}]}}`,
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	out, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"})
	if err == nil || !strings.Contains(err.Error(), "rel_1") {
		t.Fatalf("failed 应非零退出: %v", err)
	}
	if !strings.Contains(out, "index.html missing") {
		t.Fatalf("应输出外层 error_logs:\n%s", out)
	}
}

// TestAppsHTMLPublish_WaitStopsOnPendingApproval 审批节点 PENDING 时立即停止轮询（不是失败），
// 且 camelCase 的 currentStatus 也能识别。
func TestAppsHTMLPublish_WaitStopsOnPendingApproval(t *testing.T) {
	_, polls := setupAppsPublishMock(t, []string{
		`{"code":0,"data":{"release":{"release_id":"rel_1","status":"publishing"},"current_node_info":{"currentStatus":"PENDING","result":{"approvalURL":"https://example.feishu.cn/approval/1"}}}}`,
	})
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	out, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true"})
	if err != nil {
		t.Fatalf("待审批不是失败: %v", err)
	}
	if *polls != 1 {
		t.Fatalf("PENDING 应立即停止轮询，实际 %d 次", *polls)
	}
	for _, want := range []string{`"outcome": "pending_approval"`, `"current_status": "PENDING"`, `"approval_url"`} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺 %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "currentStatus") {
		t.Errorf("camelCase 字段应被规范化:\n%s", out)
	}
}

func TestAppsHTMLPublish_WaitTimeout(t *testing.T) {
	_, polls := setupAppsPublishMock(t, []string{`{"code":0,"data":{"release":{"release_id":"rel_1","status":"publishing"}}}`})
	orig := appsReleasePollInterval
	appsReleasePollInterval = 20 * time.Millisecond
	t.Cleanup(func() { appsReleasePollInterval = orig })
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	out, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "app_x", "path": dir, "wait": "true", "wait-timeout": "50ms"})
	if err != nil {
		t.Fatalf("超时不应报错（发布仍在进行）: %v", err)
	}
	if !strings.Contains(out, `"outcome": "timeout"`) || *polls < 2 {
		t.Fatalf("应超时停止，polls=%d:\n%s", *polls, out)
	}
}

// TestAppsHTMLPublish_RejectsNonAppID 对齐官方 validateRealAppID：meta_token 等非 app_ 前缀直接用法错误（dry-run 也拦）。
func TestAppsHTMLPublish_RejectsNonAppID(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	dir := writeAppsIndexFixture(t, "<h1>hi</h1>")
	for _, dry := range []string{"false", "true"} {
		_, err := runSlidesCmd(t, appsHTMLPublishCmd, nil, map[string]string{"app-id": "meta_xxx", "path": dir, "dry-run": dry})
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "apps get") {
			t.Fatalf("dry-run=%s 非 app_ 前缀应用法错误并提示 apps get: %v", dry, err)
		}
	}
	if len(m.requests) != 0 {
		t.Fatal("不应发请求")
	}
}

func TestAppsReleaseGet_ProjectsNestedRelease(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/apps/app_x/releases/rel_9", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"release":{"release_id":"rel_9","status":"pending","error_logs":[{"step":"inner"}],"current_node_info":{"current_status":"X"}},"current_node_info":{"currentNode":"approve","currentStatus":"PENDING","submittedBy":{"openID":"ou_x","username":"u"}}}}`
	})
	out, err := runSlidesCmd(t, appsReleaseGetCmd, nil, map[string]string{"app-id": "app_x", "release-id": "rel_9"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got["release_id"] != "rel_9" || got["status"] != "pending" {
		t.Fatalf("应展平 data.release: %v", got)
	}
	node := got["current_node_info"].(map[string]any)
	if node["current_status"] != "PENDING" || node["current_node"] != "approve" {
		t.Fatalf("外层 current_node_info 优先且规范化为 snake_case: %v", node)
	}
	if node["submitted_by"].(map[string]any)["open_id"] != "ou_x" {
		t.Fatalf("submitted_by.openID 应规范化: %v", node)
	}
	// 非 app_ 前缀 → 用法错误
	if _, err := runSlidesCmd(t, appsReleaseGetCmd, nil, map[string]string{"app-id": "cli_x", "release-id": "r"}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("cli_ 前缀应拒绝: %v", err)
	}
}

func TestAppsReleaseList_ParamsAndValidation(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/apps/app_x/releases", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"releases":[{"release_id":"r1","status":"failed"}],"has_more":true,"page_token":"pt2"}}`
	})
	out, err := runSlidesCmd(t, appsReleaseListCmd, nil, map[string]string{"app-id": "app_x", "status": "failed", "page-size": "5"})
	if err != nil {
		t.Fatal(err)
	}
	q := m.find("GET", "/apps/app_x/releases")[0].Query
	if q.Get("status") != "failed" || q.Get("page_size") != "5" {
		t.Fatalf("query = %v", q)
	}
	if !strings.Contains(out, `"page_token": "pt2"`) {
		t.Fatalf("应透出续翻游标:\n%s", out)
	}
	for _, bad := range []map[string]string{{"app-id": "app_x", "status": "done"}, {"app-id": "app_x", "page-size": "0"}, {"app-id": ""}} {
		if _, err := runSlidesCmd(t, appsReleaseListCmd, nil, bad); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v 应是用法错误: %v", bad, err)
		}
	}
}

// TestAppsCreate_LowercaseTypesAndSourceAgent 对齐官方小写枚举 html/frontend/full_stack，大写为兼容别名；
// 设置 FEISHU_CLI_AGENT_NAME 时附带 source_agent。
func TestAppsCreate_LowercaseTypesAndSourceAgent(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	t.Setenv("FEISHU_CLI_AGENT_NAME", "claude-code")
	m.on("POST", "/spark/v1/apps", func(_ *http.Request, body map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"app":{"app_id":"app_new"}}}`
	})
	for in, want := range map[string]string{"html": "html", "frontend": "frontend", "full_stack": "full_stack", "HTML": "html"} {
		if _, err := runSlidesCmd(t, appsCreateCmd, nil, map[string]string{"name": "x", "app-type": in}); err != nil {
			t.Fatalf("--app-type %s: %v", in, err)
		}
		reqs := m.find("POST", "/spark/v1/apps")
		body := reqs[len(reqs)-1].Body
		if body["app_type"] != want || body["source_agent"] != "claude-code" {
			t.Fatalf("--app-type %s body = %v", in, body)
		}
	}
	if _, err := runSlidesCmd(t, appsCreateCmd, nil, map[string]string{"name": "x", "app-type": "spa"}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("未知类型应用法错误: %v", err)
	}
	t.Setenv("FEISHU_CLI_AGENT_NAME", "bad\nname")
	t.Setenv("LARKSUITE_CLI_AGENT_NAME", "")
	if body := buildAppsCreateBody("x", "html", "", ""); body["source_agent"] != nil {
		t.Fatalf("含控制字符的 agent 名应丢弃: %v", body)
	}
}

func TestAppsList_VisibleWithFilters(t *testing.T) {
	if appsListCmd.Hidden {
		t.Fatal("apps list 应恢复可见")
	}
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test")
	m.on("GET", "/spark/v1/apps", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"items":[{"app_id":"app_1","name":"审批"}],"has_more":false}}`
	})
	if _, err := runSlidesCmd(t, appsListCmd, nil, map[string]string{"keyword": "审批", "ownership": "mine", "app-type": "html"}); err != nil {
		t.Fatal(err)
	}
	q := m.find("GET", "/spark/v1/apps")[0].Query
	if q.Get("keyword") != "审批" || q.Get("ownership") != "mine" || q.Get("app_type") != "html" {
		t.Fatalf("query = %v", q)
	}
	if _, err := runSlidesCmd(t, appsListCmd, nil, map[string]string{"ownership": "others"}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 ownership 应用法错误: %v", err)
	}
}

func TestAppsHelpScopeTextIsIncremental(t *testing.T) {
	if strings.Contains(appsCmd.Long, "替换") || !strings.Contains(appsCmd.Long, "增量") {
		t.Fatalf("apps 帮助应说明 auth login 为增量授权:\n%s", appsCmd.Long)
	}
}
