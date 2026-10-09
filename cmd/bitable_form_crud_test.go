package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initTestConfig 初始化最小可用 config（仅 app_id/app_secret），供 dry-run 测试使用。
func initTestConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	tmp := t.TempDir()
	cfgFile := tmp + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("app_id: a\napp_secret: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
}

// captureRunE 设置必要 flag 后执行命令的 RunE，捕获 stdout。
func captureRunE(t *testing.T, cmd *cobra.Command, flags map[string]string) (string, error) {
	t.Helper()
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag %s=%s: %v", k, v, err)
		}
	}
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmd.RunE(cmd, nil)
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

// ---- form create body ----

func newFormCreateTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "create", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("name", "", "")
	c.Flags().String("description", "", "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestBuildFormCreateBodyConvenience(t *testing.T) {
	c := newFormCreateTestCmd()
	_ = c.Flags().Set("name", "反馈表单")
	_ = c.Flags().Set("description", "请填写")
	body, err := buildFormCreateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "反馈表单" || body["description"] != "请填写" {
		t.Errorf("body 不对: %v", body)
	}
}

func TestBuildFormCreateBodyRequiresInput(t *testing.T) {
	c := newFormCreateTestCmd()
	if _, err := buildFormCreateBody(c); err == nil {
		t.Error("无任何字段应报错")
	}
}

func TestBuildFormCreateBodyConfigOverride(t *testing.T) {
	c := newFormCreateTestCmd()
	_ = c.Flags().Set("config", `{"name":"from-config"}`)
	body, err := buildFormCreateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["name"] != "from-config" {
		t.Errorf("config 解析不对: %v", body)
	}
}

// ---- form create dry-run path ----

func TestFormCreateDryRunPath(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormCreateCmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "name": "X", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, `"method": "POST"`) ||
		!strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/forms") {
		t.Errorf("form create 路径/方法不对: %s", out)
	}
}

func TestFormDeleteDryRunPath(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormDeleteCmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "form-id": "vew1", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, `"method": "DELETE"`) ||
		!strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/forms/vew1") {
		t.Errorf("form delete 路径/方法不对: %s", out)
	}
}

// ---- form detail / submit (share token, no base-token in path) ----

func TestFormDetailDryRunPathAndBody(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormDetailCmd, map[string]string{
		"share-token": "shrABC", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/tables/forms/detail") {
		t.Errorf("form detail 端点应为 bases/tables/forms/detail（不含 base_token）: %s", out)
	}
	if !strings.Contains(out, `"share_token": "shrABC"`) {
		t.Errorf("form detail body 应含 share_token: %s", out)
	}
}

func TestFormSubmitDryRunWrapsContent(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormSubmitCmd, map[string]string{
		"share-token": "shrABC", "content": `{"评分":5}`, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/tables/forms/submit") {
		t.Errorf("form submit 端点不对: %s", out)
	}
	// 字段值应放进 content 字段，且 share_token 同级
	if !strings.Contains(out, `"content"`) || !strings.Contains(out, `"share_token": "shrABC"`) {
		t.Errorf("form submit body 结构应为 {share_token, content}: %s", out)
	}
}

// TestBuildFormSubmitContentNoUnwrap 验证 form submit 的 content 不做 fields 自动解包：
// content 是裸字段 map，{"fields":{...}} 应原样作为「字段名 fields → 对象值」保留，
// 避免对恰好有名为 fields 字段的表单静默丢数据（见 buildFormSubmitContent 注释）。
func TestBuildFormSubmitContentNoUnwrap(t *testing.T) {
	c := &cobra.Command{Use: "submit", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("content", "", "")
	c.Flags().String("content-file", "", "")

	// {"fields":{...}} 不解包，原样保留外层 fields 键。
	_ = c.Flags().Set("content", `{"fields":{"a":1}}`)
	content, err := buildFormSubmitContent(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m, ok := content.(map[string]any)
	if !ok {
		t.Fatalf("content 应为 map: %v", content)
	}
	inner, ok := m["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields 键应原样保留为对象，不被解包: %v", m)
	}
	if inner["a"] == nil {
		t.Errorf("内层应原样保留 a: %v", inner)
	}

	// 直接的字段 map 原样返回。
	_ = c.Flags().Set("content", `{"评分":5}`)
	content2, err := buildFormSubmitContent(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m2, ok := content2.(map[string]any); !ok || m2["评分"] == nil {
		t.Errorf("直接字段 map 应原样返回: %v", content2)
	}
}

// ---- form questions create / delete ----

func newFormQuestionsCreateTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "create", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("questions", "", "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestBuildFormQuestionsCreateBodyWraps(t *testing.T) {
	c := newFormQuestionsCreateTestCmd()
	_ = c.Flags().Set("questions", `[{"type":"text","title":"名字"}]`)
	body, err := buildFormQuestionsCreateBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body 应为 map: %T", body)
	}
	if arr, ok := m["questions"].([]any); !ok || len(arr) != 1 {
		t.Errorf("--questions 应包成 questions 数组: %v", m["questions"])
	}
}

func TestBuildFormQuestionsCreateBodyRequiresInput(t *testing.T) {
	c := newFormQuestionsCreateTestCmd()
	if _, err := buildFormQuestionsCreateBody(c); err == nil {
		t.Error("无 --questions 也无 --config 应报错")
	}
}

func newFormQuestionsDeleteTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "delete", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("question-ids", "", "")
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	return c
}

func TestBuildFormQuestionsDeleteBodyCSV(t *testing.T) {
	c := newFormQuestionsDeleteTestCmd()
	_ = c.Flags().Set("question-ids", "fld001, fld002")
	body, err := buildFormQuestionsDeleteBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m, _ := body.(map[string]any)
	ids, ok := m["question_ids"].([]string)
	if !ok || len(ids) != 2 || ids[0] != "fld001" || ids[1] != "fld002" {
		t.Errorf("question_ids 解析不对: %v", m["question_ids"])
	}
}

func TestBuildFormQuestionsDeleteBodyJSONArray(t *testing.T) {
	c := newFormQuestionsDeleteTestCmd()
	_ = c.Flags().Set("question-ids", `["fld001","fld002"]`)
	body, err := buildFormQuestionsDeleteBody(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m, _ := body.(map[string]any)
	ids, ok := m["question_ids"].([]string)
	if !ok || len(ids) != 2 || ids[0] != "fld001" || ids[1] != "fld002" {
		t.Errorf("JSON 数组 question_ids 解析不对: %v", m["question_ids"])
	}
}

// TestParseQuestionIDs 验证 CSV 与 JSON 数组两种输入都得到 [a b]
func TestParseQuestionIDs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`a,b`, []string{"a", "b"}},
		{` a , b `, []string{"a", "b"}},
		{`["a","b"]`, []string{"a", "b"}},
		{` [ "a" , "b" ] `, []string{"a", "b"}},
		{`["a","","b"]`, []string{"a", "b"}}, // JSON 数组里空串被过滤
		{``, nil},
	}
	for _, tc := range cases {
		got, err := parseQuestionIDs(tc.in)
		if err != nil {
			t.Errorf("parseQuestionIDs(%q) err: %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseQuestionIDs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseQuestionIDs(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
	// 非法 JSON 数组应报错
	if _, err := parseQuestionIDs(`["a",`); err == nil {
		t.Error("非法 JSON 数组应报错")
	}
}

func TestBuildFormQuestionsDeleteBodyTooMany(t *testing.T) {
	c := newFormQuestionsDeleteTestCmd()
	ids := make([]string, 11)
	for i := range ids {
		ids[i] = "fld"
	}
	_ = c.Flags().Set("question-ids", strings.Join(ids, ","))
	if _, err := buildFormQuestionsDeleteBody(c); err == nil {
		t.Error("超过 10 个应报错")
	}
}

func TestFormQuestionsCreateDryRunCollectionEndpoint(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormQuestionsCreateCmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "form-id": "vew1",
		"questions": `[{"type":"text","title":"t"}]`, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/forms/vew1/questions") {
		t.Errorf("questions create 应走 collection 端点: %s", out)
	}
	if !strings.Contains(out, `"method": "POST"`) {
		t.Errorf("questions create 应为 POST: %s", out)
	}
}

func TestFormQuestionsDeleteDryRunCollectionEndpoint(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableFormQuestionsDeleteCmd, map[string]string{
		"base-token": "bascn1", "table-id": "tbl1", "form-id": "vew1",
		"question-ids": "fld001", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/tables/tbl1/forms/vew1/questions") {
		t.Errorf("questions delete 应走 collection 端点: %s", out)
	}
	if !strings.Contains(out, `"method": "DELETE"`) {
		t.Errorf("questions delete 应为 DELETE: %s", out)
	}
	if !strings.Contains(out, `"question_ids"`) {
		t.Errorf("questions delete body 应含 question_ids: %s", out)
	}
}
