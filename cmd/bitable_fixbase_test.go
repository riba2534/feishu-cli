package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// 非法 --jq（语法错误或未定义函数）必须在任何请求（含 token 获取）之前以用法错误退出：
// 否则写命令会先执行写入、渲染时才报错，脚本按失败重试就会重复创建。
func TestBitableInvalidJQRejectedBeforeRequest(t *testing.T) {
	bt := []string{"--base-token", "bascnxxx", "--as", "bot"}
	cases := [][]string{
		append([]string{"bitable", "block", "create", "--type", "folder", "--name", "x"}, bt...),
		append([]string{"bitable", "record", "list", "--table-id", "tblx"}, bt...),
		append([]string{"bitable", "field", "extension", "update-cells", "--table-id", "tblx", "--field-id", "fldx", "--type", "column", "--yes"}, bt...),
		append([]string{"bitable", "field", "button-rule", "get", "--table-id", "tblx", "--field-id", "fldx"}, bt...),
		append([]string{"bitable", "table", "copy-status", "--task-id", "t1"}, bt...),
		{"bitable", "form", "detail", "--share-token", "shrxxx", "--as", "bot"},
	}
	for _, args := range cases {
		for _, jq := range []string{"bad(", "-r"} {
			for _, dry := range []bool{false, true} {
				hits := isolatePlatformCLIEnv(t)
				full := append(append([]string{}, args...), "--jq", jq)
				if dry {
					full = append(full, "--dry-run")
				}
				_, _, err := runCLI(t, full...)
				if got := exitCodeFor(err); got != clierr.ExitUsage {
					t.Errorf("%v: exit=%d want 2 (err=%v)", full, got, err)
				}
				if err == nil || !strings.Contains(err.Error(), "jq 表达式") {
					t.Errorf("%v: 错误信息应指出 jq 表达式: %v", full, err)
				}
				if n := atomic.LoadInt32(hits); n != 0 {
					t.Errorf("%v: 非法 --jq 不应联网，命中 %d 次", full, n)
				}
			}
		}
	}
}

// 合法 --jq 与不传 --jq 时行为不变：dry-run 预览照常输出。
func TestBitableValidJQDryRunUnchanged(t *testing.T) {
	isolateBitableNoCredEnv(t)
	out, _, err := runCLI(t, "bitable", "record", "list", "--base-token", "bascnxxx", "--table-id", "tblx", "--dry-run", "--jq", ".method")
	if err != nil || strings.TrimSpace(out) != `"GET"` {
		t.Errorf("合法 jq 应照常过滤: out=%q err=%v", out, err)
	}
	out, _, err = runCLI(t, "bitable", "record", "list", "--base-token", "bascnxxx", "--table-id", "tblx", "--dry-run")
	if err != nil || !strings.Contains(out, `"method": "GET"`) {
		t.Errorf("不传 jq 输出应不变: out=%q err=%v", out, err)
	}
}

// fieldRefServer 模拟 base/v3：表/字段 GET 接受名称并返回规范 ID；update_cells / button_rule 只认规范 ID。
func fieldRefServer(t *testing.T, calls *[]string, mu *sync.Mutex, posted *map[string]any) func() {
	return stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		const tables = "/open-apis/base/v3/bases/bascn1/tables/"
		switch {
		case r.Method == http.MethodGet && r.URL.Path == tables+"任务":
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "tblREAL", "name": "任务"}})
		case r.Method == http.MethodGet && (r.URL.Path == tables+"tblREAL/fields/AI总结" || r.URL.Path == tables+"tblREAL/fields/fldREAL"):
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "fldREAL", "name": "AI总结"}})
		case r.Method == http.MethodPost && r.URL.Path == tables+"tblREAL/fields/fldREAL/field_extensions/update_cells":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, posted)
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"task_id": "7700000000000000001"}})
		case r.Method == http.MethodGet && r.URL.Path == tables+"tblREAL/fields/fldREAL/button_rule":
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"bound": false, "target": nil}})
		default:
			// 名称直接进 update_cells/button_rule 路径时服务端的真实表现
			bitableWriteJSON(w, map[string]any{"code": 800004000, "msg": "field not exist"})
		}
	})
}

func newFieldRefTestCmd(kv ...string) *cobra.Command {
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "", "")
		c.Flags().String("field-id", "", "")
		c.Flags().String("type", "", "")
		c.Flags().String("view-id", "", "")
		c.Flags().StringArray("record-id", nil, "")
		c.Flags().String("workflow-id", "", "")
		c.Flags().Bool("yes", false, "")
	})
	output.AddFormatFlags(c)
	output.AddDryRunFlag(c)
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("user-access-token", "u-test")
	_ = c.Flags().Set("yes", "true")
	for i := 0; i+1 < len(kv); i += 2 {
		_ = c.Flags().Set(kv[i], kv[i+1])
	}
	return c
}

// update-cells 传表名 / 字段名时先解析为规范 ID，再 POST 到 ID 路径。
func TestFieldExtensionUpdateCellsResolvesNames(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var posted map[string]any
	defer fieldRefServer(t, &calls, &mu, &posted)()

	c := newFieldRefTestCmd("table-id", "任务", "field-id", "AI总结", "type", "row", "record-id", "recA")
	out, _, err := runCaptured(t, func() error { return bitableFieldExtensionUpdateCellsCmd.RunE(c, nil) })
	if err != nil {
		t.Fatalf("名称应解析后成功: %v (calls=%v)", err, calls)
	}
	want := []string{
		"GET /open-apis/base/v3/bases/bascn1/tables/任务",
		"GET /open-apis/base/v3/bases/bascn1/tables/tblREAL/fields/AI总结",
		"POST /open-apis/base/v3/bases/bascn1/tables/tblREAL/fields/fldREAL/field_extensions/update_cells",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("请求序列不对:\n got=%v\nwant=%v", calls, want)
	}
	if !strings.Contains(out, "7700000000000000001") || posted["type"] != "row" {
		t.Errorf("输出或请求体不对: out=%s body=%v", out, posted)
	}

	// tbl 前缀视为 ID：不再 GET 数据表
	calls = nil
	c = newFieldRefTestCmd("table-id", "tblREAL", "field-id", "fldREAL", "type", "column")
	if _, _, err := runCaptured(t, func() error { return bitableFieldExtensionUpdateCellsCmd.RunE(c, nil) }); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.HasSuffix(calls[0], "/tables/tblREAL") {
		t.Errorf("tbl ID 不应再解析数据表: %v", calls)
	}
}

// update-cells 的 dry-run 计划列出名称解析步骤，且不联网。
func TestFieldExtensionUpdateCellsDryRunShowsResolveSteps(t *testing.T) {
	hits := isolateBitableNoCredEnv(t)
	out, _, err := runCLI(t, "bitable", "field", "extension", "update-cells", "--base-token", "bascnxxx",
		"--table-id", "任务", "--field-id", "摘要", "--type", "column", "--dry-run", "--jq", "[.steps[].path]")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"/open-apis/base/v3/bases/bascnxxx/tables/%E4%BB%BB%E5%8A%A1"`,
		`"/open-apis/base/v3/bases/bascnxxx/tables/<resolved_table_id>/fields/%E6%91%98%E8%A6%81"`,
		`"/open-apis/base/v3/bases/bascnxxx/tables/<resolved_table_id>/fields/<resolved_field_id>/field_extensions/update_cells"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 计划缺少 %s:\n%s", want, out)
		}
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Error("dry-run 不应联网")
	}
}

// button-rule 传表名时先解析为 tbl ID：button_rule 路径不认表名（服务端报 800004007 table not found）。
func TestButtonRuleResolvesTableName(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var posted map[string]any
	defer fieldRefServer(t, &calls, &mu, &posted)()

	c := newFieldRefTestCmd("table-id", "任务", "field-id", "fldREAL")
	out, _, err := runCaptured(t, func() error { return newButtonRuleCmd("get").RunE(c, nil) })
	if err != nil {
		t.Fatalf("表名应解析后成功: %v (calls=%v)", err, calls)
	}
	want := []string{
		"GET /open-apis/base/v3/bases/bascn1/tables/任务",
		"GET /open-apis/base/v3/bases/bascn1/tables/tblREAL/fields/fldREAL",
		"GET /open-apis/base/v3/bases/bascn1/tables/tblREAL/fields/fldREAL/button_rule",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") || !strings.Contains(out, `"bound": false`) {
		t.Errorf("请求序列或输出不对:\n got=%v\nwant=%v\nout=%s", calls, want, out)
	}
}

// 表名解析失败时错误带上 --table-id 上下文，并保留 API 错误链。
func TestResolveBitableTableRefError(t *testing.T) {
	defer stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 800030104, "msg": "not_found"})
	})()
	_, err := resolveBitableTableRef("bascn1", "不存在", "u-test")
	if err == nil || !strings.Contains(err.Error(), `解析 --table-id "不存在" 失败`) || !client.HasAPICode(err, 800030104) {
		t.Errorf("错误应带上下文并保留 API 码: %v", err)
	}
}

// copy-status 遇 800030110（任务结束后状态已清理）给出「通常已完成、去 table list 确认、勿重复提交」提示。
func TestTableCopyStatusGoneHasHint(t *testing.T) {
	defer stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 800030110, "msg": "not_found: The copy-table task status is no longer available."})
	})()
	c := newTableCopyTestCmd("task-id", "t1")
	_, _, err := runCaptured(t, func() error { return bitableTableCopyStatusCmd.RunE(c, nil) })
	if err == nil {
		t.Fatal("应返回错误")
	}
	for _, want := range []string{"table.id", "bitable table list", "请勿重复提交"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("提示缺少 %q: %v", want, err)
		}
	}
	if !client.HasAPICode(err, tableCopyStateGoneCode) {
		t.Errorf("应保留 API 错误链: %v", err)
	}
	if got := exitCodeFor(err); got != 1 {
		t.Errorf("800030110 保持一般错误 exit 1，得到 %d", got)
	}
}

// resolve 的「不支持链接」提示列出 /app/（BaseApp 链接已支持离线解析）。
func TestBitableResolveUnsupportedMentionsApp(t *testing.T) {
	isolateBitableNoCredEnv(t)
	_, _, err := runCLI(t, "bitable", "resolve", "--url", "https://example.feishu.cn/foo/bar")
	if exitCodeFor(err) != clierr.ExitUsage || err == nil || !strings.Contains(err.Error(), "/app/") {
		t.Errorf("应 exit 2 且列出 /app/: %v", err)
	}
}

// help 文案与实际行为一致：auto 已配置但不可用时 fail-closed；workflow create 需要 client_token。
func TestBitableHelpMatchesBehavior(t *testing.T) {
	if f := bitableCmd.PersistentFlags().Lookup("as"); f == nil || f.Usage != identityAsFlagHelp {
		t.Errorf("bitable --as 帮助应复用 identityAsFlagHelp")
	}
	if strings.Contains(bitableCmd.Long, "过期自动回落") || !strings.Contains(bitableCmd.Long, "fail-closed") {
		t.Errorf("bitable 根帮助的 auto 描述与 fail-closed 行为不符")
	}
	if !strings.Contains(bitableWorkflowCreateCmd.Long, `"client_token"`) {
		t.Errorf("workflow create 示例应带 client_token")
	}
}
