package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newDocReadTestCmd 构造与 docReadCmd 同 flag 集的独立命令，避免全局 flag 状态在测试间串扰。
func newDocReadTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "read", RunE: docReadCmd.RunE, Args: docReadCmd.Args}
	c.Flags().AddFlagSet(docReadCmd.Flags())
	// AddFlagSet 共享 *pflag.Flag：先复位，测试结束再复位，避免把 Changed 状态泄漏给直接调用 docReadCmd 的测试
	reset := func() {
		docReadCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	}
	reset()
	t.Cleanup(reset)
	if err := c.Flags().Parse(args); err != nil {
		t.Fatalf("解析参数失败: %v", err)
	}
	return c
}

func TestBuildDocsAIReadOptions(t *testing.T) {
	cases := []struct {
		args    []string
		wantErr string
		check   func(t *testing.T, o *docsAIReadOptions, body map[string]any)
	}{
		{args: []string{"--with-ids"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			if o.format != "xml" || o.detail != "with-ids" || o.scope != "full" {
				t.Fatalf("opts = %+v", o)
			}
			if body["export_option"].(map[string]any)["export_block_id"] != true || body["read_option"] != nil {
				t.Fatalf("body = %#v", body)
			}
		}},
		{args: []string{"--with-ids", "--heading", "性能"}, check: func(t *testing.T, o *docsAIReadOptions, _ map[string]any) {
			if o.scope != "section" {
				t.Fatalf("--heading 应映射为 section: %+v", o)
			}
		}},
		{args: []string{"--engine", "docs_ai", "--keyword", "QPS|限流", "--context-after", "2"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			ro := body["read_option"].(map[string]any)
			if o.format != "markdown" || ro["read_mode"] != "keyword" || ro["keyword"] != "QPS|限流" || ro["context_after"] != "2" {
				t.Fatalf("body = %#v", body)
			}
		}},
		{args: []string{"--start-block-id", "blkA", "--end-block-id", "-1"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			ro := body["read_option"].(map[string]any)
			if ro["read_mode"] != "range" || ro["start_block_id"] != "blkA" || ro["end_block_id"] != "-1" {
				t.Fatalf("body = %#v", body)
			}
		}},
		{args: []string{"--engine", "docs_ai", "--outline", "--max-depth", "2"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			ro := body["read_option"].(map[string]any)
			if ro["read_mode"] != "outline" || ro["max_depth"] != "2" {
				t.Fatalf("body = %#v", body)
			}
		}},
		{args: []string{"--with-ids", "--doc-format", "markdown"}, wantErr: "只支持 --doc-format xml"},
		{args: []string{"--scope", "range"}, wantErr: "需要 --start-block-id"},
		{args: []string{"--scope", "keyword"}, wantErr: "需要 --keyword"},
		{args: []string{"--scope", "outline", "--keyword", "x"}, wantErr: "冲突"},
		{args: []string{"--engine", "docs_ai", "--detail", "bogus"}, wantErr: "不支持的 --detail"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			c := newDocReadTestCmd(t, tc.args...)
			use, err := wantsDocsAIRead(c)
			if err != nil || !use {
				t.Fatalf("应切到 docs_ai 引擎: use=%v err=%v", use, err)
			}
			o, err := buildDocsAIReadOptions(c)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDocsAIReadOptions: %v", err)
			}
			tc.check(t, o, buildDocsAIFetchBody(o))
		})
	}
}

func TestDocReadLocalEngineUnchangedByDefault(t *testing.T) {
	c := newDocReadTestCmd(t, "--outline")
	if use, err := wantsDocsAIRead(c); err != nil || use {
		t.Fatalf("默认应走本地引擎: use=%v err=%v", use, err)
	}
	c = newDocReadTestCmd(t, "--engine", "local", "--with-ids")
	if _, err := wantsDocsAIRead(c); err == nil {
		t.Fatal("显式 --engine local 搭配 docs_ai 专属 flag 应报错")
	}
}

func TestDocReadDocsAIFetchRequest(t *testing.T) {
	var gotBody map[string]any
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"content":"<p id=\"blk1\">正文</p>","revision_id":3}}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	c := newDocReadTestCmd(t, "--with-ids")
	var out bytes.Buffer
	c.SetOut(&out)
	if err := c.RunE(c, []string{"docABC"}); err != nil {
		t.Fatalf("doc read --with-ids 失败: %v", err)
	}
	if gotPath != "/open-apis/docs_ai/v1/documents/docABC/fetch" || gotBody["format"] != "xml" {
		t.Fatalf("请求异常: path=%s body=%#v", gotPath, gotBody)
	}
	if !strings.Contains(out.String(), `<p id="blk1">正文</p>`) {
		t.Fatalf("输出异常: %q", out.String())
	}
}

func TestBuildDocsAIReadOptionsIMMarkdownAndLang(t *testing.T) {
	cases := []struct {
		args    []string
		wantErr string
		check   func(t *testing.T, o *docsAIReadOptions, body map[string]any)
	}{
		{args: []string{"--doc-format", "im-markdown"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			if o.format != docFormatIMMarkdown || body["format"] != "markdown" {
				t.Fatalf("im-markdown 应向服务端请求 markdown: opts=%+v body=%#v", o, body)
			}
			if _, ok := body["lang"]; ok {
				t.Fatalf("未传 --lang 时不应带 lang: %#v", body)
			}
		}},
		{args: []string{"--doc-format", "IM-Markdown", "--scope", "keyword", "--keyword", "部署"}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			ro := body["read_option"].(map[string]any)
			if body["format"] != "markdown" || ro["read_mode"] != "keyword" {
				t.Fatalf("body = %#v", body)
			}
		}},
		{args: []string{"--lang", " en-US "}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			if body["lang"] != "en-US" || body["format"] != "markdown" {
				t.Fatalf("--lang 应去空白后透传: %#v", body)
			}
		}},
		{args: []string{"--engine", "docs_ai", "--lang", "  "}, check: func(t *testing.T, o *docsAIReadOptions, body map[string]any) {
			if _, ok := body["lang"]; ok {
				t.Fatalf("空白 --lang 不应下发: %#v", body)
			}
		}},
		{args: []string{"--with-ids", "--doc-format", "im-markdown"}, wantErr: "IM Markdown 无法携带 block id"},
		{args: []string{"--doc-format", "im-markdown", "--detail", "full"}, wantErr: "只支持 --doc-format xml"},
		{args: []string{"--doc-format", "im_markdown"}, wantErr: "可选 xml / markdown / im-markdown"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			c := newDocReadTestCmd(t, tc.args...)
			use, err := wantsDocsAIRead(c)
			if err != nil || !use {
				t.Fatalf("应切到 docs_ai 引擎: use=%v err=%v", use, err)
			}
			o, err := buildDocsAIReadOptions(c)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !clierr.HasKind(err, clierr.KindUsage) {
					t.Fatalf("期望用法错误含 %q，得到 %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDocsAIReadOptions: %v", err)
			}
			tc.check(t, o, buildDocsAIFetchBody(o))
		})
	}
}

func TestDocReadIMMarkdownRejectsLocalEngine(t *testing.T) {
	c := newDocReadTestCmd(t, "--engine", "local", "--doc-format", "im-markdown")
	_, err := wantsDocsAIRead(c)
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "改用 --engine docs_ai") {
		t.Fatalf("本地引擎 + im-markdown 应返回带改用提示的用法错误，得到 %v", err)
	}
	c = newDocReadTestCmd(t, "--engine", "local", "--lang", "en-US")
	if _, err := wantsDocsAIRead(c); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("本地引擎 + --lang 应返回用法错误，得到 %v", err)
	}
}

// TestDocReadIMMarkdownFetch 端到端：请求体 format=markdown + lang，输出经 IM Markdown 降级，
// 文档链接使用输入 URL 的租户域名；-o json 时 document.content 同样被转换。
func TestDocReadIMMarkdownFetch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const serverContent = `<title>周报</title>` + "\n" +
		`<callout emoji="💡">注意<b>风险</b></callout>` + "\n" +
		`见 <cite type="doc" doc-id="doxcnRef" title="设计稿"></cite>，负责人 <cite type="user" user-id="ou_x" user-name="张三"></cite>` + "\n" +
		`<table><tr><th>项</th><th>值</th></tr><tr><td>QPS</td><td>1|2</td></tr></table>`
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		bodies = append(bodies, body)
		resp, _ := json.Marshal(map[string]any{"code": 0, "msg": "", "data": map[string]any{
			"document": map[string]any{"content": serverContent, "revision_id": 3},
		}})
		_, _ = w.Write(resp)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	wantText := "# 周报\n---\n💡 注意**风险**\n---\n见 [设计稿](https://xxx.feishu.cn/docx/doxcnRef)，负责人 <at user_id=\"ou_x\">张三</at>\n" +
		"| 项 | 值 |\n| - | - |\n| QPS | 1\\|2 |\n"

	c := newDocReadTestCmd(t, "--doc-format", "im-markdown", "--lang", "en-US")
	var out bytes.Buffer
	c.SetOut(&out)
	if err := c.RunE(c, []string{"https://xxx.feishu.cn/docx/docABC"}); err != nil {
		t.Fatalf("doc read --doc-format im-markdown 失败: %v", err)
	}
	if len(bodies) != 1 || bodies[0]["format"] != "markdown" || bodies[0]["lang"] != "en-US" {
		t.Fatalf("请求体异常: %#v", bodies)
	}
	if out.String() != wantText {
		t.Fatalf("IM Markdown 输出异常:\n got %q\nwant %q", out.String(), wantText)
	}

	// token 输入：链接回退到品牌标准域名
	c = newDocReadTestCmd(t, "--doc-format", "im-markdown")
	out.Reset()
	c.SetOut(&out)
	if err := c.RunE(c, []string{"docABC"}); err != nil {
		t.Fatalf("token 输入失败: %v", err)
	}
	if !strings.Contains(out.String(), "[设计稿](https://www.feishu.cn/docx/doxcnRef)") {
		t.Fatalf("token 输入应使用品牌标准域名: %q", out.String())
	}

	// -o json：完整响应中的 content 已转换
	c = newDocReadTestCmd(t, "--doc-format", "im-markdown", "-o", "json")
	out.Reset()
	c.SetOut(&out)
	if err := c.RunE(c, []string{"https://xxx.feishu.cn/docx/docABC"}); err != nil {
		t.Fatalf("-o json 失败: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON 输出无法解析: %v\n%s", err, out.String())
	}
	content := parsed["document"].(map[string]any)["content"].(string)
	if content+"\n" != wantText {
		t.Fatalf("JSON content 未转换: %q", content)
	}
}

// TestDocReadMarkdownUnchangedWithoutNewFlags 回归：不带新参数时 markdown 输出保持服务端原文，请求体无 lang。
func TestDocReadMarkdownUnchangedWithoutNewFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const serverContent = `<callout emoji="💡">原样</callout>`
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		resp, _ := json.Marshal(map[string]any{"code": 0, "msg": "", "data": map[string]any{
			"document": map[string]any{"content": serverContent, "revision_id": 3},
		}})
		_, _ = w.Write(resp)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	c := newDocReadTestCmd(t, "--engine", "docs_ai", "--doc-format", "markdown")
	var out bytes.Buffer
	c.SetOut(&out)
	if err := c.RunE(c, []string{"https://xxx.feishu.cn/docx/docABC"}); err != nil {
		t.Fatalf("doc read markdown 失败: %v", err)
	}
	if out.String() != serverContent+"\n" {
		t.Fatalf("markdown 输出不应被改写: %q", out.String())
	}
	if _, ok := gotBody["lang"]; ok || gotBody["format"] != "markdown" {
		t.Fatalf("请求体异常: %#v", gotBody)
	}
}
