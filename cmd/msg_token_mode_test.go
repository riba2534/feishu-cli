package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

const (
	testChatID     = "oc_test_chat"
	testMessageID  = "om_test_message"
	testUserToken  = "u-test-token"
	testTenantAuth = "Bearer t-fake"
)

func isolateMsgTokenTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_BASE_URL", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
}

func tenantTokenHandler(t *testing.T, business http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		business(w, r)
	}
}

func newListMessagesTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("container-id", "", "")
	cmd.Flags().String("container-id-type", "chat", "")
	cmd.Flags().String("start-time", "", "")
	cmd.Flags().String("end-time", "", "")
	cmd.Flags().String("sort-type", "", "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	addCardContentTypeFlag(cmd)
	return cmd
}

func newGetMessageTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	addCardContentTypeFlag(cmd)
	return cmd
}

func newGetMessageHistoryTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("container-id-type", "chat", "")
	cmd.Flags().String("container-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("user-email", "", "")
	cmd.Flags().String("start-time", "", "")
	cmd.Flags().String("end-time", "", "")
	cmd.Flags().String("sort-type", "ByCreateTimeDesc", "")
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	addCardContentTypeFlag(cmd)
	return cmd
}

func mustSetFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("设置 flag %s 失败: %v", name, err)
	}
}

func TestMsgListContainerIDFallsBackToTenantTokenWhenNotLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var capturedAuth string
	var searchCalled bool

	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/search/v2/message" {
			searchCalled = true
		}
		if r.URL.Path != "/open-apis/im/v1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		if got := r.URL.Query().Get("card_msg_content_type"); got != client.CardMsgContentTypeUser {
			t.Errorf("card_msg_content_type = %q, want %q", got, client.CardMsgContentTypeUser)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":true,"page_token":"next"}}`)
	}))
	defer cleanup()

	cmd := newListMessagesTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	if err := listMessagesCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("msg list 返回错误: %v", err)
	}
	if capturedAuth != testTenantAuth {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, testTenantAuth)
	}
	if searchCalled {
		t.Fatalf("未显式 User Token 时不应触发 search fallback")
	}
}

func TestMsgGetFallsBackToTenantTokenWhenNotLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var capturedAuth string

	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/messages/"+testMessageID {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"text","sender":{"id":"ou_sender","sender_type":"user"},"body":{"content":"{\"text\":\"hi\"}"}}]}}`, testMessageID)
	}))
	defer cleanup()

	cmd := newGetMessageTestCmd()
	if err := getMessageCmd.RunE(cmd, []string{testMessageID}); err != nil {
		t.Fatalf("msg get 返回错误: %v", err)
	}
	if capturedAuth != testTenantAuth {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, testTenantAuth)
	}
}

func TestMsgHistoryContainerIDFallsBackToTenantTokenWhenNotLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var capturedAuth string
	var searchCalled bool
	var contactCalled bool

	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open-apis/search/v2/message":
			searchCalled = true
		case strings.HasPrefix(r.URL.Path, "/open-apis/contact/"):
			contactCalled = true
		case r.URL.Path == "/open-apis/im/v1/messages":
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"om_1","msg_type":"text","sender":{"id":"ou_sender","sender_type":"user"},"body":{"content":"{\"text\":\"hi\"}"}}],"has_more":true,"page_token":"next"}}`)
			return
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	}))
	defer cleanup()

	cmd := newGetMessageHistoryTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("msg history 返回错误: %v", err)
	}
	if capturedAuth != testTenantAuth {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, testTenantAuth)
	}
	if searchCalled {
		t.Fatalf("未显式 User Token 时不应触发 search fallback")
	}
	if contactCalled {
		t.Fatalf("未显式 User Token 时 sender name 解析不应调用 contact API")
	}
}

func TestMsgContainerCommandsUseExplicitUserToken(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "list",
			run: func(t *testing.T) {
				cmd := newListMessagesTestCmd()
				mustSetFlag(t, cmd, "container-id", testChatID)
				mustSetFlag(t, cmd, "user-access-token", testUserToken)
				if err := listMessagesCmd.RunE(cmd, nil); err != nil {
					t.Fatalf("msg list 返回错误: %v", err)
				}
			},
		},
		{
			name: "get",
			run: func(t *testing.T) {
				cmd := newGetMessageTestCmd()
				mustSetFlag(t, cmd, "user-access-token", testUserToken)
				if err := getMessageCmd.RunE(cmd, []string{testMessageID}); err != nil {
					t.Fatalf("msg get 返回错误: %v", err)
				}
			},
		},
		{
			name: "history",
			run: func(t *testing.T) {
				cmd := newGetMessageHistoryTestCmd()
				mustSetFlag(t, cmd, "container-id", testChatID)
				mustSetFlag(t, cmd, "user-access-token", testUserToken)
				if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
					t.Fatalf("msg history 返回错误: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			var capturedAuth string

			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					t.Errorf("%s 显式 User Token 时不应请求 tenant token", tt.name)
					http.Error(w, "unexpected tenant token request", http.StatusInternalServerError)
					return
				}
				capturedAuth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/open-apis/im/v1/messages":
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
				case "/open-apis/im/v1/messages/" + testMessageID:
					_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"text","body":{"content":"{\"text\":\"hi\"}"}}]}}`, testMessageID)
				default:
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
				}
			})
			defer cleanup()

			tt.run(t)
			if capturedAuth != "Bearer "+testUserToken {
				t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer "+testUserToken)
			}
		})
	}
}

func TestMsgHistoryUserEntrypointsStillRequireUserToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	})
	defer cleanup()

	cmd := newGetMessageHistoryTestCmd()
	mustSetFlag(t, cmd, "user-id", "ou_test_user")
	err := getMessageHistoryCmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("--user-id 未提供 User Token 时应返回错误")
	}
	if !strings.Contains(err.Error(), "缺少 User Access Token") {
		t.Fatalf("错误 = %q, want 包含 缺少 User Access Token", err.Error())
	}
}

// 用户登录后（token.json 存在），读类命令优先用 User Token，未登录才回落 Tenant Token。
// 这与"User 权限 ⊇ Bot 权限"的常识一致，且避免外部群读取时返回 230002 "Bot/User can NOT be out of the chat"。

func writeFakeUserToken(t *testing.T, accessToken string) {
	t.Helper()
	if err := auth.SaveToken(&auth.TokenStore{
		AccessToken:      accessToken,
		RefreshToken:     "r-fake",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "im:message:readonly",
	}); err != nil {
		t.Fatalf("写假 token.json 失败: %v", err)
	}
}

func TestMsgHistoryContainerIDPrefersUserTokenWhenLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	writeFakeUserToken(t, "u-from-token-json")
	var capturedAuth string
	var tenantTokenRequested bool

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			tenantTokenRequested = true
			http.Error(w, "should not request tenant token when User Token available", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/open-apis/im/v1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
	})
	defer cleanup()

	cmd := newGetMessageHistoryTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	if err := getMessageHistoryCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("msg history 返回错误: %v", err)
	}
	if tenantTokenRequested {
		t.Fatal("已登录时不应请求 tenant token")
	}
	if capturedAuth != "Bearer u-from-token-json" {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer u-from-token-json")
	}
}

func TestMsgListContainerIDPrefersUserTokenWhenLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	writeFakeUserToken(t, "u-from-token-json")
	var capturedAuth string
	var tenantTokenRequested bool

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			tenantTokenRequested = true
			http.Error(w, "should not request tenant token when User Token available", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/open-apis/im/v1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
	})
	defer cleanup()

	cmd := newListMessagesTestCmd()
	mustSetFlag(t, cmd, "container-id", testChatID)
	if err := listMessagesCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("msg list 返回错误: %v", err)
	}
	if tenantTokenRequested {
		t.Fatal("已登录时不应请求 tenant token")
	}
	if capturedAuth != "Bearer u-from-token-json" {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer u-from-token-json")
	}
}

func TestMsgGetPrefersUserTokenWhenLoggedIn(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	writeFakeUserToken(t, "u-from-token-json")
	var capturedAuth string
	var tenantTokenRequested bool

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			tenantTokenRequested = true
			http.Error(w, "should not request tenant token when User Token available", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/open-apis/im/v1/messages/"+testMessageID {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"text","sender":{"id":"ou_sender","sender_type":"user"},"body":{"content":"{\"text\":\"hi\"}"}}]}}`, testMessageID)
	})
	defer cleanup()

	cmd := newGetMessageTestCmd()
	if err := getMessageCmd.RunE(cmd, []string{testMessageID}); err != nil {
		t.Fatalf("msg get 返回错误: %v", err)
	}
	if tenantTokenRequested {
		t.Fatal("已登录时不应请求 tenant token")
	}
	if capturedAuth != "Bearer u-from-token-json" {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer u-from-token-json")
	}
}
