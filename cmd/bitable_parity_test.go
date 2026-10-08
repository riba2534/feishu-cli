package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// ---------- 测试辅助 ----------

// newBitableTestCmd 构造一个只带指定 flag 的临时命令，供直接调用全局命令的 RunE（避免全局 flag 状态串扰）。
func newBitableTestCmd(setup func(c *cobra.Command)) *cobra.Command {
	c := &cobra.Command{Use: "t", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("base-token", "", "")
	c.Flags().String("user-access-token", "", "")
	c.Flags().String("as", "auto", "")
	if setup != nil {
		setup(c)
	}
	return c
}

// runCaptured 执行 fn 并捕获 stdout / stderr。
func runCaptured(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { _, _ = io.Copy(&outBuf, rOut); wg.Done() }()
	go func() { _, _ = io.Copy(&errBuf, rErr); wg.Done() }()
	err := fn()
	_ = wOut.Close()
	_ = wErr.Close()
	wg.Wait()
	os.Stdout, os.Stderr = oldOut, oldErr
	return outBuf.String(), errBuf.String(), err
}

func bitableWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------- record list 分页（P0-1） ----------

// recordPage 构造一页 record list 矩阵响应。
func recordPage(ids []string, hasMore bool, rev int) map[string]any {
	rows := make([]any, 0, len(ids))
	idList := make([]any, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, []any{"v-" + id})
		idList = append(idList, id)
	}
	return map[string]any{
		"data": rows, "record_id_list": idList, "has_more": hasMore, "rev": json.Number(strconv.Itoa(rev)),
		"fields": []any{"名称"}, "field_id_list": []any{"fld1"}, "field_type_list": []any{"text"},
		"timezone": "UTC",
	}
}

func TestFetchRecordListSinglePageAddsNextOffset(t *testing.T) {
	var warn bytes.Buffer
	var gotParams map[string]any
	fetch := func(p map[string]any) (map[string]any, error) {
		gotParams = p
		return recordPage([]string{"r1", "r2"}, true, 1), nil
	}
	out, err := fetchRecordList(fetch, map[string]any{"view_id": "v1"}, recordListOptions{Offset: 10, Limit: 2}, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if gotParams["offset"] != 10 || gotParams["limit"] != 2 || gotParams["view_id"] != "v1" {
		t.Errorf("请求参数不对: %v", gotParams)
	}
	if out["next_offset"] != 12 {
		t.Errorf("has_more=true 时应输出 next_offset=12，got %v", out["next_offset"])
	}
	if !strings.Contains(warn.String(), "--offset 12") || !strings.Contains(warn.String(), "--page-all") {
		t.Errorf("stderr 应提示续翻方式: %q", warn.String())
	}
}

func TestFetchRecordListPageAllMerges(t *testing.T) {
	pages := map[int]map[string]any{
		0: recordPage([]string{"r1", "r2"}, true, 5),
		2: recordPage([]string{"r3", "r4"}, true, 5),
		4: recordPage([]string{"r5"}, false, 5),
	}
	var offsets []int
	fetch := func(p map[string]any) (map[string]any, error) {
		off := p["offset"].(int)
		offsets = append(offsets, off)
		return pages[off], nil
	}
	var warn bytes.Buffer
	out, err := fetchRecordList(fetch, nil, recordListOptions{Limit: 2, PageAll: true}, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(offsets) != "[0 2 4]" {
		t.Errorf("应按 offset 0,2,4 翻页，got %v", offsets)
	}
	ids := out["record_id_list"].([]any)
	rows := out["data"].([]any)
	if len(ids) != 5 || len(rows) != 5 || ids[4] != "r5" {
		t.Errorf("合并结果不对: ids=%v rows=%d", ids, len(rows))
	}
	if out["has_more"] != false {
		t.Errorf("取完后 has_more 应为 false")
	}
	if _, ok := out["next_offset"]; ok {
		t.Errorf("取完后不应有 next_offset")
	}
	if _, ok := out["rev_changed"]; ok || warn.Len() != 0 {
		t.Errorf("rev 未变化不应告警: %q", warn.String())
	}
}

func TestFetchRecordListPageAllRevChanged(t *testing.T) {
	pages := map[int]map[string]any{
		0: recordPage([]string{"r1"}, true, 5),
		1: recordPage([]string{"r2"}, false, 6),
	}
	fetch := func(p map[string]any) (map[string]any, error) { return pages[p["offset"].(int)], nil }
	var warn bytes.Buffer
	out, err := fetchRecordList(fetch, nil, recordListOptions{Limit: 1, PageAll: true}, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if out["rev_changed"] != true || !strings.Contains(warn.String(), "rev 5 → 6") {
		t.Errorf("rev 变化应告警并标记 rev_changed: out=%v warn=%q", out["rev_changed"], warn.String())
	}
}

func TestFetchRecordListPageAllSchemaChanged(t *testing.T) {
	second := recordPage([]string{"r2"}, false, 5)
	second["fields"] = []any{"新名称"}
	pages := map[int]map[string]any{0: recordPage([]string{"r1"}, true, 5), 1: second}
	fetch := func(p map[string]any) (map[string]any, error) { return pages[p["offset"].(int)], nil }
	_, err := fetchRecordList(fetch, nil, recordListOptions{Limit: 1, PageAll: true}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "列结构") {
		t.Fatalf("分页期间表结构变化应报错，got %v", err)
	}
}

func TestFetchRecordListPageAllEmptyPageWithHasMore(t *testing.T) {
	calls := 0
	fetch := func(p map[string]any) (map[string]any, error) {
		calls++
		if p["offset"].(int) == 0 {
			return recordPage([]string{"r1"}, true, 1), nil
		}
		return recordPage(nil, true, 1), nil
	}
	_, err := fetchRecordList(fetch, nil, recordListOptions{Limit: 1, PageAll: true}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "死循环") || calls != 2 {
		t.Fatalf("空页 + has_more=true 应立即停止报错，calls=%d err=%v", calls, err)
	}
}

func TestFetchRecordListPageAllMaxPages(t *testing.T) {
	fetch := func(p map[string]any) (map[string]any, error) {
		off := p["offset"].(int)
		return recordPage([]string{fmt.Sprintf("r%d", off)}, true, 1), nil
	}
	var warn bytes.Buffer
	out, err := fetchRecordList(fetch, nil, recordListOptions{Limit: 1, PageAll: true, MaxPages: 3}, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(out["record_id_list"].([]any)) != 3 || out["has_more"] != true || out["next_offset"] != 3 {
		t.Errorf("达到页数上限应保留 has_more/next_offset: %v", out)
	}
	if !strings.Contains(warn.String(), "翻页上限") {
		t.Errorf("应在 stderr 告警: %q", warn.String())
	}
}

// TestRecordListRunEDefaultLimit 锁住 P0-1：不传 --limit 时显式下发 limit=100
// （服务端不传 limit 只返回 20 条），has_more 时输出 next_offset。
func TestRecordListRunEDefaultLimit(t *testing.T) {
	var gotLimit, gotOffset string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		gotOffset = r.URL.Query().Get("offset")
		bitableWriteJSON(w, map[string]any{"code": 0, "data": recordPage([]string{"r1"}, true, 1)})
	})
	defer cleanup()

	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "", "")
		c.Flags().String("view-id", "", "")
		c.Flags().Int("offset", 0, "")
		c.Flags().Int("limit", recordListDefaultLimit, "")
		c.Flags().Bool("page-all", false, "")
		c.Flags().String("filter-json", "", "")
		c.Flags().String("sort-json", "", "")
		c.Flags().StringArray("field-id", nil, "")
	})
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("table-id", "tbl1")
	_ = c.Flags().Set("user-access-token", "u-test")
	out, stderr, err := runCaptured(t, func() error { return bitableRecordListCmd.RunE(c, nil) })
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotLimit != "100" || gotOffset != "0" {
		t.Errorf("默认应下发 limit=100 offset=0，got limit=%q offset=%q", gotLimit, gotOffset)
	}
	if !strings.Contains(out, `"next_offset": 1`) || !strings.Contains(stderr, "has_more=true") {
		t.Errorf("应输出 next_offset 并提示: out=%s stderr=%s", out, stderr)
	}
}

func TestRecordListRunEValidatesLimitAndSort(t *testing.T) {
	newCmd := func() *cobra.Command {
		return newBitableTestCmd(func(c *cobra.Command) {
			c.Flags().String("table-id", "tbl1", "")
			c.Flags().String("view-id", "", "")
			c.Flags().Int("offset", 0, "")
			c.Flags().Int("limit", recordListDefaultLimit, "")
			c.Flags().Bool("page-all", false, "")
			c.Flags().String("filter-json", "", "")
			c.Flags().String("sort-json", "", "")
			c.Flags().StringArray("field-id", nil, "")
		})
	}
	c := newCmd()
	_ = c.Flags().Set("limit", "2001")
	if err := bitableRecordListCmd.RunE(c, nil); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Errorf("--limit 2001 应为用法错误，got %v", err)
	}
	c = newCmd()
	sorts, _ := json.Marshal(make([]map[string]any, 11))
	_ = c.Flags().Set("sort-json", string(sorts))
	if err := bitableRecordListCmd.RunE(c, nil); err == nil || !strings.Contains(err.Error(), "最多 10 条") {
		t.Errorf("排序条件超过 10 条应本地报错，got %v", err)
	}
}

// ---------- table / field / view 列表翻页（P0-2） ----------

// TestListBaseV3ItemsFetchesAllByTotal 服务端只给 total 不给 has_more，且每页实际条数可能小于请求的 limit：
// 必须按 total 翻页取完，并按 id 去重（实测字段列表跨页顺序不稳定）。
func TestListBaseV3ItemsFetchesAllByTotal(t *testing.T) {
	all := []string{"f1", "f2", "f3", "f4", "f5"}
	var offsets []string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		if r.URL.Query().Get("limit") != strconv.Itoa(baseV3ListPageLimit) {
			t.Errorf("每页应请求 limit=%d，got %s", baseV3ListPageLimit, r.URL.Query().Get("limit"))
		}
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		// 服务端每页最多给 2 条；第二页故意重复一个已出现的元素
		var items []any
		for i := off; i < off+2 && i < len(all); i++ {
			items = append(items, map[string]any{"id": all[i]})
		}
		if off == 2 {
			items = append([]any{map[string]any{"id": "f1"}}, items[1:]...)
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"fields": items, "total": len(all)}})
	})
	defer cleanup()

	var items []any
	var total int
	_, stderr, err := runCaptured(t, func() error {
		var e error
		items, total, e = listBaseV3Items("/open-apis/base/v3/bases/b/tables/t/fields", "fields", "u-test")
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(offsets) != "[0 2 4]" {
		t.Errorf("应按 offset 0,2,4 翻页，got %v", offsets)
	}
	if total != 5 || len(items) != 4 {
		t.Errorf("去重后 4 项（f3 被重复的 f1 顶掉），total=5，got len=%d total=%d", len(items), total)
	}
	if !strings.Contains(stderr, "去重后只取到 4 项") {
		t.Errorf("去重后少于 total 应告警: %q", stderr)
	}
}

func TestFieldListRunEReturnsAllFields(t *testing.T) {
	calls := 0
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/open-apis/base/v3/bases/bascn1/tables/tbl1/fields" {
			t.Errorf("path = %s", r.URL.Path)
		}
		items := make([]any, 0, 150)
		for i := 0; i < 150; i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("fld%03d", i)})
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"fields": items, "total": 150}})
	})
	defer cleanup()
	c := newBitableTestCmd(func(c *cobra.Command) { c.Flags().String("table-id", "", "") })
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("table-id", "tbl1")
	_ = c.Flags().Set("user-access-token", "u-test")
	out, _, err := runCaptured(t, func() error { return bitableFieldListCmd.RunE(c, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Fields []any `json:"fields"`
		Total  int   `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if len(got.Fields) != 150 || got.Total != 150 || calls != 1 {
		t.Errorf("应一次取回 150 个字段，got %d/%d calls=%d", len(got.Fields), got.Total, calls)
	}
}

// ---------- batch-delete 分批（P0-3） ----------

func TestRunRecordBatchDeleteChunks(t *testing.T) {
	ids := make([]string, 450)
	for i := range ids {
		ids[i] = fmt.Sprintf("rec%03d", i)
	}
	var sizes []int
	call := func(chunk []string) (map[string]any, error) {
		sizes = append(sizes, len(chunk))
		echo := make([]any, 0, len(chunk))
		for _, id := range chunk {
			echo = append(echo, id)
		}
		return map[string]any{"record_id_list": echo}, nil
	}
	var progress bytes.Buffer
	out, err := runRecordBatchDelete(ids, maxRecordBatchDelete, call, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(sizes) != "[200 200 50]" {
		t.Errorf("应按 200 条一批串行提交，got %v", sizes)
	}
	if len(out["record_id_list"].([]any)) != 450 || out["batch_count"] != 3 {
		t.Errorf("合并结果不对: %v", out["batch_count"])
	}
	if !strings.Contains(progress.String(), "第 3/3 批") {
		t.Errorf("分批时应提示进度: %q", progress.String())
	}
}

func TestRunRecordBatchDeleteStopsOnFailure(t *testing.T) {
	ids := make([]string, 450)
	for i := range ids {
		ids[i] = fmt.Sprintf("rec%03d", i)
	}
	calls := 0
	call := func(chunk []string) (map[string]any, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("base/v3 API 失败: code=1254291, msg=conflict")
		}
		return map[string]any{"record_id_list": []any{}}, nil
	}
	_, err := runRecordBatchDelete(ids, maxRecordBatchDelete, call, io.Discard)
	if err == nil || calls != 2 {
		t.Fatalf("第 2 批失败应立即停止，calls=%d err=%v", calls, err)
	}
	if !strings.Contains(err.Error(), "共 200 条已删除") || !strings.Contains(err.Error(), "剩余 250 条未提交") {
		t.Errorf("错误应注明已删除与未提交的条数: %v", err)
	}
}

func TestRunRecordBatchDeleteSingleBatchKeepsResponse(t *testing.T) {
	call := func(chunk []string) (map[string]any, error) {
		return map[string]any{"record_id_list": []any{"rec1"}, "extra": "x"}, nil
	}
	out, err := runRecordBatchDelete([]string{"rec1"}, maxRecordBatchDelete, call, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if out["extra"] != "x" || out["batch_count"] != nil {
		t.Errorf("单批时应原样输出服务端响应: %v", out)
	}
}

// ---------- 表单问题删除（P0-4） ----------

func newFQDeleteRunCmd() *cobra.Command {
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "", "")
		c.Flags().String("form-id", "", "")
		c.Flags().String("question-ids", "", "")
		c.Flags().String("config", "", "")
		c.Flags().String("config-file", "", "")
		c.Flags().Bool("keep-field", false, "")
		c.Flags().Bool("yes", false, "")
		output.AddFormatFlags(c)
		output.AddDryRunFlag(c)
	})
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("table-id", "tbl1")
	_ = c.Flags().Set("form-id", "vew1")
	_ = c.Flags().Set("question-ids", "fldA,fldB")
	_ = c.Flags().Set("user-access-token", "u-test")
	return c
}

func TestFormQuestionsDeleteRequiresConfirmation(t *testing.T) {
	calls := 0
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{}})
	})
	defer cleanup()
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	defer func() { confirmIsInteractive = origInteractive }()

	c := newFQDeleteRunCmd()
	_, stderr, err := runCaptured(t, func() error { return bitableFormQuestionsDeleteCmd.RunE(c, nil) })
	if err == nil || !clierr.HasKind(err, clierr.KindConfirmationRequired) {
		t.Fatalf("不带 --keep-field 的非交互删除必须要求确认（exit 10），got %v", err)
	}
	if calls != 0 {
		t.Errorf("未确认时不能发请求，calls=%d", calls)
	}
	if !strings.Contains(stderr, "整列全部记录数据") && !strings.Contains(stderr, "该列全部记录数据") {
		t.Errorf("stderr 应明确提示会删除整列数据: %q", stderr)
	}
}

func TestFormQuestionsDeleteKeepFieldSkipsConfirmation(t *testing.T) {
	var body map[string]any
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{}})
	})
	defer cleanup()
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	defer func() { confirmIsInteractive = origInteractive }()

	c := newFQDeleteRunCmd()
	_ = c.Flags().Set("keep-field", "true")
	if _, _, err := runCaptured(t, func() error { return bitableFormQuestionsDeleteCmd.RunE(c, nil) }); err != nil {
		t.Fatalf("--keep-field 不需要确认: %v", err)
	}
	if body["keep_field"] != true || fmt.Sprint(body["question_ids"]) != "[fldA fldB]" {
		t.Errorf("请求体应带 keep_field=true: %v", body)
	}
}

func TestFormQuestionsDeleteWithYesAndDryRun(t *testing.T) {
	calls := 0
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{}})
	})
	defer cleanup()
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	defer func() { confirmIsInteractive = origInteractive }()

	c := newFQDeleteRunCmd()
	_ = c.Flags().Set("dry-run", "true")
	out, _, err := runCaptured(t, func() error { return bitableFormQuestionsDeleteCmd.RunE(c, nil) })
	if err != nil || calls != 0 || !strings.Contains(out, `"method": "DELETE"`) {
		t.Fatalf("dry-run 应只预览不确认不发请求: err=%v calls=%d out=%s", err, calls, out)
	}

	c = newFQDeleteRunCmd()
	_ = c.Flags().Set("yes", "true")
	if _, _, err := runCaptured(t, func() error { return bitableFormQuestionsDeleteCmd.RunE(c, nil) }); err != nil || calls != 1 {
		t.Fatalf("--yes 应直接执行: err=%v calls=%d", err, calls)
	}
}

func TestBuildFormQuestionsDeleteBodyKeepFieldWithConfig(t *testing.T) {
	c := newFQDeleteRunCmd()
	_ = c.Flags().Set("question-ids", "")
	_ = c.Flags().Set("config", `{"question_ids":["fldA"]}`)
	_ = c.Flags().Set("keep-field", "true")
	body, err := buildFormQuestionsDeleteBody(c)
	if err != nil {
		t.Fatal(err)
	}
	if !formQuestionsDeleteKeepsField(body) {
		t.Errorf("--keep-field 应写入 --config 请求体: %v", body)
	}
}

// ---------- create/copy 平铺响应（P0-5） ----------

func TestPrintBaseSummaryFlatResponse(t *testing.T) {
	data := map[string]any{"base_token": "bascnNEW", "name": "项目", "url": "https://example.feishu.cn/base/bascnNEW"}
	out, _, _ := runCaptured(t, func() error { printBaseSummary(data, "base_token"); return nil })
	for _, want := range []string{"base_token: bascnNEW", "项目", "https://example.feishu.cn/base/bascnNEW"} {
		if !strings.Contains(out, want) {
			t.Errorf("文本模式应打印 %q（base/v3 create 响应是平铺的），got:\n%s", want, out)
		}
	}
	// 兼容旧的 {base:{...}} 形状
	out, _, _ = runCaptured(t, func() error {
		printBaseSummary(map[string]any{"base": map[string]any{"base_token": "bascnOLD", "name": "旧"}}, "new base_token")
		return nil
	})
	if !strings.Contains(out, "bascnOLD") || !strings.Contains(out, "旧") {
		t.Errorf("嵌套形状也应能打印: %s", out)
	}
}

// TestSetupFirstBitableTableWithFields --fields：新建自定义表后删除默认表。
func TestSetupFirstBitableTableWithFields(t *testing.T) {
	var steps []string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		steps = append(steps, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tables"):
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"tables": []any{map[string]any{"id": "tblDefault"}}, "total": 1}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tables"):
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"fields"`) || !strings.Contains(string(raw), `"任务"`) {
				t.Errorf("建表请求体应含 name 与 fields: %s", raw)
			}
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "tblNew", "name": "任务", "fields": []any{map[string]any{"id": "fld1"}}}})
		case r.Method == http.MethodDelete:
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{}})
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
		}
	})
	defer cleanup()
	orig := bitableCreateDefaultTableDeleteDelay
	bitableCreateDefaultTableDeleteDelay = 0
	defer func() { bitableCreateDefaultTableDeleteDelay = orig }()

	data := map[string]any{"base_token": "bascn1"}
	if err := setupFirstBitableTable(data, "bascn1", "任务", []any{map[string]any{"name": "标题", "type": "text"}}, "u-test"); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 || steps[2] != "DELETE /open-apis/base/v3/bases/bascn1/tables/tblDefault" {
		t.Errorf("应 GET 默认表 → POST 新表 → DELETE 默认表，got %v", steps)
	}
	if data["default_table_deleted"] != true || data["deleted_default_table_id"] != "tblDefault" {
		t.Errorf("输出字段不对: %v", data)
	}
}

func TestSetupFirstBitableTableRenameOnly(t *testing.T) {
	var patched string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"tables": []any{map[string]any{"id": "tblDefault"}}, "total": 1}})
		case http.MethodPatch:
			patched = r.URL.Path
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": "tblDefault", "name": "主表"}})
		default:
			t.Errorf("仅 --table-name 时不应 %s", r.Method)
		}
	})
	defer cleanup()
	data := map[string]any{}
	if err := setupFirstBitableTable(data, "bascn1", "主表", nil, "u-test"); err != nil {
		t.Fatal(err)
	}
	if patched != "/open-apis/base/v3/bases/bascn1/tables/tblDefault" || data["default_table_renamed"] != true {
		t.Errorf("应重命名默认表: patched=%s data=%v", patched, data)
	}
}

func TestParseBitableFieldsArray(t *testing.T) {
	if _, err := parseBitableFieldsArray(`[]`, "--fields"); err == nil {
		t.Error("空数组应报错")
	}
	if _, err := parseBitableFieldsArray(`[1]`, "--fields"); err == nil {
		t.Error("非对象元素应报错")
	}
	if items, err := parseBitableFieldsArray(`[{"name":"a","type":"text"}]`, "--fields"); err != nil || len(items) != 1 {
		t.Errorf("合法数组应通过: %v %v", items, err)
	}
}

// ---------- 视图配置解包（P0-6） ----------

func TestViewConfigGetUnwrapsArrayData(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/base/v3/bases/bascn1/tables/tbl1/views/vew1/group" {
			t.Errorf("path = %s", r.URL.Path)
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "msg": "", "data": []any{map[string]any{"field": "状态", "desc": false}}})
	})
	defer cleanup()
	getCmd := newViewConfigCmd("group", "get")
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "", "")
		c.Flags().String("view-id", "", "")
	})
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("table-id", "tbl1")
	_ = c.Flags().Set("view-id", "vew1")
	_ = c.Flags().Set("user-access-token", "u-test")
	out, _, err := runCaptured(t, func() error { return getCmd.RunE(c, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %s", out)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("应输出解包后的数组，而不是 {code,data,msg} 信封: %s", out)
	}
}

func TestViewConfigSetWrapsVisibleFieldsArray(t *testing.T) {
	var body map[string]any
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		bitableWriteJSON(w, map[string]any{"code": 0, "data": []any{"名称"}})
	})
	defer cleanup()
	setCmd := newViewConfigCmd("visible-fields", "set")
	c := newBitableTestCmd(func(c *cobra.Command) {
		c.Flags().String("table-id", "tbl1", "")
		c.Flags().String("view-id", "vew1", "")
		c.Flags().String("config", "", "")
		c.Flags().String("config-file", "", "")
	})
	_ = c.Flags().Set("base-token", "bascn1")
	_ = c.Flags().Set("user-access-token", "u-test")
	_ = c.Flags().Set("config", `["名称"]`)
	if _, _, err := runCaptured(t, func() error { return setCmd.RunE(c, nil) }); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(body["visible_fields"]) != "[名称]" {
		t.Errorf("数组应自动包成 {visible_fields:[...]}: %v", body)
	}
}

// ---------- URL 解析（P1-8） ----------

func TestResolveBaseTokenAcceptsURLs(t *testing.T) {
	c := newBitableTestCmd(nil)
	_ = c.Flags().Set("base-token", "https://example.feishu.cn/base/bascnABC?table=tbl1&view=vew1")
	if got, err := resolveBaseToken(c); err != nil || got != "bascnABC" {
		t.Errorf("/base/ URL 应解析出 token，got %q %v", got, err)
	}
	_ = c.Flags().Set("base-token", "bascnRAW")
	if got, _ := resolveBaseToken(c); got != "bascnRAW" {
		t.Errorf("裸 token 原样返回，got %q", got)
	}
	for _, bad := range []string{
		"https://example.feishu.cn/record/shrX",
		"https://example.feishu.cn/docx/doxX",
		"https://evil.example.com/base/bascnABC",
	} {
		_ = c.Flags().Set("base-token", bad)
		if _, err := resolveBaseToken(c); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s 应为用法错误，got %v", bad, err)
		}
	}
}

func TestResolveBaseTokenWikiURL(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/wiki/v2/spaces/node_by_token") || r.URL.Query().Get("token") != "wikNODE" {
			t.Errorf("应走 node_by_token: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"node_token": "wikNODE", "obj_type": "bitable", "obj_token": "bascnWIKI"}}})
	})
	defer cleanup()
	c := newBitableTestCmd(nil)
	_ = c.Flags().Set("base-token", "https://example.feishu.cn/wiki/wikNODE")
	_ = c.Flags().Set("user-access-token", "u-test")
	got, _, err := func() (string, string, error) {
		var tok string
		_, se, e := runCaptured(t, func() error { var e error; tok, e = resolveBaseToken(c); return e })
		return tok, se, e
	}()
	if err != nil || got != "bascnWIKI" {
		t.Fatalf("wiki URL 应换成底层 base_token，got %q %v", got, err)
	}
}

func TestResolveBaseTokenWikiNotBitable(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"node_token": "wikNODE", "obj_type": "docx", "obj_token": "doxX"}}})
	})
	defer cleanup()
	c := newBitableTestCmd(nil)
	_ = c.Flags().Set("base-token", "https://example.feishu.cn/wiki/wikNODE")
	_ = c.Flags().Set("user-access-token", "u-test")
	if _, err := resolveBaseToken(c); err == nil || !strings.Contains(err.Error(), "不是多维表格") {
		t.Fatalf("wiki 底层不是 bitable 应报错，got %v", err)
	}
}

func TestBitableURLKind(t *testing.T) {
	cases := map[string]string{
		"https://example.feishu.cn/base/bascnX?table=tbl":          "base_url",
		"https://example.feishu.cn/wiki/wikX":                      "wiki_url",
		"https://example.feishu.cn/record/shrX":                    "record_share_url",
		"https://example.feishu.cn/share/base/form/shrF":           "form_share_url",
		"https://example.feishu.cn/share/base/shrF":                "form_share_url",
		"https://example.feishu.cn/share/base/view/shrV":           "view_share_url",
		"https://example.feishu.cn/share/base/dashboard/shrD":      "dashboard_share_url",
		"https://example.feishu.cn/base/workspace/wsX":             "workspace_url",
		"https://example.feishu.cn/docx/doxX?from=/base/bascnFAKE": "",
	}
	for raw, want := range cases {
		_, kind, err := parseBitableResolveURL(raw)
		if err != nil {
			t.Errorf("%s: %v", raw, err)
			continue
		}
		if kind != want {
			t.Errorf("%s: kind=%q want %q", raw, kind, want)
		}
	}
	if got := bitableFormShareToken("/share/base/form/shrF"); got != "shrF" {
		t.Errorf("form share token = %q", got)
	}
	if got := bitableFormShareToken("/share/base/shrG"); got != "shrG" {
		t.Errorf("form share token = %q", got)
	}
}

func newResolveTestCmd(url string) *cobra.Command {
	c := &cobra.Command{Use: "resolve", Run: func(*cobra.Command, []string) {}}
	c.Flags().String("url", url, "")
	c.Flags().String("user-access-token", "u-test", "")
	c.Flags().String("as", "auto", "")
	output.AddFormatFlags(c)
	output.AddDryRunFlag(c)
	return c
}

// TestResolveBaseURLSelectsBlockType ?table= 是顶层块 ID，需 blocks/list 判型后才输出 table_id / dashboard_id。
func TestResolveBaseURLSelectsBlockType(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/open-apis/base/v3/bases/bascnX/blocks/list" {
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"blocks": []any{
			map[string]any{"id": "tblA", "type": "table", "name": "订单"},
			map[string]any{"id": "blkD", "type": "dashboard", "name": "看板"},
		}}})
	})
	defer cleanup()

	out, _, err := runCaptured(t, func() error {
		return bitableResolveCmd.RunE(newResolveTestCmd("https://example.feishu.cn/base/bascnX?table=tblA&view=vewV&record=recR"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	if got["block_type"] != "table" || got["table_id"] != "tblA" || got["view_id"] != "vewV" || got["record_id"] != "recR" || got["base_token"] != "bascnX" {
		t.Errorf("数据表块解析不对: %v", got)
	}

	out, _, err = runCaptured(t, func() error {
		return bitableResolveCmd.RunE(newResolveTestCmd("https://example.feishu.cn/base/bascnX?table=blkD"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	_ = json.Unmarshal([]byte(out), &got)
	if got["block_type"] != "dashboard" || got["dashboard_id"] != "blkD" || got["table_id"] != nil {
		t.Errorf("仪表盘块不应输出 table_id: %v", got)
	}
}

func TestResolveRecordShareURL(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/base/v3/record_share/shrREC/meta" {
			t.Errorf("path = %s", r.URL.Path)
		}
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"base_token": "bascnX", "table_id": "tblA", "record_id": "recR", "record_share_token": "shrREC"}})
	})
	defer cleanup()
	out, _, err := runCaptured(t, func() error {
		return bitableResolveCmd.RunE(newResolveTestCmd("https://example.feishu.cn/record/shrREC"), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"record_id": "recR"`) || !strings.Contains(out, `"table_id": "tblA"`) {
		t.Errorf("记录分享链接解析不对: %s", out)
	}
}

func TestResolveUnsupportedShareURL(t *testing.T) {
	err := bitableResolveCmd.RunE(newResolveTestCmd("https://example.feishu.cn/share/base/view/shrV"), nil)
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "视图分享") {
		t.Fatalf("视图分享链接应明确报用法错误，got %v", err)
	}
}

func TestFilterBitableBlocks(t *testing.T) {
	data := map[string]any{"blocks": []any{
		map[string]any{"id": "a", "type": "table"}, map[string]any{"id": "b", "type": "dashboard"},
	}, "total": 2}
	filterBitableBlocks(data, "dashboard")
	if data["total"] != 1 || len(data["blocks"].([]any)) != 1 {
		t.Errorf("按 --type 过滤不对: %v", data)
	}
}

// ---------- page_token 自动翻页（P1-12） ----------

func TestFetchBitablePageTokenList(t *testing.T) {
	var tokens []string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		pt, _ := body["page_token"].(string)
		tokens = append(tokens, pt)
		if body["status"] != "enabled" {
			t.Errorf("过滤参数应保留: %v", body)
		}
		switch pt {
		case "":
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": []any{"w1"}, "has_more": true, "page_token": "p2"}})
		case "p2":
			bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"items": []any{"w2"}, "has_more": false, "page_token": ""}})
		}
	})
	defer cleanup()
	out, err := fetchBitablePageTokenList(bitablePageTokenList{Method: "POST", Path: "/open-apis/base/v3/bases/b/workflows/list", Key: "items", Params: map[string]any{"status": "enabled"}, PageSize: 1}, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(tokens) != "[ p2]" || fmt.Sprint(out["items"]) != "[w1 w2]" || out["total"] != 2 || out["has_more"] != false {
		t.Errorf("翻页结果不对: tokens=%v out=%v", tokens, out)
	}
}

func TestFetchBitablePageTokenListRepeatedToken(t *testing.T) {
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		bitableWriteJSON(w, map[string]any{"code": 0, "data": map[string]any{"forms": []any{"f"}, "has_more": true, "page_token": "same"}})
	})
	defer cleanup()
	_, err := fetchBitablePageTokenList(bitablePageTokenList{Method: "GET", Path: "/open-apis/base/v3/bases/b/tables/t/forms", Key: "forms"}, "u-test")
	if err == nil || !strings.Contains(err.Error(), "重复的 page_token") {
		t.Fatalf("重复游标应停止，got %v", err)
	}
}

// ---------- data-query / workflow 本地校验 ----------

func TestValidateDataQueryDSL(t *testing.T) {
	if err := validateDataQueryDSL(map[string]any{"dimensions": []any{}}); err == nil || !strings.Contains(err.Error(), "datasource") {
		t.Errorf("缺 datasource 应报错，got %v", err)
	}
	if err := validateDataQueryDSL(map[string]any{"datasource": map[string]any{}}); err == nil {
		t.Error("缺 dimensions/measures 应报错")
	}
	if err := validateDataQueryDSL(map[string]any{"datasource": map[string]any{}, "measures": []any{}}); err != nil {
		t.Errorf("合法 DSL 应通过: %v", err)
	}
}
