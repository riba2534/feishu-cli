package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

const commentItemA = `{
  "comment_id": "c1", "user_id": "ou_a", "create_time": 100, "is_solved": false, "is_whole": false, "quote": "原文",
  "has_more": true, "page_token": "rp",
  "reply_list": {"replies": [
    {"reply_id": "r2", "user_id": "ou_b", "create_time": 200, "content": {"elements": [{"type": "text_run", "text_run": {"text": "收到"}}]}},
    {"reply_id": "r1", "user_id": "ou_a", "create_time": 100, "content": {"elements": [
      {"type": "text_run", "text_run": {"text": "请看 "}},
      {"type": "docs_link", "docs_link": {"url": "https://example.feishu.cn/docx/X"}},
      {"type": "text_run", "text_run": {"text": " "}},
      {"type": "person", "person": {"user_id": "ou_c"}}
    ]}}
  ]}
}`

func TestCommentListPaginationAndContent(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path != "/open-apis/drive/v1/files/doc1/comments" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("page_token") == "p2" {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"comment_id":"c2","is_whole":true,"reply_list":{"replies":[]}}],"has_more":false}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"items":[%s],"has_more":true,"page_token":"p2"}}`, commentItemA)
	})

	// 单页：保留 has_more 并在 stderr 提示 page_token
	out, errOut, err := runCmdWithFlags(t, listCommentsCmd, []string{"doc1"}, "--type", "docx", "--page-size", "1", "-o", "json", "--solved-status", "false")
	if err != nil {
		t.Fatalf("comment list: %v", err)
	}
	var page1 []map[string]any
	if err := json.Unmarshal([]byte(out), &page1); err != nil {
		t.Fatalf("输出不是 JSON 数组: %v\n%s", err, out)
	}
	if len(page1) != 1 || !strings.Contains(errOut, "page_token=p2") {
		t.Fatalf("单页应只返回 1 条并提示续翻: n=%d stderr=%q", len(page1), errOut)
	}
	// 正文：根回复按 create_time 排序后渲染，保留 @人与链接
	if got := page1[0]["content"]; got != "请看 https://example.feishu.cn/docx/X @ou_c" {
		t.Fatalf("content = %q", got)
	}
	// reply_list 原样保留服务端结构（person / docs_link 元素都在）
	replyList, _ := json.Marshal(page1[0]["reply_list"])
	if !strings.Contains(string(replyList), `"person":{"user_id":"ou_c"}`) || !strings.Contains(string(replyList), `"docs_link":{"url":"https://example.feishu.cn/docx/X"}`) {
		t.Fatalf("reply_list 未保留服务端结构: %s", replyList)
	}
	if q := reqs()[0].Query; !strings.Contains(q, "is_solved=false") || !strings.Contains(q, "page_size=1") || !strings.Contains(q, "file_type=docx") {
		t.Fatalf("查询参数 = %q", q)
	}

	// --page-all：拉完两页，不再提示
	out, errOut, err = runCmdWithFlags(t, listCommentsCmd, []string{"doc1"}, "--type", "docx", "--page-size", "1", "-o", "json", "--page-all")
	if err != nil {
		t.Fatalf("comment list --page-all: %v", err)
	}
	var all []map[string]any
	if err := json.Unmarshal([]byte(out), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || strings.Contains(errOut, "has_more=true") {
		t.Fatalf("--page-all 应拉全两页: n=%d stderr=%q", len(all), errOut)
	}

	// 文本模式：正文与回复都渲染出来
	out, _, err = runCmdWithFlags(t, listCommentsCmd, []string{"doc1"}, "--type", "docx", "--page-size", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"正文:     请看 https://example.feishu.cn/docx/X @ou_c", "[r2] ou_b: 收到", "comment reply list"} {
		if !strings.Contains(out, want) {
			t.Fatalf("文本输出缺少 %q:\n%s", want, out)
		}
	}
}

func TestCommentListFilterValidation(t *testing.T) {
	for _, tc := range []struct {
		solved, scope string
		wantSolved    *bool
		wantWhole     *bool
		wantErr       bool
	}{
		{solved: "all", scope: "all"},
		{solved: "true", scope: "whole", wantSolved: boolPtrForTest(true), wantWhole: boolPtrForTest(true)},
		{solved: "false", scope: "partial", wantSolved: boolPtrForTest(false), wantWhole: boolPtrForTest(false)},
		{solved: "yes", wantErr: true},
		{scope: "half", wantErr: true},
	} {
		s, err1 := parseCommentSolvedStatus(tc.solved)
		w, err2 := parseCommentScope(tc.scope)
		if tc.wantErr {
			if err1 == nil && err2 == nil {
				t.Errorf("%+v 期望用法错误", tc)
			}
			for _, e := range []error{err1, err2} {
				if e != nil && !clierr.HasKind(e, clierr.KindUsage) {
					t.Errorf("应为用法错误(exit 2): %v", e)
				}
			}
			continue
		}
		if err1 != nil || err2 != nil || !equalBoolPtr(s, tc.wantSolved) || !equalBoolPtr(w, tc.wantWhole) {
			t.Errorf("%+v => %v %v %v %v", tc, s, w, err1, err2)
		}
	}
}

func boolPtrForTest(b bool) *bool { return &b }

func equalBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestCommentReplyListElementsAndPagination(t *testing.T) {
	newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path != "/open-apis/drive/v1/files/doc1/comments/c1/replies" {
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("page_token") == "n2" {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"reply_id":"r2","content":{"elements":[{"type":"text_run","text_run":{"text":"二"}}]}}],"has_more":false}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"reply_id":"r1","user_id":"ou_a","content":{"elements":[
			{"type":"text_run","text_run":{"text":"cc "}},{"type":"person","person":{"user_id":"ou_x"}},
			{"type":"text_run","text_run":{"text":" 见 "}},{"type":"docs_link","docs_link":{"url":"https://example.feishu.cn/wiki/W"}}]}}],
			"has_more":true,"page_token":"n2"}}`)
	})

	out, errOut, err := runCmdWithFlags(t, listReplyCmd, []string{"doc1", "c1"}, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	if err := json.Unmarshal([]byte(out), &replies); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(replies) != 1 || replies[0]["content"] != "cc @ou_x 见 https://example.feishu.cn/wiki/W" {
		t.Fatalf("content 应包含 @人与链接: %+v", replies)
	}
	elems, _ := json.Marshal(replies[0]["elements"])
	if !strings.Contains(string(elems), `"person":{"user_id":"ou_x"}`) {
		t.Fatalf("elements 未保留服务端结构: %s", elems)
	}
	if !strings.Contains(errOut, "page_token=n2") {
		t.Fatalf("应提示续翻: %q", errOut)
	}

	out, _, err = runCmdWithFlags(t, listReplyCmd, []string{"doc1", "c1"}, "-o", "json", "--page-all")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &replies); err != nil || len(replies) != 2 {
		t.Fatalf("--page-all 应返回 2 条: %v %s", err, out)
	}
}

// P0-6：业务码随 HTTP 400 下发时必须解析为 *APIError（code=N, msg=...），而不是 "HTTP 400, body: ..."。
func TestCommentWritesParseBusinessCodeOnHTTP400(t *testing.T) {
	newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":1069302,"msg":"param error"}`)
	})

	_, err := client.CreateCommentReply("doc1", "c1", "docx", "hi", "")
	assertAPIErrorCode(t, "CreateCommentReply", err, 1069302)

	_, err = client.CreateNewComment(client.CreateNewCommentReq{FileToken: "doc1", FileType: "docx", ReplyElements: []map[string]any{{"type": "text", "text": "x"}}}, "u-test")
	assertAPIErrorCode(t, "CreateNewComment", err, 1069302)

	_, _, _, err = client.ListComments("doc1", "docx", 10, "", "")
	assertAPIErrorCode(t, "ListComments", err, 1069302)

	err = client.UpdateCommentReply("doc1", "c1", "r1", "docx", []map[string]any{{"type": "text_run"}}, "")
	assertAPIErrorCode(t, "UpdateCommentReply", err, 1069302)
}

func assertAPIErrorCode(t *testing.T, name string, err error, code int) {
	t.Helper()
	apiErr, ok := client.AsAPIError(err)
	if !ok || apiErr.Code != code {
		t.Fatalf("%s: 期望 *APIError code=%d，得到 %v", name, code, err)
	}
	if strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("%s: 错误不应停留在 HTTP 状态层: %v", name, err)
	}
}
