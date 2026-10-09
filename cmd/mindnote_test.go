package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newMindnoteTestCmd 构造与 base 同 flag 集的独立命令；共享的 *pflag.Flag 在测试前后复位。
func newMindnoteTestCmd(t *testing.T, base *cobra.Command, args ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	reset := func() {
		base.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
	}
	reset()
	t.Cleanup(reset)
	c := &cobra.Command{Use: base.Use, RunE: base.RunE, Args: base.Args}
	c.Flags().AddFlagSet(base.Flags())
	if err := c.Flags().Parse(args); err != nil {
		t.Fatalf("解析参数失败: %v", err)
	}
	var out, errOut bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errOut)
	return c, &out, &errOut
}

type mindnoteRecordedRequest struct {
	method, path, query, auth string
	body                      map[string]any
}

// newMindnoteTestServer 模拟 tenant token、node_by_token（底层类型 objType）与思维笔记节点接口。
func newMindnoteTestServer(t *testing.T, objType, objToken string, handle func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*httptest.Server, func() []mindnoteRecordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []mindnoteRecordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		var body map[string]any
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		mu.Lock()
		reqs = append(reqs, mindnoteRecordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), body})
		mu.Unlock()
		if r.URL.Path == client.WikiNodeByTokenPath {
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"node":{"space_id":"sp-1","node_token":%q,"obj_token":%q,"obj_type":%q,"title":"T"}}}`,
				r.URL.Query().Get("token"), objToken, objType)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/open-apis/mindnote/v1/mindnotes/") && handle != nil {
			handle(w, r, body)
			return
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server, func() []mindnoteRecordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]mindnoteRecordedRequest(nil), reqs...)
	}
}

const mindnoteListFixture = `{"code":0,"msg":"success","data":{"nodes":[
 {"node_id":"n1","texts":[{"element_type":"text","text":{"content":"中心主题"}}]},
 {"node_id":"n2","parent_id":"n1","finish":true,"highlight":"yellow",
  "texts":[{"element_type":"text","text":{"content":"负责人 "}},{"element_type":"user","mention_user":{"user_id":"ou_x"}}],
  "notes":[{"element_type":"text","text":{"content":"周五前"}}],"images":[{"token":"img_1"}]},
 {"node_id":"n3","parent_id":"n2","texts":[{"element_type":"link","link":{"text":"设计稿","url":"https://example.com"}}]},
 {"node_id":"n4","parent_id":"missing","texts":[]}
]}}`

func TestMindnoteNodesListWikiUnwrapTreeAndJSON(t *testing.T) {
	server, reqs := newMindnoteTestServer(t, "mindnote", "MnObjTok", func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		_, _ = fmt.Fprint(w, mindnoteListFixture)
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	c, out, _ := newMindnoteTestCmd(t, mindnoteNodesListCmd, "--user-access-token", "u-test", "--user-id-type", "union_id")
	if err := c.RunE(c, []string{"https://example.feishu.cn/wiki/WikTok"}); err != nil {
		t.Fatalf("nodes list 失败: %v", err)
	}
	got := reqs()
	if len(got) != 2 || got[0].path != client.WikiNodeByTokenPath ||
		got[1].method != "GET" || got[1].path != "/open-apis/mindnote/v1/mindnotes/MnObjTok/nodes" ||
		got[1].query != "user_id_type=union_id" || got[1].auth != "Bearer u-test" {
		t.Fatalf("请求异常（应先解包 wiki，再以 User 身份读取 obj_token）: %+v", got)
	}
	want := "思维笔记 MnObjTok：4 个节点\n" +
		"- 中心主题  [n1]\n" +
		"  - 负责人 @ou_x  [n2]  (已完成, 高亮:yellow, 图片×1)\n" +
		"      备注: 周五前\n" +
		"    - 设计稿  [n3]\n" +
		"- （空节点）  [n4]\n"
	if out.String() != want {
		t.Fatalf("节点树输出异常:\n got %q\nwant %q", out.String(), want)
	}

	c, out, _ = newMindnoteTestCmd(t, mindnoteNodesListCmd, "--mindnote-id", "MnBare", "--as", "bot", "-o", "json")
	if err := c.RunE(c, nil); err != nil {
		t.Fatalf("nodes list -o json 失败: %v", err)
	}
	last := reqs()[len(reqs())-1]
	if last.path != "/open-apis/mindnote/v1/mindnotes/MnBare/nodes" || last.query != "" || last.auth != "Bearer t-test" {
		t.Fatalf("--as bot 应使用 App Token 且不带 user_id_type: %+v", last)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON 输出无法解析: %v\n%s", err, out.String())
	}
	if nodes, _ := parsed["nodes"].([]any); len(nodes) != 4 {
		t.Fatalf("JSON 应输出接口原始 data: %s", out.String())
	}
}

func TestMindnoteNodesListRejectsNonMindnote(t *testing.T) {
	server, reqs := newMindnoteTestServer(t, "docx", "DocObjTok", nil)
	initWikiNodeDeleteTestConfig(t, server.URL)

	c, _, _ := newMindnoteTestCmd(t, mindnoteNodesListCmd, "--user-access-token", "u-test")
	err := c.RunE(c, []string{"https://example.feishu.cn/wiki/WikTok"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "不是思维笔记") {
		t.Fatalf("wiki 底层为 docx 时应返回用法错误，得到 %v", err)
	}
	if got := reqs(); len(got) != 1 || got[0].path != client.WikiNodeByTokenPath {
		t.Fatalf("类型不符时不应调用思维笔记接口: %+v", got)
	}

	// 非思维笔记链接、参数冲突与缺参在本地拒绝，不发请求
	for _, tc := range []struct {
		args    []string
		pos     []string
		wantErr string
	}{
		{pos: []string{"https://example.feishu.cn/docx/DocABC"}, wantErr: "仅支持 mindnote"},
		{pos: []string{"MnA"}, args: []string{"--mindnote-id", "MnB"}, wantErr: "不一致"},
		{wantErr: "缺少思维笔记"},
		{pos: []string{"MnA"}, args: []string{"--user-id-type", "email"}, wantErr: "--user-id-type"},
		{pos: []string{"MnA"}, args: []string{"--as", "root"}, wantErr: "--as 仅支持"},
		{pos: []string{"MnA"}, args: []string{"-o", "yaml"}, wantErr: "仅支持 json"},
	} {
		c, _, _ := newMindnoteTestCmd(t, mindnoteNodesListCmd, tc.args...)
		err := c.RunE(c, tc.pos)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%v %v 应返回含 %q 的用法错误，得到 %v", tc.pos, tc.args, tc.wantErr, err)
		}
	}
	if got := reqs(); len(got) != 1 {
		t.Fatalf("本地校验失败时不应发请求: %+v", got)
	}
}

func TestBuildMindnoteCreateBody(t *testing.T) {
	cases := []struct {
		data, clientToken string
		want              map[string]any
		wantErr           string
	}{
		{data: `{"nodes":[{"parent_id":"p1","highlight":"blue"}]}`, clientToken: "ct-1",
			want: map[string]any{"client_token": "ct-1", "nodes": []any{map[string]any{"parent_id": "p1", "highlight": "blue"}}}},
		{data: `{"client_token":"ct-1","nodes":[{"node_id":"n1","finish":true}]}`, clientToken: "ct-1",
			want: map[string]any{"client_token": "ct-1", "nodes": []any{map[string]any{"node_id": "n1", "finish": true}}}},
		{data: `{"nodes":[{"node_id":"n1"}]}`, want: map[string]any{"nodes": []any{map[string]any{"node_id": "n1"}}}},
		{data: ``, wantErr: "缺少 --data"},
		{data: `[]`, wantErr: "必须是 JSON 对象"},
		{data: `{}`, wantErr: "nodes 必须是非空数组"},
		{data: `{"nodes":[]}`, wantErr: "nodes 必须是非空数组"},
		{data: `{"nodes":[1]}`, wantErr: "nodes[0] 必须是 JSON 对象"},
		{data: `{"nodes":[{"highlight":"purple"}]}`, wantErr: "highlight"},
		{data: `{"nodes":[{}]} {}`, wantErr: "多余内容"},
		{data: `{"nodes":`, wantErr: "不是合法 JSON"},
		{data: `{"client_token":"","nodes":[{}]}`, wantErr: "client_token 必须是非空字符串"},
		{data: `{"client_token":"a","nodes":[{}]}`, clientToken: "b", wantErr: "不一致"},
	}
	for _, tc := range cases {
		t.Run(tc.data+"|"+tc.clientToken, func(t *testing.T) {
			args := []string{"--data", tc.data}
			if tc.clientToken != "" {
				args = append(args, "--client-token", tc.clientToken)
			}
			c, _, _ := newMindnoteTestCmd(t, mindnoteNodesCreateCmd, args...)
			body, err := buildMindnoteCreateBody(c)
			if tc.wantErr != "" {
				if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望含 %q 的用法错误，得到 body=%v err=%v", tc.wantErr, body, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildMindnoteCreateBody: %v", err)
			}
			if !reflect.DeepEqual(body, tc.want) {
				t.Fatalf("body = %#v, want %#v", body, tc.want)
			}
		})
	}
}

func TestMindnoteNodesCreateDryRunMakesNoRequest(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "dry-run 不应发请求", http.StatusInternalServerError)
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	// --as user 未配置 User Token 也必须成功（dry-run 不解析身份）；--as bot 也不得联网解包 wiki
	for _, as := range []string{"bot", "user"} {
		c, _, _ := newMindnoteTestCmd(t, mindnoteNodesCreateCmd,
			"--data", `{"nodes":[{"node_id":"n1"}]}`, "--as", as, "--dry-run")
		if err := c.RunE(c, []string{"https://example.feishu.cn/wiki/WikTok"}); err != nil {
			t.Fatalf("--as %s dry-run 失败: %v", as, err)
		}
	}
	c, out, errOut := newMindnoteTestCmd(t, mindnoteNodesCreateCmd,
		"--data", `{"nodes":[{"node_id":"n1","finish":true}]}`, "--user-id-type", "open_id", "--as", "user", "--dry-run")
	if err := c.RunE(c, []string{"https://example.feishu.cn/wiki/WikTok"}); err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if hits != 0 {
		t.Fatalf("dry-run 发出了 %d 个请求", hits)
	}
	var plan struct {
		DryRun bool             `json:"dry_run"`
		Steps  []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatalf("dry-run 输出无法解析: %v\n%s", err, out.String())
	}
	if !plan.DryRun || len(plan.Steps) != 2 || plan.Steps[0]["path"] != client.WikiNodeByTokenPath ||
		plan.Steps[1]["path"] != "/open-apis/mindnote/v1/mindnotes/<resolved_mindnote_token>/nodes" ||
		plan.Steps[1]["params"].(map[string]any)["user_id_type"] != "open_id" {
		t.Fatalf("dry-run 计划异常: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "未提供 client_token") {
		t.Fatalf("缺 client_token 时应在 stderr 提示: %q", errOut.String())
	}
}

func TestMindnoteNodesCreateRequest(t *testing.T) {
	server, reqs := newMindnoteTestServer(t, "mindnote", "MnObjTok", func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		data := map[string]any{"ids": []string{"n9"}}
		if ct, ok := body["client_token"]; ok {
			data["client_token"] = ct
		}
		resp, _ := json.Marshal(map[string]any{"code": 0, "msg": "success", "data": data})
		_, _ = w.Write(resp)
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	dataFile := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(dataFile, []byte(`{"nodes":[{"parent_id":"n1","texts":[{"element_type":"text","text":{"content":"子节点"}}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, out, errOut := newMindnoteTestCmd(t, mindnoteNodesCreateCmd, "--data", "@"+dataFile, "--client-token", "ct-9", "--as", "bot")
	if err := c.RunE(c, []string{"https://example.feishu.cn/mindnotes/MnTok"}); err != nil {
		t.Fatalf("nodes create 失败: %v", err)
	}
	got := reqs()
	if len(got) != 1 || got[0].method != "POST" || got[0].path != "/open-apis/mindnote/v1/mindnotes/MnTok/nodes" || got[0].auth != "Bearer t-test" {
		t.Fatalf("请求异常（/mindnotes/ 链接不应解包 wiki，--as bot 用 App Token）: %+v", got)
	}
	wantBody := map[string]any{"client_token": "ct-9", "nodes": []any{map[string]any{
		"parent_id": "n1", "texts": []any{map[string]any{"element_type": "text", "text": map[string]any{"content": "子节点"}}},
	}}}
	if !reflect.DeepEqual(got[0].body, wantBody) {
		t.Fatalf("请求体 = %#v, want %#v", got[0].body, wantBody)
	}
	if !strings.Contains(out.String(), "n9") || !strings.Contains(out.String(), "client_token: ct-9") {
		t.Fatalf("输出异常: %q", out.String())
	}
	if strings.Contains(errOut.String(), "未提供 client_token") {
		t.Fatalf("已提供 client_token 时不应提示: %q", errOut.String())
	}

	// stdin 输入 + 更新节点
	old := slidesStdin
	slidesStdin = strings.NewReader(`{"nodes":[{"node_id":"n1","finish":true}]}`)
	t.Cleanup(func() { slidesStdin = old })
	c, out, _ = newMindnoteTestCmd(t, mindnoteNodesCreateCmd, "--data", "-", "--as", "bot", "-o", "json")
	if err := c.RunE(c, []string{"MnTok"}); err != nil {
		t.Fatalf("stdin 输入失败: %v", err)
	}
	last := reqs()[len(reqs())-1]
	if !reflect.DeepEqual(last.body, map[string]any{"nodes": []any{map[string]any{"node_id": "n1", "finish": true}}}) {
		t.Fatalf("stdin 请求体异常: %#v", last.body)
	}
	if !strings.Contains(out.String(), `"ids"`) {
		t.Fatalf("-o json 应输出接口 data: %q", out.String())
	}
}

func TestMindnoteNotFoundHint(t *testing.T) {
	server, _ := newMindnoteTestServer(t, "mindnote", "MnObjTok", func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":3410003,"msg":"resource not found"}`)
	})
	initWikiNodeDeleteTestConfig(t, server.URL)

	c, _, _ := newMindnoteTestCmd(t, mindnoteNodesListCmd, "--as", "bot")
	err := c.RunE(c, []string{"WikTokAsBare"})
	if err == nil || !client.HasAPICode(err, 3410003) || !strings.Contains(err.Error(), "drive inspect") {
		t.Fatalf("3410003 应保留业务码并附带解包提示，得到 %v", err)
	}
}
