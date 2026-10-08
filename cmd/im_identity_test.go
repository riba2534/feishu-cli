package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newIMIdentityTestCmd(extra ...string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("user-access-token", "", "")
	addAsFlag(cmd)
	for _, name := range extra {
		cmd.Flags().String(name, "", "")
	}
	return cmd
}

// TestIMResourceCommandsFallBackToBotWhenNotLoggedIn 验证 reaction/pin/chat get 等
// 接口两种身份都支持的命令：默认 auto 在未登录时回退 Bot（旧版直接报"需要 User Token"），
// 显式 User Token 时仍用 User，--as user 未登录时报错。
func TestIMResourceCommandsFallBackToBotWhenNotLoggedIn(t *testing.T) {
	type call struct {
		name   string
		run    func(cmd *cobra.Command) error
		extra  []string
		setup  func(t *testing.T, cmd *cobra.Command)
		path   string
		method string
		body   string
	}
	calls := []call{
		{
			name:  "msg reaction add",
			run:   func(cmd *cobra.Command) error { return msgReactionAddCmd.RunE(cmd, []string{testMessageID}) },
			extra: []string{"emoji-type"},
			setup: func(t *testing.T, cmd *cobra.Command) { mustSetFlag(t, cmd, "emoji-type", "THUMBSUP") },
			path:  "/open-apis/im/v1/messages/" + testMessageID + "/reactions", method: http.MethodPost,
			body: `{"code":0,"msg":"success","data":{"reaction_id":"r_1"}}`,
		},
		{
			name:  "msg reaction list",
			run:   func(cmd *cobra.Command) error { return msgReactionListCmd.RunE(cmd, []string{testMessageID}) },
			extra: []string{"emoji-type", "page-token"},
			path:  "/open-apis/im/v1/messages/" + testMessageID + "/reactions", method: http.MethodGet,
			body: `{"code":0,"msg":"success","data":{"items":[],"has_more":false}}`,
		},
		{
			name: "msg pin",
			run:  func(cmd *cobra.Command) error { return msgPinCmd.RunE(cmd, []string{testMessageID}) },
			path: "/open-apis/im/v1/pins", method: http.MethodPost,
			body: `{"code":0,"msg":"success","data":{"pin":{"message_id":"om_test_message"}}}`,
		},
		{
			name: "chat get",
			run:  func(cmd *cobra.Command) error { return chatGetCmd.RunE(cmd, []string{testChatID}) },
			path: "/open-apis/im/v1/chats/" + testChatID, method: http.MethodGet,
			body: `{"code":0,"msg":"success","data":{"name":"测试群"}}`,
		},
	}

	for _, c := range calls {
		for _, mode := range []string{"auto-not-logged-in", "explicit-user-token", "as-bot-with-user-token"} {
			t.Run(c.name+"/"+mode, func(t *testing.T) {
				isolateMsgTokenTestEnv(t)
				var gotAuth string
				cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != c.path || r.Method != c.method {
						http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
						return
					}
					gotAuth = r.Header.Get("Authorization")
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, c.body)
				}))
				defer cleanup()

				cmd := newIMIdentityTestCmd(c.extra...)
				if f := cmd.Flags().Lookup("page-size"); f == nil {
					cmd.Flags().Int("page-size", 0, "")
				}
				if c.setup != nil {
					c.setup(t, cmd)
				}
				wantAuth := testTenantAuth
				switch mode {
				case "explicit-user-token":
					mustSetFlag(t, cmd, "user-access-token", testUserToken)
					wantAuth = "Bearer " + testUserToken
				case "as-bot-with-user-token":
					mustSetFlag(t, cmd, "user-access-token", testUserToken)
					mustSetFlag(t, cmd, "as", "bot")
				}
				out := captureStdout(t, func() {
					if err := c.run(cmd); err != nil {
						t.Fatalf("%s 返回错误: %v", c.name, err)
					}
				})
				_ = out
				if gotAuth != wantAuth {
					t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
				}
			})
		}
	}
}

func TestIMResourceCommandsAsUserRequiresLogin(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("--as user 未登录时不应发出业务请求: %s", r.URL.Path)
	}))
	defer cleanup()

	cmd := newIMIdentityTestCmd()
	mustSetFlag(t, cmd, "as", "user")
	err := chatGetCmd.RunE(cmd, []string{testChatID})
	if err == nil || !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("期望 --as user 缺 User Token 报错，got %v", err)
	}
}

// TestMergeForwardAlwaysUsesBot 合并转发接口仅支持 tenant：传了 User Token 也固定 Bot，并在 stderr 提示。
func TestMergeForwardAlwaysUsesBot(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var gotAuth string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/open-apis/im/v1/messages/merge_forward") {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"message":{"message_id":"om_merged"}}}`)
	}))
	defer cleanup()

	cmd := &cobra.Command{}
	cmd.Flags().String("receive-id", "", "")
	cmd.Flags().String("receive-id-type", "chat_id", "")
	cmd.Flags().String("message-ids", "", "")
	cmd.Flags().String("user-access-token", "", "")
	mustSetFlag(t, cmd, "receive-id", testChatID)
	mustSetFlag(t, cmd, "message-ids", "om_a,om_b")
	mustSetFlag(t, cmd, "user-access-token", testUserToken)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	captureStdout(t, func() {
		if err := mergeForwardMsgCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("merge-forward 返回错误: %v", err)
		}
	})
	if gotAuth != testTenantAuth {
		t.Fatalf("merge-forward 应固定 Bot 身份，Authorization = %q", gotAuth)
	}
	if !strings.Contains(stderr.String(), "仅支持 Bot") {
		t.Fatalf("应在 stderr 提示忽略 User Token，got %q", stderr.String())
	}
}
