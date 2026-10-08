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

	"github.com/riba2534/feishu-cli/internal/client"
)

func TestParseResourceArg_Offline(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		opts      resourceArgOptions
		wantType  string
		wantToken string
		wantErr   string
	}{
		{name: "裸 token 用默认类型", raw: "DocABC", opts: resourceArgOptions{DefaultType: "docx"}, wantType: "docx", wantToken: "DocABC"},
		{name: "裸 token 显式类型并规范化别名", raw: "ShtTok", opts: resourceArgOptions{ExplicitType: "Sheets"}, wantType: "sheet", wantToken: "ShtTok"},
		{name: "裸 token 无类型报错", raw: "DocABC", opts: resourceArgOptions{ArgName: "--url"}, wantErr: "必须显式指定资源类型"},
		{name: "部分路径报错", raw: "tmp/wiki/wikcn123", opts: resourceArgOptions{DefaultType: "docx"}, wantErr: "不接受部分路径"},
		{name: "非法 token 报错", raw: "a.b", opts: resourceArgOptions{DefaultType: "docx"}, wantErr: "不是有效的 token"},
		{name: "query 中 /wiki/ 不劫持", raw: "https://example.feishu.cn/docx/DocABC?from=/wiki/zzz", opts: resourceArgOptions{}, wantType: "docx", wantToken: "DocABC"},
		{name: "URL 与 --type 一致", raw: "https://example.feishu.cn/docx/DocABC", opts: resourceArgOptions{ExplicitType: "docx"}, wantType: "docx", wantToken: "DocABC"},
		{name: "docx URL + --type sheet 冲突报错", raw: "https://example.feishu.cn/docx/DocABC", opts: resourceArgOptions{ExplicitType: "sheet"}, wantErr: "冲突"},
		{name: "wiki URL + 非 wiki 类型且不解包时冲突", raw: "https://example.feishu.cn/wiki/WikTok", opts: resourceArgOptions{ExplicitType: "docx"}, wantErr: "冲突"},
		{name: "wiki URL + 非 wiki 类型且解包时视为断言", raw: "https://example.feishu.cn/wiki/WikTok", opts: resourceArgOptions{ExplicitType: "docx", ResolveWiki: true}, wantType: "wiki", wantToken: "WikTok"},
		{name: "类型不在白名单", raw: "https://example.feishu.cn/sheets/ShtTok", opts: resourceArgOptions{Allowed: []string{"docx"}}, wantErr: "仅支持 docx"},
		{name: "wiki 解包前不校验白名单", raw: "https://example.feishu.cn/wiki/WikTok", opts: resourceArgOptions{Allowed: []string{"docx"}, ResolveWiki: true}, wantType: "wiki", wantToken: "WikTok"},
		{name: "folder 共享链接", raw: "https://example.feishu.cn/drive/shr/FldTok", opts: resourceArgOptions{Allowed: []string{"folder"}}, wantType: "folder", wantToken: "FldTok"},
		{name: "伪造域名报错", raw: "https://evilfeishu.cn/docx/DocABC", opts: resourceArgOptions{}, wantErr: "不支持的域名"},
		{name: "空输入报错", raw: "  ", opts: resourceArgOptions{ArgName: "--doc"}, wantErr: "--doc 不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := parseResourceArg(tc.raw, tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望包含 %q 的错误，得到 res=%+v err=%v", tc.wantErr, res, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if res.Type != tc.wantType || res.Token != tc.wantToken {
				t.Fatalf("得到 type=%q token=%q，期望 type=%q token=%q", res.Type, res.Token, tc.wantType, tc.wantToken)
			}
		})
	}
}

// newResourceTestServer 模拟 node_by_token（wiki → 指定 obj_type/obj_token）并记录请求。
func newResourceTestServer(t *testing.T, objType, objToken string, extra func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var reqs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		mu.Lock()
		reqs = append(reqs, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path == client.WikiNodeByTokenPath {
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"node":{"space_id":"sp-1","node_token":%q,"obj_token":%q,"obj_type":%q,"title":"T"}}}`,
				r.URL.Query().Get("token"), objToken, objType)
			return
		}
		if extra != nil && extra(w, r) {
			return
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), reqs...)
	}
}

func TestResolveResourceArg_WikiUnwrap(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	res, err := resolveResourceArg("https://example.feishu.cn/wiki/WikTok", resourceArgOptions{
		DefaultType: "docx", Allowed: []string{"docx"}, ResolveWiki: true, UserAccessToken: "u-test",
	})
	if err != nil {
		t.Fatalf("resolveResourceArg 失败: %v", err)
	}
	if res.Type != "docx" || res.Token != "DocObjTok" || res.WikiNode == nil || res.InputToken != "WikTok" {
		t.Fatalf("wiki 解包结果不符: %+v", res)
	}
	if got := reqs(); len(got) != 1 || !strings.HasPrefix(got[0], "GET "+client.WikiNodeByTokenPath+"?token=WikTok") {
		t.Fatalf("应只请求一次 node_by_token: %v", got)
	}

	// 底层类型不在白名单
	if _, err := resolveResourceArg("https://example.feishu.cn/wiki/WikTok", resourceArgOptions{
		ArgName: "<xml_presentation_id|url>", Allowed: []string{"slides"}, ResolveWiki: true, UserAccessToken: "u-test",
	}); err == nil || !strings.Contains(err.Error(), "仅支持 slides") {
		t.Fatalf("wiki 底层为 docx 时 slides 命令应报错，得到 %v", err)
	}
	// 显式期望类型与底层不一致
	if _, err := resolveResourceArg("https://example.feishu.cn/wiki/WikTok", resourceArgOptions{
		ExplicitType: "sheet", ResolveWiki: true, UserAccessToken: "u-test",
	}); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("wiki 底层类型与 --type 不一致应报错，得到 %v", err)
	}
}

func TestResolveDocxArg_NonWikiMakesNoRequest(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	for _, in := range []string{"DocABC", "https://example.feishu.cn/docx/DocABC?from=/wiki/zzz"} {
		got, err := resolveDocxArg(in, "<document_id>", "u-test")
		if err != nil || got != "DocABC" {
			t.Fatalf("resolveDocxArg(%q) = %q, %v；期望 DocABC", in, got, err)
		}
	}
	if len(reqs()) != 0 {
		t.Fatalf("非 wiki 输入不应发请求: %v", reqs())
	}
	if _, err := resolveDocxArg("https://example.feishu.cn/sheets/ShtTok", "<document_id>", "u-test"); err == nil {
		t.Fatal("sheet URL 传给 docx 命令应报错")
	}
}

// TestDocRead_WikiURLUsesObjToken 回归：doc read <wiki URL> 旧实现报「无效的文档 token」。
func TestDocRead_WikiURLUsesObjToken(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/open-apis/docx/v1/documents/") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = docReadCmd.Flags().Set("outline", "true")
	defer resetCmdFlag(docReadCmd, "outline")
	if _, err := captureCmdStdout(t, func() error {
		return docReadCmd.RunE(docReadCmd, []string{"https://example.feishu.cn/wiki/WikTok"})
	}); err != nil {
		t.Fatalf("doc read <wiki URL> 失败: %v", err)
	}
	got := reqs()
	if len(got) < 2 || !strings.Contains(got[1], "/open-apis/docx/v1/documents/DocObjTok/blocks") {
		t.Fatalf("应先 node_by_token 再用 obj_token 读取块: %v", got)
	}
	for _, r := range got {
		if strings.Contains(r, "/documents/WikTok") {
			t.Fatalf("不应把 wiki token 当 document_id 使用: %v", got)
		}
	}
}

// TestDocMediaInsert_WikiURLNeverUsesWikiTokenAsRoute 回归：media-insert 不能把 wiki token 当 document_id / drive_route_token。
func TestDocMediaInsert_WikiURLNeverUsesWikiTokenAsRoute(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/open-apis/docx/v1/documents/") {
			// 让流程在第一步就停下：只关心它用哪个 token
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":1770002,"msg":"not found"}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = docMediaInsertCmd.Flags().Set("file", "/nonexistent.png")
	defer resetCmdFlag(docMediaInsertCmd, "file")
	_ = docMediaInsertCmd.RunE(docMediaInsertCmd, []string{"https://example.feishu.cn/wiki/WikTok"})
	got := reqs()
	if len(got) < 2 || !strings.Contains(got[1], "/open-apis/docx/v1/documents/DocObjTok/blocks/DocObjTok") {
		t.Fatalf("media-insert 应使用 wiki 解包后的 obj_token: %v", got)
	}
}

func TestSlidesGet_RejectsNonSlidesURLWithoutRequest(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	err := slidesGetCmd.RunE(slidesGetCmd, []string{"https://example.feishu.cn/docx/DocABC"})
	if err == nil || !strings.Contains(err.Error(), "仅支持 slides") {
		t.Fatalf("slides get <docx URL> 应在本地报类型错误，得到 %v", err)
	}
	if len(reqs()) != 0 {
		t.Fatalf("类型不符时不应发请求: %v", reqs())
	}
	// wiki 底层不是 slides
	err = slidesGetCmd.RunE(slidesGetCmd, []string{"https://example.feishu.cn/wiki/WikTok"})
	if err == nil || !strings.Contains(err.Error(), "仅支持 slides") {
		t.Fatalf("wiki 底层为 docx 时 slides get 应报错，得到 %v", err)
	}
}

func TestDriveInspect_QueryWikiMarkerNotHijacked(t *testing.T) {
	var batchBody string
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "metas/batch_query") {
			b, _ := io.ReadAll(r.Body)
			batchBody = string(b)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"metas":[{"title":"T","doc_token":"DocABC","doc_type":"docx"}]}}`)
			return true
		}
		return false
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = driveInspectCmd.Flags().Set("url", "https://example.feishu.cn/docx/DocABC?from=/wiki/zzz")
	_ = driveInspectCmd.Flags().Set("output", "json")
	defer resetCmdFlag(driveInspectCmd, "url", "output")
	out, err := captureCmdStdout(t, func() error { return driveInspectCmd.RunE(driveInspectCmd, nil) })
	if err != nil {
		t.Fatalf("drive inspect 失败: %v", err)
	}
	for _, r := range reqs() {
		if strings.Contains(r, "node_by_token") || strings.Contains(r, "zzz") {
			t.Fatalf("query 中的 /wiki/zzz 不应触发 wiki 解析: %v", reqs())
		}
	}
	if !strings.Contains(batchBody, `"doc_token":"DocABC"`) || !strings.Contains(batchBody, `"doc_type":"docx"`) {
		t.Fatalf("batch_query 应查询 docx DocABC，body=%s", batchBody)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil || result["type"] != "docx" || result["token"] != "DocABC" {
		t.Fatalf("输出不符: %s (err=%v)", out, err)
	}
}

func TestDriveInspect_TypeConflictFailsLocally(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = driveInspectCmd.Flags().Set("url", "https://example.feishu.cn/docx/DocABC")
	_ = driveInspectCmd.Flags().Set("type", "sheet")
	defer resetCmdFlag(driveInspectCmd, "url", "type")
	err := driveInspectCmd.RunE(driveInspectCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("docx URL + --type sheet 应报冲突（旧实现静默改成 sheet 且 exit 0），得到 %v", err)
	}
	if len(reqs()) != 0 {
		t.Fatalf("冲突时不应发请求: %v", reqs())
	}
}

func TestResolvePermApplyTarget(t *testing.T) {
	cases := []struct {
		raw, typ, wantTok, wantType, wantErr string
	}{
		{raw: "https://example.feishu.cn/docx/DocA?from=/wiki/WikB", wantTok: "DocA", wantType: "docx"},
		{raw: "https://example.feishu.cn/wiki/WikB", wantTok: "WikB", wantType: "wiki"},
		{raw: "https://example.feishu.cn/docx/DocA", typ: "sheet", wantErr: "冲突"},
		{raw: "DocA", typ: "docx", wantTok: "DocA", wantType: "docx"},
		{raw: "DocA", wantErr: "--type 必填"},
		{raw: "https://example.feishu.cn/drive/folder/FldA", wantErr: "仅支持"},
		{raw: "https://example.feishu.cn/x/docx/DocA", wantErr: "无法从 URL 路径"},
	}
	for _, tc := range cases {
		tok, typ, err := resolvePermApplyTarget(tc.raw, tc.typ)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("resolvePermApplyTarget(%q,%q) 期望错误含 %q，得到 tok=%q typ=%q err=%v", tc.raw, tc.typ, tc.wantErr, tok, typ, err)
			}
			continue
		}
		if err != nil || tok != tc.wantTok || typ != tc.wantType {
			t.Errorf("resolvePermApplyTarget(%q,%q) = %q,%q,%v；期望 %q,%q", tc.raw, tc.typ, tok, typ, err, tc.wantTok, tc.wantType)
		}
	}
}

func TestResolveCommentDoc(t *testing.T) {
	server, reqs := newResourceTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	tok, typ, by, err := resolveCommentDoc("https://example.feishu.cn/docx/DocA?from=/wiki/WikB", "u-test")
	if err != nil || tok != "DocA" || typ != "docx" || by != "docx_url" {
		t.Fatalf("docx URL 带 wiki query: %q %q %q %v", tok, typ, by, err)
	}
	if len(reqs()) != 0 {
		t.Fatalf("非 wiki 不应发请求: %v", reqs())
	}
	tok, typ, by, err = resolveCommentDoc("https://example.feishu.cn/wiki/WikB", "u-test")
	if err != nil || tok != "DocObjTok" || typ != "docx" || by != "wiki" {
		t.Fatalf("wiki URL: %q %q %q %v", tok, typ, by, err)
	}
	if _, _, _, err := resolveCommentDoc("https://example.feishu.cn/sheets/ShtA", "u-test"); err == nil {
		t.Fatal("sheet URL 应被拒绝（当前仅支持 doc/docx）")
	}
}

func TestNormalizeDriveExportInput_PathPrefixOnly(t *testing.T) {
	st, tok, rt, err := normalizeDriveExportInput("https://example.feishu.cn/docx/DocA?from=/wiki/WikB", "", "")
	if err != nil || st != "docx" || tok != "DocA" || rt != "docx" {
		t.Fatalf("得到 %q %q %q %v，期望 docx DocA docx", st, tok, rt, err)
	}
	if _, _, _, err := normalizeDriveExportInput("https://example.feishu.cn/docx/DocA", "", "sheet"); err == nil {
		t.Fatal("docx URL + --doc-type sheet 应报冲突")
	}
	if _, _, _, err := normalizeDriveExportInput("", "a/b", "docx"); err == nil {
		t.Fatal("含路径分隔符的 --token 应被拒绝")
	}
}
