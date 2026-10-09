package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// TestFindByStartEndUsesNearestEndAnchor 结束锚点取起点之后最近的一次出现（此前取最后一次，可删到文末）。
func TestFindByStartEndUsesNearestEndAnchor(t *testing.T) {
	children := []*larkdocx.Block{
		makeTextBlock("b0", "前言"),
		makeTextBlock("b1", "开头锚点 A1"),
		makeTextBlock("b2", "中间"),
		makeTextBlock("b3", "结束锚点 Z9 第一次"),
		makeTextBlock("b4", "无关正文"),
		makeTextBlock("b5", "结束锚点 Z9 第二次"),
	}
	ranges, err := findByStartEnd(children, "A1", "Z9")
	if err != nil {
		t.Fatalf("findByStartEnd 失败: %v", err)
	}
	if len(ranges) != 1 || ranges[0].startIndex != 1 || ranges[0].endIndex != 4 {
		t.Fatalf("范围 = %+v，期望 [1,4)（取最近的结束锚点，不能吞掉 b4/b5）", ranges)
	}
}

func TestFindByStartEndSameBlockAndMultipleRanges(t *testing.T) {
	children := []*larkdocx.Block{
		makeTextBlock("b0", "开始 X 结束"),
		makeTextBlock("b1", "中间"),
		makeTextBlock("b2", "开始"),
		makeTextBlock("b3", "结束"),
	}
	ranges, err := findByStartEnd(children, "开始", "结束")
	if err != nil {
		t.Fatalf("findByStartEnd 失败: %v", err)
	}
	if len(ranges) != 2 || ranges[0] != (blockRange{0, 1}) || ranges[1] != (blockRange{2, 4}) {
		t.Fatalf("范围 = %+v，期望 [{0 1} {2 4}]", ranges)
	}
}

const twoMatchChildren = `[
	{"block_id":"h1","block_type":4,"heading2":{"elements":[{"text_run":{"content":"待办"}}]}},
	{"block_id":"p1","block_type":2,"text":{"elements":[{"text_run":{"content":"甲 关键字 乙"}}]}},
	{"block_id":"h2","block_type":4,"heading2":{"elements":[{"text_run":{"content":"待办"}}]}},
	{"block_id":"p2","block_type":2,"text":{"elements":[{"text_run":{"content":"丙 关键字 丁"}}]}}
]`

// TestBlockModesRejectMultipleHits 块级 replace_range / delete_range / insert_* 命中多处时报错，不再静默取第一处。
func TestBlockModesRejectMultipleHits(t *testing.T) {
	cases := []struct {
		name string
		p    contentUpdateParams
	}{
		{"replace_range 标题", contentUpdateParams{mode: "replace_range", selByTitle: "## 待办", content: "x"}},
		{"delete_range 标题", contentUpdateParams{mode: "delete_range", selByTitle: "## 待办"}},
		{"insert_after 文本", contentUpdateParams{mode: "insert_after", selEllipsis: "关键字", content: "x"}},
		{"insert_before 文本", contentUpdateParams{mode: "insert_before", selEllipsis: "关键字", content: "x"}},
		{"replace_range 范围", contentUpdateParams{mode: "replace_range", selEllipsis: "甲...乙", content: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeDocsAI(t, "")
			f.children = twoMatchChildren
			if tc.name == "replace_range 范围" {
				// 两处 "甲...乙"
				f.children = strings.Replace(twoMatchChildren, "丙 关键字 丁", "甲 再次 乙", 1)
			}
			p := tc.p
			_, _, err := runParams(&p)
			if err == nil || !strings.Contains(err.Error(), "命中 2 处") {
				t.Fatalf("应报多处命中错误，得到: %v", err)
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误: %v", err)
			}
			if len(f.bodies) != 0 {
				t.Fatalf("不应发出写请求，实际 %d 次", len(f.bodies))
			}
		})
	}
}

func TestBlockIDLocatorsWireBody(t *testing.T) {
	cases := []struct {
		name  string
		p     contentUpdateParams
		check func(t *testing.T, b map[string]any)
	}{
		{"replace_range 多块", contentUpdateParams{mode: "replace_range", blockID: "blkA,blkB", content: "新段落"}, func(t *testing.T, b map[string]any) {
			if b["command"] != "block_replace" || b["block_id"] != "blkA,blkB" || b["content"] != "新段落" {
				t.Fatalf("body = %#v", b)
			}
		}},
		{"delete_range 区间", contentUpdateParams{mode: "delete_range", startBlockID: "blkA", endBlockID: "-1"}, func(t *testing.T, b map[string]any) {
			if b["command"] != "block_delete" || b["start_block_id"] != "blkA" || b["end_block_id"] != "-1" {
				t.Fatalf("body = %#v", b)
			}
			if _, ok := b["content"]; ok {
				t.Fatalf("block_delete 不应带 content: %#v", b)
			}
		}},
		{"insert_after 锚点", contentUpdateParams{mode: "insert_after", blockID: "0", content: "开头"}, func(t *testing.T, b map[string]any) {
			if b["command"] != "block_insert_after" || b["block_id"] != "0" {
				t.Fatalf("body = %#v", b)
			}
		}},
		{"block_move_after", contentUpdateParams{mode: "block_move_after", blockID: "blkT", srcBlockIDs: "blkA,blkB"}, func(t *testing.T, b map[string]any) {
			if b["command"] != "block_move_after" || b["block_id"] != "blkT" || b["src_block_ids"] != "blkA,blkB" {
				t.Fatalf("body = %#v", b)
			}
		}},
		{"block_copy_insert_after xml", contentUpdateParams{mode: "block_copy_insert_after", blockID: "blkT", srcBlockIDs: "blkA", docFormat: "xml"}, func(t *testing.T, b map[string]any) {
			if b["command"] != "block_copy_insert_after" || b["format"] != "xml" {
				t.Fatalf("body = %#v", b)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeDocsAI(t, "")
			p := tc.p
			if _, _, err := runParams(&p); err != nil {
				t.Fatalf("执行失败: %v", err)
			}
			if len(f.bodies) != 1 {
				t.Fatalf("PUT 次数 = %d", len(f.bodies))
			}
			tc.check(t, f.bodies[0])
		})
	}
}

// TestInsertBeforeBlockIDUsesPreviousSibling insert_before --block-id 映射为"前一个兄弟块之后"。
func TestInsertBeforeBlockIDUsesPreviousSibling(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/blkB"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"blkB","parent_id":"doc","block_type":2}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/doc/children"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"block_id":"blkA","block_type":2},{"block_id":"blkB","block_type":2}],"has_more":false}}`)
		case r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"revision_id":2},"result":"success"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	if _, _, err := runParams(&contentUpdateParams{mode: "insert_before", blockID: "blkB", content: "插入"}); err != nil {
		t.Fatalf("insert_before 失败: %v", err)
	}
	if !strings.Contains(putBody, `"block_id":"blkA"`) || !strings.Contains(putBody, `"block_insert_after"`) {
		t.Fatalf("应插入到前一个兄弟块 blkA 之后，实际请求体: %s", putBody)
	}
}

// TestPartialSuccessIsNonZeroAndSurfacesWarnings partial_success 必须非零退出；文本模式透出 warnings 与 log_id。
func TestPartialSuccessIsNonZeroAndSurfacesWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "log-partial")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"revision_id":3},"result":"partial_success","warnings":["degrade_code=5004,msg=Required attribute group on <img> is missing"]}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	// 文本模式
	stdout, _, err := runParams(&contentUpdateParams{mode: "overwrite", content: "x"})
	if err == nil {
		t.Fatal("partial_success 必须非零退出")
	}
	for _, want := range []string{"partial_success", "degrade_code=5004", "log_id=log-partial"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误缺少 %q: %v", want, err)
		}
	}
	if strings.Contains(stdout, "成功") {
		t.Fatalf("partial_success 不应打印成功提示: %q", stdout)
	}

	// JSON 模式：仍输出完整 data
	stdout, _, err = runParams(&contentUpdateParams{mode: "append", content: "x", output: "json"})
	if err == nil || !strings.Contains(stdout, `"partial_success"`) || !strings.Contains(stdout, `"log_id": "log-partial"`) {
		t.Fatalf("JSON 模式应输出 data 并非零退出: err=%v stdout=%s", err, stdout)
	}
	if _, ok := client.AsDocsAIResultError(err); !ok {
		t.Fatalf("错误链中应包含 DocsAIResultError: %v", err)
	}
}

// TestSuccessWarningsPrintedToStderr result=success 但带 warnings 时，文本模式在 stderr 透出。
func TestSuccessWarningsPrintedToStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "log-warn")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"revision_id":3},"result":"success","warnings":["degrade_code=5002,msg=Unsupported attribute"]}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	stdout, stderr, err := runParams(&contentUpdateParams{mode: "append", content: "x"})
	if err != nil {
		t.Fatalf("success 不应报错: %v", err)
	}
	if !strings.Contains(stdout, "追加成功") || !strings.Contains(stderr, "degrade_code=5002") || !strings.Contains(stderr, "log-warn") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestUpdateBusinessErrorParsedBeforeHTTPStatus 业务码随 HTTP 400 下发时先解析信封（得到 code 与 log_id）。
func TestUpdateBusinessErrorParsedBeforeHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":3380002,"msg":"start_block_id and end_block_id are only supported for block_replace","error":{"log_id":"log-400"}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	_, _, err := runParams(&contentUpdateParams{mode: "append", content: "x"})
	if err == nil || !client.HasAPICode(err, 3380002) || !strings.Contains(err.Error(), "log-400") {
		t.Fatalf("应解析出业务码 3380002 与 log_id，得到: %v", err)
	}
}

func TestValidateContentUpdateRequestNewLocators(t *testing.T) {
	ok := []contentUpdateParams{
		{mode: "replace_range", blockID: "a,b", content: "x"},
		{mode: "block_replace", blockID: "a", content: "x"},
		{mode: "delete_range", startBlockID: "0", endBlockID: "-1"},
		{mode: "str_replace", pattern: "旧", content: ""},
		{mode: "block_move_after", blockID: "t", srcBlockIDs: "a,b"},
		{mode: "insert_after", blockID: "-1", content: "x"},
	}
	for _, p := range ok {
		p := p
		if alias, found := contentUpdateModeAliases[p.mode]; found {
			p.mode = alias
		}
		if err := validateContentUpdateRequest(&p); err != nil {
			t.Errorf("%+v 应通过校验，得到 %v", p, err)
		}
	}
	bad := []struct {
		p    contentUpdateParams
		want string
	}{
		{contentUpdateParams{mode: "replace_range", blockID: "a", selByTitle: "## x", content: "x"}, "只能使用其中一种"},
		{contentUpdateParams{mode: "replace_range", startBlockID: "a", content: "x"}, "成对"},
		{contentUpdateParams{mode: "delete_range", startBlockID: "-1", endBlockID: "b"}, "不能为 -1"},
		{contentUpdateParams{mode: "delete_range", startBlockID: "a", endBlockID: "0"}, "不能为 0"},
		{contentUpdateParams{mode: "insert_after", startBlockID: "a", endBlockID: "b", content: "x"}, "只用于 --mode replace_range"},
		{contentUpdateParams{mode: "str_replace", content: "x"}, "需要 --pattern"},
		{contentUpdateParams{mode: "replace_all", pattern: "x", content: "y"}, "--pattern 只用于"},
		{contentUpdateParams{mode: "replace_all", blockID: "a", content: "y"}, "replace_all 只支持"},
		{contentUpdateParams{mode: "block_move_after", blockID: "t"}, "--src-block-ids"},
		{contentUpdateParams{mode: "block_move_after", blockID: "t", srcBlockIDs: "a", content: "x", contentSet: true}, "不接受写入内容"},
		{contentUpdateParams{mode: "insert_before", blockID: "a,b", content: "x"}, "单个锚点块"},
		{contentUpdateParams{mode: "append", blockID: "a", content: "x"}, "不接受定位参数"},
		{contentUpdateParams{mode: "replace_all", content: "x"}, "--mode overwrite"},
	}
	for _, tc := range bad {
		p := tc.p
		err := validateContentUpdateRequest(&p)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v 应报含 %q 的错误，得到 %v", tc.p, tc.want, err)
			continue
		}
		if !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%+v 应为用法错误（exit 2）: %v", tc.p, err)
		}
	}
}
