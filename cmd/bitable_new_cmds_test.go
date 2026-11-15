package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// ---------- field extension ----------

func TestParseFieldExtensionUpdateBody(t *testing.T) {
	ok := []string{
		`{}`,
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"总结："},{"type":"field_ref","field":"描述"}]}}`,
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":""}]}}`,
	}
	for _, raw := range ok {
		if _, err := parseFieldExtensionUpdateBody(raw); err != nil {
			t.Errorf("%s 应通过: %v", raw, err)
		}
	}
	bad := map[string]string{
		`[]`:                       "解析 --config 失败",
		`null`:                     "必须是 JSON 对象",
		`{"extension_id":"other"}`: "必须是",
		`{"inputs":{"prompt":[]}}`: "extension_id",
		`{"extension_id":"builtin_llm_completion"}`:                                                                   "inputs 必填",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[]}}`:                                            "至少需要一段",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text"}]}}`:                             "必须带 text",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"a","field":"b"}]}}`:      "field 只能用于",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"field_ref","field":""}]}}`:             "必须带 field",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"field_ref","field":"a","text":"b"}]}}`: "text 只能用于",
		`{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"img"}]}}`:                              "只能是 text 或 field_ref",
		`{"extension_id":"builtin_llm_completion","unknown":1}`:                                                       "unknown field",
		`{} {}`: "只能包含一个 JSON 对象",
	}
	for raw, msg := range bad {
		_, err := parseFieldExtensionUpdateBody(raw)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: 应报用法错误含 %q，得到 %v", raw, msg, err)
		}
	}
	// 清除时请求体就是 {}；正常配置按结构体规范化
	body, _ := parseFieldExtensionUpdateBody(ok[1])
	b, _ := json.Marshal(body)
	if string(b) != `{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"总结："},{"type":"field_ref","field":"描述"}]}}` {
		t.Errorf("规范化 body 不对: %s", b)
	}
}

func TestBuildFieldExtensionUpdateCellsBody(t *testing.T) {
	newCmd := func(kv ...string) *cobra.Command {
		c := &cobra.Command{Use: "x"}
		c.Flags().String("type", "", "")
		c.Flags().String("view-id", "", "")
		c.Flags().StringArray("record-id", nil, "")
		for i := 0; i+1 < len(kv); i += 2 {
			_ = c.Flags().Set(kv[i], kv[i+1])
		}
		return c
	}
	body, err := buildFieldExtensionUpdateCellsBody(newCmd("type", "column", "view-id", "vew1"))
	if err != nil || body["type"] != "column" || body["view_id"] != "vew1" || body["record_ids"] != nil {
		t.Errorf("column body 不对: %v %v", body, err)
	}
	body, err = buildFieldExtensionUpdateCellsBody(newCmd("type", "row", "record-id", "recA", "record-id", "recB"))
	if err != nil || len(body["record_ids"].([]string)) != 2 {
		t.Errorf("row body 不对: %v %v", body, err)
	}
	for _, kv := range [][]string{
		{"type", "row"},
		{"type", "row", "record-id", "recA", "view-id", "v"},
		{"type", "column", "record-id", "recA"},
		{"type", "row", "record-id", "recA", "record-id", "recA"},
		{"type", "cell"},
	} {
		if _, err := buildFieldExtensionUpdateCellsBody(newCmd(kv...)); !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v 应报用法错误，得到 %v", kv, err)
		}
	}
}

// 新增高危命令：非交互且未带 --yes 时 exit 10，不发任何请求；--dry-run 不经过门禁。
func TestBitableNewHighRiskCommandsRequireConfirmation(t *testing.T) {
	base := []string{"--base-token", "bascnxxx", "--as", "bot"}
	for _, args := range [][]string{
		append([]string{"bitable", "block", "delete", "--block-id", "blk1"}, base...),
		append([]string{"bitable", "field", "extension", "update", "--table-id", "tbl1", "--field-id", "fld1", "--config", "{}"}, base...),
		append([]string{"bitable", "field", "extension", "update-cells", "--table-id", "tbl1", "--field-id", "fld1", "--type", "column"}, base...),
	} {
		hits := isolatePlatformCLIEnv(t)
		_, _, err := runCLI(t, args...)
		if got := exitCodeFor(err); got != clierr.ExitConfirmationRequired {
			t.Errorf("%v: exit=%d want 10 (err=%v)", args, got, err)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v: 未确认不应联网，命中 %d 次", args, n)
		}
		hits = isolateBitableNoCredEnv(t)
		if _, _, err := runCLI(t, append(args, "--dry-run")...); err != nil {
			t.Errorf("%v --dry-run 应不经门禁直接预览: %v", args, err)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v --dry-run 不应联网，命中 %d 次", args, n)
		}
	}
}

// --yes 后真正发出写请求，路径与方法正确。
func TestBitableBlockDeleteWithYesSendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "blk1"}})
	})
	defer cleanup()
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("block-id", "", "")
		c.Flags().Bool("yes", false, "")
	})
	output.AddDryRunFlag(c)
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("block-id", "blk1")
	_ = c.Flags().Set("user-access-token", "u-test")
	_ = c.Flags().Set("yes", "true")
	if _, _, err := runCaptured(t, func() error { return bitableBlockDeleteCmd.RunE(c, nil) }); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/open-apis/base/v3/bases/bascn1/blocks/blk1" {
		t.Errorf("请求不对: %s %s", gotMethod, gotPath)
	}
}

// ---------- button-rule ----------

func TestButtonRuleBindResolvesFieldThenPuts(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var putBody map[string]any
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/fields/审批按钮"):
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "fldBTN", "type": "button"}})
		case r.Method == http.MethodPut && r.URL.Path == "/open-apis/base/v3/bases/bascn1/tables/tbl1/fields/fldBTN/button_rule":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &putBody)
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"bound": true}})
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
			http.Error(w, "x", 500)
		}
	})
	defer cleanup()
	for _, action := range []string{"bind", "unbind"} {
		calls, putBody = nil, nil
		cmd := newButtonRuleCmd(action)
		c := newBitableTestCmd(func(c *cobra.Command) {
			c.Flags().String("table-id", "", "")
			c.Flags().String("field-id", "", "")
			c.Flags().String("workflow-id", "", "")
		})
		_ = c.Flags().Set("base-token", "bascn1")
		_ = c.Flags().Set("table-id", "tbl1")
		_ = c.Flags().Set("field-id", "审批按钮")
		_ = c.Flags().Set("workflow-id", "wkfABC")
		_ = c.Flags().Set("user-access-token", "u-test")
		out, _, err := runCaptured(t, func() error { return cmd.RunE(c, nil) })
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		want := "wkfABC"
		if action == "unbind" {
			want = ""
		}
		if len(calls) != 2 || putBody["workflow_id"] != want || !strings.Contains(out, `"bound": true`) {
			t.Errorf("%s: calls=%v body=%v out=%s", action, calls, putBody, out)
		}
	}
}

func TestButtonRuleRejectsNonWkfWorkflowID(t *testing.T) {
	hits := isolateBitableNoCredEnv(t)
	_, _, err := runCLI(t, "bitable", "field", "button-rule", "bind", "--base-token", "bascnx", "--table-id", "t", "--field-id", "f", "--workflow-id", "7012345")
	if got := exitCodeFor(err); got != clierr.ExitUsage || !strings.Contains(err.Error(), "wkf 前缀") {
		t.Errorf("非 wkf 前缀应 exit 2: %d %v", got, err)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Error("用法错误不应联网")
	}
}

// ---------- table copy ----------

func newTableCopyTestCmd(kv ...string) *cobra.Command {
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "", "")
		c.Flags().String("name", "", "")
		c.Flags().String("range", tableCopyRangeSchema, "")
		c.Flags().Bool("wait", false, "")
		c.Flags().Duration("timeout", 5*time.Minute, "")
		c.Flags().String("task-id", "", "")
	})
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("user-access-token", "u-test")
	for i := 0; i+1 < len(kv); i += 2 {
		_ = c.Flags().Set(kv[i], kv[i+1])
	}
	return c
}

func TestTableCopySchemaAndAllWithWait(t *testing.T) {
	oldInit, oldMax := tableCopyPollInitial, tableCopyPollMax
	tableCopyPollInitial, tableCopyPollMax = time.Millisecond, 2*time.Millisecond
	defer func() { tableCopyPollInitial, tableCopyPollMax = oldInit, oldMax }()

	var polls int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/base/v3/bases/bascn1/tables/tbl1/copy":
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			if body["range"] == "schema" {
				bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"table": map[string]any{"id": "tblNEW", "name": body["name"]}, "state": "success"}})
				return
			}
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"table": map[string]any{"id": "tblALL"}, "state": "init", "task_id": "task 1"}})
		case "/open-apis/base/v3/bases/bascn1/copy_table_state":
			state := "process"
			if atomic.AddInt32(&polls, 1) >= 2 {
				state = "success"
			}
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"table_id": "tblALL", "state": state}})
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	})
	defer cleanup()

	out, _, err := runCaptured(t, func() error {
		return bitableTableCopyCmd.RunE(newTableCopyTestCmd("table-id", "tbl1", "name", "副本"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	if got["completed"] != true || got["state"] != "success" || got["table"].(map[string]any)["id"] != "tblNEW" {
		t.Errorf("schema 复制输出不对: %s", out)
	}

	// --range all 不 --wait：返回 next_command，shell 安全引用 task_id
	out, _, err = runCaptured(t, func() error {
		return bitableTableCopyCmd.RunE(newTableCopyTestCmd("table-id", "tbl1", "name", "全量", "range", "all"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"next_command": "feishu-cli bitable table copy-status --base-token bascn1 --task-id 'task 1' --as auto"`) {
		t.Errorf("未完成任务应给 next_command: %s", out)
	}

	// --wait：轮询到 success
	out, _, err = runCaptured(t, func() error {
		return bitableTableCopyCmd.RunE(newTableCopyTestCmd("table-id", "tbl1", "name", "全量", "range", "all", "wait", "true"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	_ = json.Unmarshal([]byte(out), &got)
	if got["completed"] != true || got["next_command"] != nil || atomic.LoadInt32(&polls) < 2 {
		t.Errorf("--wait 应轮询到 success: %s polls=%d", out, polls)
	}
}

func TestTableCopySubmitNetworkErrorSaysOutcomeUnknown(t *testing.T) {
	err := tableCopySubmitError(&net.OpError{Op: "read", Err: errors.New("connection reset by peer")})
	if err == nil || !strings.Contains(err.Error(), "结果未知") || !strings.Contains(err.Error(), "请勿直接重试") {
		t.Fatalf("网络错误应提示结果未知、勿重试: %v", err)
	}
	if exitCodeFor(err) != clierr.ExitNetwork {
		t.Errorf("仍保留网络错误链（exit 4），得到 %d", exitCodeFor(err))
	}
	plain := errors.New("base/v3 API 失败: code=1254000")
	if got := tableCopySubmitError(plain); got != plain {
		t.Errorf("非网络错误应原样返回: %v", got)
	}
}

func TestTableCopyValidation(t *testing.T) {
	isolateBitableNoCredEnv(t)
	base := []string{"bitable", "table", "copy", "--base-token", "bascnx", "--table-id", "t", "--name", "n"}
	for _, extra := range [][]string{
		{"--wait"},
		{"--range", "all", "--timeout", "1m"},
		{"--range", "all", "--wait", "--timeout", "31m"},
		{"--range", "rows"},
	} {
		_, _, err := runCLI(t, append(append([]string{}, base...), extra...)...)
		if exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("%v 应 exit 2: %v", extra, err)
		}
	}
}

func TestTableCopyStatusInvalidTaskIsUsageError(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 800010109, "msg": "invalid task id"})
	})
	defer cleanup()
	c := newTableCopyTestCmd("task-id", "bad")
	_, _, err := runCaptured(t, func() error { return bitableTableCopyStatusCmd.RunE(c, nil) })
	if exitCodeFor(err) != clierr.ExitUsage {
		t.Errorf("800010109 应归为用法错误: %v", err)
	}
}

func TestPollTableCopyTimesOut(t *testing.T) {
	oldInit := tableCopyPollInitial
	tableCopyPollInitial = time.Millisecond
	defer func() { tableCopyPollInitial = oldInit }()
	state, timedOut, err := pollTableCopy(context.Background(), 20*time.Millisecond, func() (string, error) { return "process", nil })
	if err != nil || !timedOut || state != "process" {
		t.Errorf("应超时返回最后状态: %q %v %v", state, timedOut, err)
	}
	_, _, err = pollTableCopy(context.Background(), time.Second, func() (string, error) { return "", errors.New("boom") })
	if err == nil {
		t.Error("非网络错误应立即返回")
	}
}

// ---------- template / block ----------

func TestBitableTemplateAndBlockDryRun(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"bitable", "template", "categories"}, []string{`"path": "/open-apis/base/v3/bases/templates/category"`}},
		{[]string{"bitable", "template", "list", "--category-key", "office", "--offset", "cur1"}, []string{`"category_key": "office"`, `"offset": "cur1"`, `"limit": 10`}},
		{[]string{"bitable", "template", "search", "--keyword", "项目管理", "--limit", "5"}, []string{`"keyword": "项目管理"`, `"limit": 5`, `/bases/templates/search`}},
		{[]string{"bitable", "block", "create", "--base-token", "bascnx", "--type", "folder", "--name", "文档"}, []string{`"POST"`, `/bases/bascnx/blocks"`, `"type": "folder"`}},
		{[]string{"bitable", "block", "move", "--base-token", "bascnx", "--block-id", "b1"}, []string{`/blocks/b1/move`, `"parent_id": null`}},
		{[]string{"bitable", "block", "move", "--base-token", "bascnx", "--block-id", "b1", "--parent-id", "f1", "--after-id", "b2"}, []string{`"parent_id": "f1"`, `"after_id": "b2"`}},
		{[]string{"bitable", "block", "rename", "--base-token", "bascnx", "--block-id", "b1", "--name", "新"}, []string{`/blocks/b1/rename`, `"name": "新"`}},
		{[]string{"bitable", "copy", "--base-token", "bascnx", "--name", "副本", "--time-zone", "Asia/Shanghai"}, []string{`"time_zone": "Asia/Shanghai"`}},
	}
	for _, tc := range cases {
		isolateBitableNoCredEnv(t)
		out, _, err := runCLI(t, append(tc.args, "--dry-run")...)
		if err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v: 预览缺少 %s:\n%s", tc.args, w, out)
			}
		}
	}
	for _, args := range [][]string{
		{"bitable", "template", "search", "--keyword", " "},
		{"bitable", "template", "list", "--limit", "101"},
		{"bitable", "block", "create", "--base-token", "b", "--type", "sheet", "--name", "x"},
		{"bitable", "block", "move", "--base-token", "b", "--block-id", "x", "--before-id", "a", "--after-id", "c"},
		{"bitable", "block", "rename", "--base-token", "b", "--block-id", "x", "--name", " "},
	} {
		isolateBitableNoCredEnv(t)
		_, _, err := runCLI(t, args...)
		if exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("%v 应 exit 2: %v", args, err)
		}
	}
}

// --wait 轮询：task_id 来自提交响应，800010109 不是用户的用法错误；终态 failed 不提示续查。
func TestTableCopyWaitServerErrors(t *testing.T) {
	oldInit, oldMax := tableCopyPollInitial, tableCopyPollMax
	tableCopyPollInitial, tableCopyPollMax = time.Millisecond, 2*time.Millisecond
	defer func() { tableCopyPollInitial, tableCopyPollMax = oldInit, oldMax }()
	for _, tc := range []struct {
		name  string
		state func(w http.ResponseWriter)
		check func(t *testing.T, out string, err error)
	}{
		{"invalid-task", func(w http.ResponseWriter) {
			bitableWriteJSON(w, map[string]any{"code": 800010109, "msg": "invalid task id"})
		}, func(t *testing.T, out string, err error) {
			if err == nil || exitCodeFor(err) == clierr.ExitUsage || strings.Contains(err.Error(), "--task-id 无效") {
				t.Fatalf("--wait 中服务端错误不应报 --task-id 用法错误: %v", err)
			}
		}},
		{"failed", func(w http.ResponseWriter) {
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"table_id": "tblALL", "state": "failed"}})
		}, func(t *testing.T, out string, err error) {
			if err == nil || strings.Contains(err.Error(), "请勿重复提交") || !strings.Contains(err.Error(), "重新提交") {
				t.Fatalf("终态失败应提示可重新提交: %v", err)
			}
			if strings.Contains(out, "poll_status") {
				t.Fatalf("终态失败不应提示续查: %s", out)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/copy") {
					bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"table": map[string]any{"id": "tblALL"}, "state": "init", "task_id": "t1"}})
					return
				}
				tc.state(w)
			})
			defer cleanup()
			out, _, err := runCaptured(t, func() error {
				return bitableTableCopyCmd.RunE(newTableCopyTestCmd("table-id", "tbl1", "name", "全量", "range", "all", "wait", "true"), nil)
			})
			tc.check(t, out, err)
		})
	}
}
