package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/client"
)

type historyStubCalls struct {
	members  int
	search   int
	listToks []string
	searchQ  string
}

func historyStub(t *testing.T, calls *historyStubCalls, listBody string) func() {
	t.Helper()
	return stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/members/list"):
			calls.members++
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"member_id":"ou_m","name":"成员"}],"bots":[],"has_more":false}}`)
		case r.URL.Path == "/open-apis/im/v1/messages/search":
			calls.search++
			var body strings.Builder
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			body.Write(buf[:n])
			calls.searchQ = body.String()
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[],"has_more":false}}`)
		case r.URL.Path == "/open-apis/im/v1/messages":
			calls.listToks = append(calls.listToks, r.URL.Query().Get("page_token"))
			_, _ = fmt.Fprint(w, listBody)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
}

// TestMsgHistoryLoadsChatMembersOnlyForJSON 文本模式不再拉群成员（旧版每次都翻成员接口）。
func TestMsgHistoryLoadsChatMembersOnlyForJSON(t *testing.T) {
	for _, output := range []string{"", "json"} {
		t.Run("output="+output, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			calls := &historyStubCalls{}
			cleanup := historyStub(t, calls, `{"code":0,"data":{"items":[{"message_id":"om_1","msg_type":"text","body":{"content":"{}"}}],"has_more":false}}`)
			defer cleanup()

			cmd := newGetMessageHistoryTestCmd()
			mustSetFlag(t, cmd, "container-id", testChatID)
			if output != "" {
				mustSetFlag(t, cmd, "output", output)
			}
			captureStdout(t, func() {
				if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
					t.Fatalf("history 返回错误: %v", err)
				}
			})
			want := 0
			if output == "json" {
				want = 1
			}
			if calls.members != want {
				t.Fatalf("成员接口调用 %d 次，want %d", calls.members, want)
			}
		})
	}
}

// TestMsgHistoryNoSearchFallbackWhenPaging 续翻（带 page_token）遇到空页 + has_more 时不再切搜索接口
// （list 的 page_token 喂给搜索接口会报 1020），而是提示继续翻页。
func TestMsgHistoryNoSearchFallbackWhenPaging(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	calls := &historyStubCalls{}
	cleanup := historyStub(t, calls, `{"code":0,"data":{"items":[],"has_more":true,"page_token":"p3"}}`)
	defer cleanup()

	cmd := newGetMessageHistoryTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	mustSetFlag(t, cmd, "page-token", "p2")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	captureStdout(t, func() {
		if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("history 返回错误: %v", err)
		}
	})
	if calls.search != 0 {
		t.Fatalf("续翻空页不应切搜索接口，search 调用 %d 次", calls.search)
	}
	if !strings.Contains(stderr.String(), "--page-token p3") {
		t.Fatalf("应提示继续翻页，stderr=%q", stderr.String())
	}
}

// TestMsgHistoryFirstPageFallbackMessageAndTimeRange 首页为空走搜索降级：
// 提示语与 User 身份一致（不再说"bot 不在群"），并把时间范围透传给搜索。
func TestMsgHistoryFirstPageFallbackMessageAndTimeRange(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	calls := &historyStubCalls{}
	cleanup := historyStub(t, calls, `{"code":0,"data":{"items":[],"has_more":true,"page_token":"p2"}}`)
	defer cleanup()

	cmd := newGetMessageHistoryTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	mustSetFlag(t, cmd, "start-time", "1700000000")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	captureStdout(t, func() {
		if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("history 返回错误: %v", err)
		}
	})
	if calls.search != 1 {
		t.Fatalf("首页为空应降级到搜索，search 调用 %d 次", calls.search)
	}
	msg := stderr.String()
	if strings.Contains(msg, "bot 不在") || !strings.Contains(msg, "当前用户身份") || !strings.Contains(msg, "--sort-type 不生效") {
		t.Fatalf("降级提示不符: %q", msg)
	}
	if !strings.Contains(calls.searchQ, "time_range") {
		t.Fatalf("时间范围应透传给搜索: %s", calls.searchQ)
	}
}

func TestResolveOpenIDByEmail(t *testing.T) {
	old := resolveContactIDs
	defer func() { resolveContactIDs = old }()

	resolveContactIDs = func(emails, mobiles []string) ([]*client.UserContactIDInfo, error) {
		return []*client.UserContactIDInfo{{Email: "Alice@Example.com", OpenID: "ou_alice"}}, nil
	}
	got, err := resolveOpenIDByEmail(&bytes.Buffer{}, "alice@example.com", "")
	if err != nil || got != "ou_alice" {
		t.Fatalf("精确解析 = %q, %v", got, err)
	}

	resolveContactIDs = func(emails, mobiles []string) ([]*client.UserContactIDInfo, error) {
		return []*client.UserContactIDInfo{{Email: "alice@example.com"}}, nil
	}
	if _, err := resolveOpenIDByEmail(&bytes.Buffer{}, "alice@example.com", ""); err == nil {
		t.Fatal("batch_get_id 未返回 open_id 时应报未找到，而不是去模糊搜索")
	}

	// batch_get_id 失败 → 退回搜索，命中多条时报错
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"open_id":"ou_1","name":"A"},{"open_id":"ou_2","name":"B"}],"has_more":false}}`)
	})
	defer cleanup()
	resolveContactIDs = func(emails, mobiles []string) ([]*client.UserContactIDInfo, error) {
		return nil, errors.New("code=99991672, msg=no permission")
	}
	if _, err := resolveOpenIDByEmail(&bytes.Buffer{}, "alice@example.com", testUserToken); err == nil || !strings.Contains(err.Error(), "--user-id") {
		t.Fatalf("搜索命中多条应报错并建议 --user-id，got %v", err)
	}
}
