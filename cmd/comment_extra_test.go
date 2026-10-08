package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestBuildCommentAnchor(t *testing.T) {
	cases := []struct {
		fileType, blockID string
		full              bool
		want              string // JSON；"nil" 表示全文评论
		wantErr           bool
	}{
		{fileType: "docx", want: "nil"},
		{fileType: "docx", blockID: "blk1", want: `{"block_id":"blk1"}`},
		{fileType: "docx", blockID: "blk1", full: true, want: "nil"},
		{fileType: "doc", blockID: "blk1", wantErr: true},
		{fileType: "sheet", blockID: "a281f9!D6", want: `{"block_id":"a281f9","sheet_col":3,"sheet_row":5}`},
		{fileType: "sheet", blockID: "s1!AA10", want: `{"block_id":"s1","sheet_col":26,"sheet_row":9}`},
		{fileType: "sheet", wantErr: true},
		{fileType: "sheet", blockID: "s1!D0", wantErr: true},
		{fileType: "sheet", blockID: "s1!12", wantErr: true},
		{fileType: "slides", blockID: "shape!bPq", want: `{"block_id":"bPq","slide_block_type":"shape"}`},
		{fileType: "slides", blockID: "bPq", wantErr: true},
		{fileType: "bitable", blockID: "tbl!rec!vew", want: `{"base_record_id":"rec","base_view_id":"vew","block_id":"tbl"}`},
		{fileType: "bitable", blockID: "tbl!rec", wantErr: true},
		{fileType: "file", want: `{"block_id":"test"}`},
		{fileType: "file", blockID: "x", wantErr: true},
		{fileType: "mindnote", wantErr: true},
	}
	for _, tc := range cases {
		got, err := buildCommentAnchor(tc.fileType, tc.blockID, tc.full)
		if tc.wantErr {
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Errorf("%+v 期望用法错误，得到 %v / %v", tc, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%+v: %v", tc, err)
			continue
		}
		gotJSON := "nil"
		if got != nil {
			b, _ := json.Marshal(got)
			gotJSON = string(b)
		}
		if gotJSON != tc.want {
			t.Errorf("%+v => %s, want %s", tc, gotJSON, tc.want)
		}
	}
}

func TestParseReplyElementsTotalRuneCap(t *testing.T) {
	long := strings.Repeat("字", 6000)
	if _, err := parseReplyElements(fmt.Sprintf(`[{"type":"text","text":%q}]`, long)); err != nil {
		t.Fatalf("单元素 6000 字应允许（服务端上限是合计 10000）: %v", err)
	}
	if _, err := parseReplyElements(fmt.Sprintf(`[{"type":"text","text":%q},{"type":"text","text":%q}]`, long, long)); err == nil || !strings.Contains(err.Error(), "合计") {
		t.Fatalf("合计超过 10000 字必须拒绝: %v", err)
	}
}

func TestCreateNewCommentSendsAnchor(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"comment_id":"c9"}}`)
	})
	anchor, err := buildCommentAnchor("sheet", "s1!B3", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateNewComment(client.CreateNewCommentReq{FileToken: "sht1", FileType: "sheet", ReplyElements: []map[string]any{{"type": "text", "text": "x"}}, Anchor: anchor}, "u-test"); err != nil {
		t.Fatal(err)
	}
	got := reqs()[0]
	if got.Path != "/open-apis/drive/v1/files/sht1/new_comments" || !strings.Contains(got.Body, `"anchor":{"block_id":"s1","sheet_col":1,"sheet_row":2}`) || !strings.Contains(got.Body, `"file_type":"sheet"`) {
		t.Fatalf("请求 = %+v", got)
	}
}

func TestReplyUpdateAndReact(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		_, _ = fmt.Fprint(w, `{"code":0,"data":{}}`)
	})

	// dry-run 不发请求
	out, _, err := runCmdWithFlags(t, updateReplyCmd, []string{"doc1", "c1", "r1"}, "--text", "改", "--dry-run")
	if err != nil || !strings.Contains(out, `"dry_run": true`) || len(reqs()) != 0 {
		t.Fatalf("dry-run: err=%v out=%s reqs=%v", err, out, reqs())
	}

	_, _, err = runCmdWithFlags(t, updateReplyCmd, []string{"doc1", "c1", "r1"},
		"--content", `[{"type":"text","text":"请 "},{"type":"mention_user","mention_user":"ou_x"},{"type":"link","link":"https://example.feishu.cn/docx/D"}]`)
	if err != nil {
		t.Fatal(err)
	}
	got := reqs()[0]
	if got.Method != http.MethodPut || got.Path != "/open-apis/drive/v1/files/doc1/comments/c1/replies/r1" || got.Query != "file_type=docx" {
		t.Fatalf("update 请求 = %+v", got)
	}
	for _, want := range []string{`"type":"text_run"`, `"person":{"user_id":"ou_x"}`, `"docs_link":{"url":"https://example.feishu.cn/docx/D"}`} {
		if !strings.Contains(got.Body, want) {
			t.Fatalf("update body 缺少 %s: %s", want, got.Body)
		}
	}
	if got.Auth != "Bearer t-test-token" {
		t.Fatalf("写操作默认应为 Bot 身份: %q", got.Auth)
	}

	if _, _, err := runCmdWithFlags(t, updateReplyCmd, []string{"doc1", "c1", "r1"}, "--text", "a", "--content", `[{"type":"text","text":"b"}]`); !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("--text 与 --content 同时给应为用法错误: %v", err)
	}

	if _, _, err := runCmdWithFlags(t, reactReplyCmd, []string{"doc1", "r1"}, "--emoji", "THUMBSUP"); err != nil {
		t.Fatal(err)
	}
	got = reqs()[len(reqs())-1]
	if got.Method != http.MethodPost || got.Path != "/open-apis/drive/v2/files/doc1/comments/reaction" || got.Query != "file_type=docx" ||
		!strings.Contains(got.Body, `"action":"add"`) || !strings.Contains(got.Body, `"reaction_type":"THUMBSUP"`) || !strings.Contains(got.Body, `"reply_id":"r1"`) {
		t.Fatalf("react 请求 = %+v", got)
	}
	n := len(reqs())
	if _, _, err := runCmdWithFlags(t, reactReplyCmd, []string{"doc1", "r1"}, "--emoji", "thumbsup"); !clierr.HasKind(err, clierr.KindUsage) || len(reqs()) != n {
		t.Fatalf("未知表情应本地拒绝且不发请求: %v", err)
	}
}

func TestCommentGetAndBatchGetUseBatchQuery(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"items":[%s]}}`, commentItemA)
	})
	out, _, err := runCmdWithFlags(t, getCommentCmd, []string{"doc1", "c1"}, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	got := reqs()[0]
	if got.Method != http.MethodPost || got.Path != "/open-apis/drive/v1/files/doc1/comments/batch_query" || !strings.Contains(got.Body, `"comment_ids":["c1"]`) {
		t.Fatalf("comment get 应走 batch_query: %+v", got)
	}
	if !strings.Contains(out, `"content": "请看 https://example.feishu.cn/docx/X @ou_c"`) {
		t.Fatalf("get 输出缺少正文: %s", out)
	}

	if _, _, err := runCmdWithFlags(t, batchGetCommentsCmd, []string{"doc1"}, "--comment-ids", "c1,c2", "-o", "json"); err != nil {
		t.Fatal(err)
	}
	if body := reqs()[1].Body; !strings.Contains(body, `"comment_ids":["c1","c2"]`) {
		t.Fatalf("batch-get body = %s", body)
	}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("c%d", i)
	}
	if _, err := normalizeCommentIDs(ids); !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("超过 100 个应为用法错误: %v", err)
	}
}
