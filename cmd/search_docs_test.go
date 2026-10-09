package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// startSearchDocsStub 假 v2 doc_wiki/search 服务：记录请求路径与请求体，返回一条 v2 res_unit。
func startSearchDocsStub(t *testing.T) (cfg string, seen func() (paths []string, body map[string]any)) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		lastBody = nil
		_ = json.Unmarshal(raw, &lastBody)
		mu.Unlock()
		if r.URL.Path != "/open-apis/search/v2/doc_wiki/search" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"total":42,"has_more":true,"page_token":"pt_next",`+
			`"res_units":[{"entity_type":"DOC","title_highlighted":"<h>产品</h>需求","result_meta":{"token":"doxcnFpTest","doc_types":"DOCX",`+
			`"url":"https://example.feishu.cn/docx/doxcnFpTest","owner_id":"ou_owner"}},`+
			`{"entity_type":"WIKI","title":"无链接","result_meta":{"token":"wikcnFpTest","doc_types":"WIKI"}}]}}`)
	}))
	t.Cleanup(srv.Close)
	return writeStubConfig(t, srv.URL), func() ([]string, map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...), lastBody
	}
}

// TestSearchDocsUsesV2Endpoint search docs 走 v2 doc_wiki/search（不再调旧 suite/docs-api/search/object），
// 旧 flag 映射到 v2 请求体，输出保持旧字段名并新增 PageToken。
func TestSearchDocsUsesV2Endpoint(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, seen := startSearchDocsStub(t)

	stdout, stderr, err := runCLI(t, "search", "docs", "产品需求", "--count", "50", "--page-token", "pt_prev",
		"--owner-ids", "ou_a,ou_b", "--chat-ids", "oc_x", "--docs-types", "docx,Sheet", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("search docs 失败: %v\nstderr=%s", err, stderr)
	}
	paths, body := seen()
	if len(paths) != 1 || paths[0] != "POST /open-apis/search/v2/doc_wiki/search" {
		t.Fatalf("请求路径 = %v，应只调用 v2 doc_wiki/search", paths)
	}
	if body["query"] != "产品需求" || body["page_size"] != float64(20) || body["page_token"] != "pt_prev" {
		t.Fatalf("body = %v（--count 50 应夹到 page_size 20）", body)
	}
	for _, legacy := range []string{"search_key", "count", "offset", "owner_ids", "docs_types"} {
		if _, ok := body[legacy]; ok {
			t.Fatalf("不应再发送旧端点字段 %s: %v", legacy, body)
		}
	}
	for _, key := range []string{"doc_filter", "wiki_filter"} {
		f, _ := body[key].(map[string]any)
		if fmt.Sprint(f["creator_ids"]) != "[ou_a ou_b]" || fmt.Sprint(f["chat_ids"]) != "[oc_x]" || fmt.Sprint(f["doc_types"]) != "[DOCX SHEET]" {
			t.Fatalf("%s = %v（owner-ids→creator_ids，docs-types→大写 doc_types）", key, f)
		}
	}
	if !strings.Contains(stderr, "单页最多 20 条") {
		t.Fatalf("--count 超过 20 应在 stderr 提示，stderr=%q", stderr)
	}

	var out struct {
		Total     int
		HasMore   bool
		PageToken string
		ResUnits  []struct{ DocsToken, DocsType, Title, OwnerID, URL string }
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
	}
	if out.Total != 42 || !out.HasMore || out.PageToken != "pt_next" || len(out.ResUnits) != 2 {
		t.Fatalf("输出 = %+v", out)
	}
	first := out.ResUnits[0]
	if first.DocsToken != "doxcnFpTest" || first.DocsType != "docx" || first.Title != "产品需求" || first.OwnerID != "ou_owner" || first.URL != "https://example.feishu.cn/docx/doxcnFpTest" {
		t.Fatalf("第一条 = %+v", first)
	}
	if second := out.ResUnits[1]; second.DocsType != "wiki" || !strings.HasSuffix(second.URL, "/wiki/wikcnFpTest") {
		t.Fatalf("缺 url 时应按类型拼接链接: %+v", second)
	}

	// 文本模式翻页提示改为 --page-token
	stdout, _, err = runCLI(t, "search", "docs", "产品需求", "--config", cfg)
	if err != nil || !strings.Contains(stdout, "--page-token pt_next") || strings.Contains(stdout, "--offset") {
		t.Fatalf("文本翻页提示应为 --page-token: err=%v\n%s", err, stdout)
	}
	if _, body := seen(); body["page_size"] != float64(20) {
		t.Fatalf("默认 --count 20 应映射 page_size 20: %v", body)
	}
}

// TestSearchDocsLegacyFlagUsageErrors --offset>0 无法映射到 v2、非法类型与负数均为用法错误，且不发请求。
func TestSearchDocsLegacyFlagUsageErrors(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, seen := startSearchDocsStub(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--offset", "20"}, "--page-token"},
		{[]string{"--offset", "-1"}, "--offset"},
		{[]string{"--count", "-1"}, "--count"},
		{[]string{"--docs-types", "docx,pdf"}, "pdf"},
	}
	for _, tc := range cases {
		args := append([]string{"search", "docs", "x", "--config", cfg}, tc.args...)
		_, _, err := runCLI(t, args...)
		if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v 应为用法错误 exit 2 且提示 %q，实际: %v", tc.args, tc.want, err)
		}
	}
	if paths, _ := seen(); len(paths) != 0 {
		t.Fatalf("用法错误不应发请求: %v", paths)
	}
}
