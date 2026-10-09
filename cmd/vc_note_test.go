package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

const testNoteID = "7690848884788907213"

func newVCNoteTranscriptTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "markdown", "")
	cmd.Flags().String("locale", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

// noteDetailJSON 构造纪要详情响应
func noteDetailJSON(displayType int) string {
	return fmt.Sprintf(`{"code":0,"msg":"ok","data":{"note":{"note_display_type":%d,"create_time":"1790665510",`+
		`"artifacts":[{"artifact_type":1,"doc_token":"doc_main"},{"artifact_type":2,"doc_token":"doc_verbatim"}],`+
		`"references":[{"doc_token":"doc_ref"}]}}}`, displayType)
}

// TestVCNoteTranscriptProtocol 锁住统一逐字稿协议（对齐官方 note_transcript.go）：
// 参数名 format / page_size=200 / locale、按 data.transcript.<format> 读取、数字游标翻页。
// 旧实现发 transcript_format=text、把 data.transcript 当字符串、游标只认字符串——对真实服务端不可用。
func TestVCNoteTranscriptProtocol(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	vcNoteTranscriptPageGap = 0
	t.Cleanup(func() { vcNoteTranscriptPageGap = noteTranscriptPageGap })

	var pages int32
	var queries []url.Values
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/vc/v1/notes/" + testNoteID:
			_, _ = fmt.Fprint(w, noteDetailJSON(2))
		case "/open-apis/vc/v1/notes/" + testNoteID + "/unified_note_transcript":
			queries = append(queries, r.URL.Query())
			switch atomic.AddInt32(&pages, 1) {
			case 1:
				// 数字游标（超过 2^53）必须原样传回
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"transcript":{"plain_text":"第一页\n"},"has_more":true,"next_cursor_id":7123456789012345678}}`)
			case 2:
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"transcript":{"plain_text":"第二页\n"},"has_more":false}}`)
			default:
				http.Error(w, "too many pages", http.StatusInternalServerError)
			}
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()

	out := filepath.Join(t.TempDir(), "t.txt")
	cmd := newVCNoteTranscriptTestCmd()
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	mustSetFlag(t, cmd, "format", "plain_text")
	mustSetFlag(t, cmd, "locale", "en_us")
	mustSetFlag(t, cmd, "output", out)
	if _, err := captureVCBotStdout(t, func() error { return vcNoteTranscriptCmd.RunE(cmd, []string{testNoteID}) }); err != nil {
		t.Fatalf("transcript 返回错误: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("读取输出失败: %v", err)
	}
	if string(got) != "第一页\n第二页\n" {
		t.Fatalf("逐字稿内容 = %q", got)
	}
	if len(queries) != 2 {
		t.Fatalf("应翻 2 页，实际 %d", len(queries))
	}
	for i, q := range queries {
		if q.Get("format") != "plain_text" || q.Get("page_size") != "200" || q.Get("locale") != "en_us" {
			t.Fatalf("第 %d 页参数错误: %v", i+1, q)
		}
		if _, has := q["transcript_format"]; has {
			t.Fatalf("不应发送旧参数 transcript_format: %v", q)
		}
	}
	if c := queries[1]["cursor_id"]; len(c) != 1 || c[0] != "7123456789012345678" {
		t.Fatalf("第 2 页 cursor_id = %v，应为精确的数字游标", c)
	}
}

// TestVCNoteTranscriptNormalNoteHint 普通纪要（note_display_type=1）不调统一逐字稿接口，直接提示 verbatim 文档。
func TestVCNoteTranscriptNormalNoteHint(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var transcriptHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/unified_note_transcript") {
			atomic.AddInt32(&transcriptHits, 1)
		}
		_, _ = fmt.Fprint(w, noteDetailJSON(1))
	})
	defer cleanup()

	cmd := newVCNoteTranscriptTestCmd()
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	err := vcNoteTranscriptCmd.RunE(cmd, []string{testNoteID})
	if err == nil || !strings.Contains(err.Error(), "doc export doc_verbatim") || !strings.Contains(err.Error(), "normal") {
		t.Fatalf("普通纪要应提示改读 verbatim 文档，实际: %v", err)
	}
	if !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("应为用法错误（exit 2），实际 kinds=%v", clierr.Kinds(err))
	}
	if hits := atomic.LoadInt32(&transcriptHits); hits != 0 {
		t.Fatalf("普通纪要不应再调统一逐字稿接口，实际 %d 次", hits)
	}
}

// TestVCNoteTranscriptNotSupportCodeHint 纪要类型未返回时照常调接口，服务端 121002 not support（随 HTTP 400 下发）
// 同样映射为改读 verbatim 文档的提示。
func TestVCNoteTranscriptNotSupportCodeHint(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/unified_note_transcript") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":121002,"msg":"not support"}`)
			return
		}
		// 不返回 note_display_type
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"note":{"artifacts":[{"artifact_type":2,"doc_token":"doc_verbatim"}]}}}`)
	})
	defer cleanup()

	cmd := newVCNoteTranscriptTestCmd()
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	err := vcNoteTranscriptCmd.RunE(cmd, []string{testNoteID})
	if err == nil || !strings.Contains(err.Error(), "doc export doc_verbatim") {
		t.Fatalf("121002 应提示改读 verbatim 文档，实际: %v", err)
	}
}

// TestVCNoteTranscriptCursorNotAdvancing has_more=true 但游标不前进时报错，不输出半截逐字稿。
func TestVCNoteTranscriptCursorNotAdvancing(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	vcNoteTranscriptPageGap = 0
	t.Cleanup(func() { vcNoteTranscriptPageGap = noteTranscriptPageGap })
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/unified_note_transcript") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"transcript":{"markdown":"x"},"has_more":true,"next_cursor_id":"c1"}}`)
			return
		}
		_, _ = fmt.Fprint(w, noteDetailJSON(2))
	})
	defer cleanup()

	cmd := newVCNoteTranscriptTestCmd()
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	out, err := captureVCBotStdout(t, func() error { return vcNoteTranscriptCmd.RunE(cmd, []string{testNoteID}) })
	if err == nil || !strings.Contains(err.Error(), "游标未前进") {
		t.Fatalf("重复游标应报错，实际: %v", err)
	}
	if out != "" {
		t.Fatalf("失败时不应输出半截逐字稿，stdout=%q", out)
	}
}

func TestParseUnifiedTranscriptPage(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		format  string
		want    string
		wantErr string
	}{
		{"官方对象结构", `{"transcript":{"markdown":"# a"}}`, "markdown", "# a", ""},
		{"空对象视为空页", `{"transcript":{}}`, "markdown", "", ""},
		{"兼容字符串", `{"transcript":"plain"}`, "plain_text", "plain", ""},
		{"缺少 format 字段", `{"transcript":{"markdown":"a"}}`, "plain_text", "", "没有 plain_text 字段"},
		{"缺少 transcript 字段显式报错", `{"items":[],"has_more":false}`, "markdown", "", "无法从响应中识别逐字稿字段（实际字段: has_more, items）"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tc.data), &data); err != nil {
				t.Fatal(err)
			}
			got, err := parseUnifiedTranscriptPage(data, tc.format)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want 包含 %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestParseLooseCursor(t *testing.T) {
	cases := []struct {
		in     any
		want   string
		wantOK bool
	}{
		{"abc", "abc", true},
		{"", "", false},
		{"0", "", false},
		{json.Number("7123456789012345678"), "7123456789012345678", true},
		{json.Number("0"), "", false},
		{float64(42), "42", true},
		{float64(1.5), "", false},
		{nil, "", false},
	}
	for _, tc := range cases {
		got, ok := parseLooseCursor(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("parseLooseCursor(%#v) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestNormalizeNoteTranscriptFormat(t *testing.T) {
	cases := map[string]string{"": "markdown", "markdown": "markdown", "plain_text": "plain_text", "text": "plain_text"}
	for in, want := range cases {
		got, err := normalizeNoteTranscriptFormat(in)
		if err != nil || got != want {
			t.Errorf("normalize(%q) = %q,%v; want %q", in, got, err, want)
		}
	}
	if _, err := normalizeNoteTranscriptFormat("html"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Errorf("非法 format 应为用法错误，实际 %v", err)
	}
}
