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

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newDocReadTestCmd 构造与 docReadCmd 同 flag 集的独立命令，避免全局 flag 状态在测试间串扰。
func newDocReadTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "read", RunE: docReadCmd.RunE, Args: docReadCmd.Args}
	c.Flags().AddFlagSet(docReadCmd.Flags())
	// AddFlagSet 共享 *pflag.Flag：先复位
	docReadCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
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
