package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

func TestParseReadStatusMessageIDs(t *testing.T) {
	ids, err := parseReadStatusMessageIDs([]string{"om_a", "om_b,om_a"}, "om_c")
	if err != nil || strings.Join(ids, ",") != "om_a,om_b,om_c" {
		t.Fatalf("应合并去重保序，got %v %v", ids, err)
	}
	fifty := make([]string, 50)
	for i := range fifty {
		fifty[i] = fmt.Sprintf("om_%d", i)
	}
	if _, err := parseReadStatusMessageIDs(fifty, ""); err != nil {
		t.Fatalf("50 个应通过: %v", err)
	}
	for name, args := range map[string][]string{
		"51 个":    append(append([]string{}, fifty...), "om_x"),
		"空":       nil,
		"omt_ 前缀": {"omt_x"},
	} {
		if _, err := parseReadStatusMessageIDs(args, ""); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s: 应为用法错误，got %v", name, err)
		}
	}
}

// 51 个 ID 在读凭据/联网前即 exit 2。
func TestMsgReadStatusTooManyIDsBeforeNetwork(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("参数错误时不应联网: %s", r.URL.Path)
	})
	defer cleanup()
	args := make([]string, 51)
	for i := range args {
		args[i] = fmt.Sprintf("om_%d", i)
	}
	c := &cobra.Command{}
	c.Flags().String("message-ids", "", "")
	c.Flags().StringP("output", "o", "", "")
	c.Flags().String("user-access-token", "", "")
	if err := msgReadStatusCmd.RunE(c, args); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("51 个 ID 应为用法错误，got %v", err)
	}
}

func TestMsgReadStatusRequest(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var gotBody, gotAuth string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/messages/read_status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[{"message_id":"om_a","is_read":true}],"invalid_message_ids":["om_b"]}}`)
	})
	defer cleanup()
	c := &cobra.Command{}
	c.Flags().String("message-ids", "", "")
	c.Flags().StringP("output", "o", "", "")
	c.Flags().String("user-access-token", "", "")
	_ = c.Flags().Set("user-access-token", testUserToken)
	_ = c.Flags().Set("output", "json")
	var err error
	out := captureStdout(t, func() { err = msgReadStatusCmd.RunE(c, []string{"om_a", "om_b"}) })
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"message_ids":["om_a","om_b"]}` || gotAuth != "Bearer "+testUserToken {
		t.Fatalf("请求不符: body=%s auth=%s", gotBody, gotAuth)
	}
	if !strings.Contains(out, `"invalid_message_ids"`) || !strings.Contains(out, `"is_read": true`) {
		t.Fatalf("应透传 data: %s", out)
	}
}

func newForwardTestCmd(dryRun bool) *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("receive-id", "", "")
	c.Flags().String("receive-id-type", "", "")
	c.Flags().StringP("output", "o", "", "")
	c.Flags().String("user-access-token", "", "")
	c.Flags().Bool("dry-run", false, "")
	_ = c.Flags().Set("receive-id", "oc_dst")
	_ = c.Flags().Set("receive-id-type", "chat_id")
	if dryRun {
		_ = c.Flags().Set("dry-run", "true")
	}
	return c
}

func TestMsgForwardDryRunNoCredentials(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	for id, wantURL := range map[string]string{
		"om_src":  "/open-apis/im/v1/messages/om_src/forward",
		"omt_src": "/open-apis/im/v1/threads/omt_src/forward",
	} {
		var err error
		out := captureStdout(t, func() { err = forwardMessageCmd.RunE(newForwardTestCmd(true), []string{id}) })
		if err != nil {
			t.Fatalf("%s dry-run 应成功: %v", id, err)
		}
		if !strings.Contains(out, wantURL) || !strings.Contains(out, `"receive_id": "oc_dst"`) || !strings.Contains(out, `"identity": "bot"`) {
			t.Fatalf("%s dry-run 输出不符:\n%s", id, out)
		}
	}
}

// omt_ 走 threads/{id}/forward；om_ 仍走 messages/{id}/forward（与旧版一致）。
func TestMsgForwardRoutesThreadID(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var paths []string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"message_id":"om_new"}}`)
	}))
	defer cleanup()
	for _, id := range []string{"omt_src", "om_src"} {
		var err error
		_ = captureStdout(t, func() { err = forwardMessageCmd.RunE(newForwardTestCmd(false), []string{id}) })
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if len(paths) != 2 || !strings.HasPrefix(paths[0], "/open-apis/im/v1/threads/omt_src/forward?") ||
		!strings.HasPrefix(paths[1], "/open-apis/im/v1/messages/om_src/forward?") || !strings.Contains(paths[0], "receive_id_type=chat_id") {
		t.Fatalf("路由不符: %v", paths)
	}
}

func TestMsgMergeForwardDryRunNoCredentials(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	c := &cobra.Command{}
	c.Flags().String("receive-id", "", "")
	c.Flags().String("receive-id-type", "email", "")
	c.Flags().String("message-ids", "", "")
	c.Flags().String("user-access-token", "", "")
	c.Flags().Bool("dry-run", false, "")
	_ = c.Flags().Set("receive-id", "oc_dst")
	_ = c.Flags().Set("receive-id-type", "chat_id")
	_ = c.Flags().Set("message-ids", "om_a,om_b")
	_ = c.Flags().Set("dry-run", "true")
	var err error
	out := captureStdout(t, func() { err = mergeForwardMsgCmd.RunE(c, nil) })
	if err != nil {
		t.Fatalf("dry-run 应成功: %v", err)
	}
	if !strings.Contains(out, "/open-apis/im/v1/messages/merge_forward") || !strings.Contains(out, `"message_id_list"`) {
		t.Fatalf("dry-run 输出不符:\n%s", out)
	}
}

func TestConciseRendering(t *testing.T) {
	if err := validateConciseOutput(true, "json"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("--concise 与 -o json 应互斥，got %v", err)
	}
	if err := validateConciseOutput(false, "json"); err != nil {
		t.Fatal(err)
	}
	msgs := []*larkim.Message{
		{
			MessageId:  strPtr("om_1"),
			MsgType:    strPtr("text"),
			CreateTime: strPtr("1700000000000"),
			ThreadId:   strPtr("omt_1"),
			Sender:     &larkim.Sender{Id: strPtr("ou_a")},
			Body:       &larkim.MessageBody{Content: strPtr(`{"text":"hi @_user_1\nline2"}`)},
			Mentions:   []*larkim.Mention{{Key: strPtr("@_user_1"), Name: strPtr("Bob")}},
		},
		{
			MessageId: strPtr("om_2"),
			MsgType:   strPtr("post"),
			Sender:    &larkim.Sender{Id: strPtr("ou_b")},
			Body:      &larkim.MessageBody{Content: strPtr(`{"title":"T","content":[[{"tag":"text","text":"see "},{"tag":"a","text":"doc","href":"https://example.com"}]]}`)},
		},
	}
	result := &client.ListMessagesResult{
		Items:     msgs,
		HasMore:   true,
		PageToken: "pt_next",
		ThreadReplies: map[string][]*larkim.Message{"omt_1": {{
			MessageId: strPtr("om_r"),
			ParentId:  strPtr("om_1"),
			MsgType:   strPtr("image"),
			Sender:    &larkim.Sender{Id: strPtr("ou_b")},
			Body:      &larkim.MessageBody{Content: strPtr(`{"image_key":"img_k"}`)},
		}}},
		ThreadHasMore: map[string]bool{"omt_1": true},
	}
	var buf bytes.Buffer
	renderHistoryConcise(&buf, "消息记录 oc_x", result, map[string]string{"ou_a": "Alice"})
	got := buf.String()
	for _, want := range []string{
		"# 消息记录 oc_x（2 条）",
		"Alice (ou_a) · om_1",
		"  hi @Bob",
		"  line2",
		"T", "see [doc](https://example.com)",
		"> 话题 omt_1：1 条回复，还有更多",
		"    - ou_b · om_r · 回复 om_1",
		"[图片 img_k]",
		"--page-token pt_next",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("concise 输出缺少 %q:\n%s", want, got)
		}
	}
}
