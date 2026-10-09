package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func resetCobraFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

func TestSearchMessagesAndChatsIdentityWire(t *testing.T) {
	cases := []struct {
		name      string
		as        string
		userTok   string
		wantAuth  string
		wantErr   string
		cmd       *cobra.Command
		args      []string
		setFlags  func(*cobra.Command)
		pathCheck string
	}{
		{
			name:      "messages --as user",
			as:        "user",
			userTok:   testUserToken,
			wantAuth:  "Bearer " + testUserToken,
			cmd:       searchMessagesCmd,
			args:      []string{"hello"},
			pathCheck: "/open-apis/im/v1/messages/search",
		},
		{
			name:      "messages --as bot",
			as:        "bot",
			wantAuth:  testTenantAuth,
			cmd:       searchMessagesCmd,
			args:      []string{"hello"},
			pathCheck: "/open-apis/im/v1/messages/search",
		},
		{
			name:      "messages --as auto no user",
			as:        "auto",
			wantAuth:  testTenantAuth,
			cmd:       searchMessagesCmd,
			args:      []string{"hello"},
			pathCheck: "/open-apis/im/v1/messages/search",
		},
		{
			name:      "chats --as user",
			as:        "user",
			userTok:   testUserToken,
			wantAuth:  "Bearer " + testUserToken,
			cmd:       searchChatsCmd,
			pathCheck: "/open-apis/im/v2/chats/search",
			setFlags: func(c *cobra.Command) {
				_ = c.Flags().Set("query", "team")
			},
		},
		{
			name:      "chats --as bot",
			as:        "bot",
			wantAuth:  testTenantAuth,
			cmd:       searchChatsCmd,
			pathCheck: "/open-apis/im/v2/chats/search",
			setFlags: func(c *cobra.Command) {
				_ = c.Flags().Set("query", "team")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			resetCobraFlags(tc.cmd)
			t.Cleanup(func() { resetCobraFlags(tc.cmd) })

			var capturedAuth string
			cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tc.pathCheck {
					capturedAuth = r.Header.Get("Authorization")
					w.Header().Set("Content-Type", "application/json")
					if strings.Contains(r.URL.Path, "messages/search") {
						_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
					}
					return
				}
				http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			}))
			defer cleanup()

			_ = tc.cmd.Flags().Set("as", tc.as)
			if tc.userTok != "" {
				_ = tc.cmd.Flags().Set("user-access-token", tc.userTok)
			}
			if tc.setFlags != nil {
				tc.setFlags(tc.cmd)
			}
			err := tc.cmd.RunE(tc.cmd, tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunE: %v", err)
			}
			if capturedAuth != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", capturedAuth, tc.wantAuth)
			}
		})
	}
}

func TestSearchMessagesAsUserMissingToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(searchMessagesCmd)
	t.Cleanup(func() { resetCobraFlags(searchMessagesCmd) })
	var n int32
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer cleanup()
	_ = searchMessagesCmd.Flags().Set("as", "user")
	err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"})
	if err == nil {
		t.Fatal("--as user without token must fail")
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("must not hit network, got %d", n)
	}
}

func TestSearchMessagesAutoFailClosedOnRefreshError(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(searchMessagesCmd)
	t.Cleanup(func() { resetCobraFlags(searchMessagesCmd) })

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	tokenDir := filepath.Join(tmpHome, ".feishu-cli")
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     "broken-refresh-token",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "search:message",
	}
	data, _ := json.MarshalIndent(store, "", "  ")
	if err := os.WriteFile(filepath.Join(tokenDir, "token.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	var biz int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/authen/v2/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/open-apis/im/v1/messages/search") {
			atomic.AddInt32(&biz, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[]}}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Errorf("refresh 失败时不应请求 tenant token 切 Bot")
		}
	})
	defer cleanup()

	_ = searchMessagesCmd.Flags().Set("as", "auto")
	err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"})
	if err == nil {
		t.Fatal("auto + expired user token must fail-closed")
	}
	if atomic.LoadInt32(&biz) != 0 {
		t.Fatalf("must not send bot business request, got %d", biz)
	}
}

func TestSearchMessagesFilterOnlyAndInvalidBeforeNetwork(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(searchMessagesCmd)
	t.Cleanup(func() { resetCobraFlags(searchMessagesCmd) })

	var n int32
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/im/v1/messages/search" {
			atomic.AddInt32(&n, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[]}}`)
			return
		}
		http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
	}))
	defer cleanup()

	_ = searchMessagesCmd.Flags().Set("as", "bot")
	_ = searchMessagesCmd.Flags().Set("chat-ids", "oc_1")
	if err := searchMessagesCmd.RunE(searchMessagesCmd, nil); err != nil {
		t.Fatalf("filter-only search: %v", err)
	}
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("filter-only hits = %d", n)
	}

	atomic.StoreInt32(&n, 0)
	resetCobraFlags(searchMessagesCmd)
	_ = searchMessagesCmd.Flags().Set("page-size", "51")
	if err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"}); err == nil {
		t.Fatal("page-size 51 must fail")
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("invalid page-size hit network %d", n)
	}

	atomic.StoreInt32(&n, 0)
	resetCobraFlags(searchMessagesCmd)
	_ = searchMessagesCmd.Flags().Set("start-time", "2026-04-27")
	_ = searchMessagesCmd.Flags().Set("end-time", "2026-04-20")
	if err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"}); err == nil {
		t.Fatal("start>end must fail")
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("start>end hit network %d", n)
	}
}

func TestSearchChatsPageAllRepeatedCursorFails(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(searchChatsCmd)
	t.Cleanup(func() { resetCobraFlags(searchChatsCmd) })

	var n int32
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v2/chats/search" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		atomic.AddInt32(&n, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"meta_data":{"chat_id":"oc_1","name":"n"}}],"has_more":true,"page_token":"same"}}`)
	}))
	defer cleanup()
	_ = searchChatsCmd.Flags().Set("as", "bot")
	_ = searchChatsCmd.Flags().Set("query", "n")
	_ = searchChatsCmd.Flags().Set("page-all", "true")
	_ = searchChatsCmd.Flags().Set("page-token", "same")
	err := searchChatsCmd.RunE(searchChatsCmd, nil)
	if err == nil {
		t.Fatal("repeated cursor must fail")
	}
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("want 1 page then stop, got %d", n)
	}
}

func TestCalendarAgendaAndEventSearchIdentityWire(t *testing.T) {
	cases := []struct {
		name     string
		as       string
		userTok  string
		wantAuth string
		cmd      *cobra.Command
		args     []string
		pathHas  string
	}{
		{name: "agenda user", as: "user", userTok: testUserToken, wantAuth: "Bearer " + testUserToken, cmd: calendarAgendaCmd, pathHas: "/events/instance_view"},
		{name: "agenda bot", as: "bot", wantAuth: testTenantAuth, cmd: calendarAgendaCmd, pathHas: "/events/instance_view"},
		{name: "event-search user", as: "user", userTok: testUserToken, wantAuth: "Bearer " + testUserToken, cmd: calendarEventSearchCmd, pathHas: "/events/search_event"},
		{name: "event-search bot", as: "bot", wantAuth: testTenantAuth, cmd: calendarEventSearchCmd, pathHas: "/events/search_event"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			resetCobraFlags(tc.cmd)
			t.Cleanup(func() { resetCobraFlags(tc.cmd) })
			var capturedAuth string
			cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, tc.pathHas) {
					capturedAuth = r.Header.Get("Authorization")
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
					return
				}
				http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			}))
			defer cleanup()
			_ = tc.cmd.Flags().Set("as", tc.as)
			if tc.userTok != "" {
				_ = tc.cmd.Flags().Set("user-access-token", tc.userTok)
			}
			if err := tc.cmd.RunE(tc.cmd, tc.args); err != nil {
				t.Fatalf("RunE: %v", err)
			}
			if capturedAuth != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", capturedAuth, tc.wantAuth)
			}
		})
	}
}

func TestCalendarAutoFailClosedOnCorruptToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"agenda", calendarAgendaCmd},
		{"event-search", calendarEventSearchCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			resetCobraFlags(tc.cmd)
			t.Cleanup(func() { resetCobraFlags(tc.cmd) })
			tmpHome := t.TempDir()
			t.Setenv("HOME", tmpHome)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
			tokenDir := filepath.Join(tmpHome, ".feishu-cli")
			if err := os.MkdirAll(tokenDir, 0o700); err != nil {
				t.Fatal(err)
			}
			store := auth.TokenStore{
				AccessToken:      "expired-access-token",
				RefreshToken:     "broken-refresh-token",
				TokenType:        "Bearer",
				ExpiresAt:        time.Now().Add(-time.Hour),
				RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				Scope:            "calendar:calendar.event:read",
			}
			data, _ := json.MarshalIndent(store, "", "  ")
			if err := os.WriteFile(filepath.Join(tokenDir, "token.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			var biz, tenant int32
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/open-apis/authen/v2/oauth/token" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
					return
				}
				if strings.Contains(r.URL.Path, "/calendars/") {
					atomic.AddInt32(&biz, 1)
					return
				}
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					atomic.AddInt32(&tenant, 1)
					t.Errorf("corrupt User token must not fall back to Bot")
				}
			})
			defer cleanup()
			_ = tc.cmd.Flags().Set("as", "auto")
			if err := tc.cmd.RunE(tc.cmd, nil); err == nil {
				t.Fatal("auto + corrupt user token must fail-closed")
			}
			if atomic.LoadInt32(&biz) != 0 || atomic.LoadInt32(&tenant) != 0 {
				t.Fatalf("biz=%d tenant=%d, want 0", biz, tenant)
			}
		})
	}
}

func TestSearchMessagesInvalidFormatJQBeforeIdentity(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(searchMessagesCmd)
	t.Cleanup(func() { resetCobraFlags(searchMessagesCmd) })
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	tokenDir := filepath.Join(tmpHome, ".feishu-cli")
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     "broken-refresh-token",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "search:message",
	}
	data, _ := json.MarshalIndent(store, "", "  ")
	if err := os.WriteFile(filepath.Join(tokenDir, "token.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	var refresh, biz int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/authen/v2/oauth/token" {
			atomic.AddInt32(&refresh, 1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		atomic.AddInt32(&biz, 1)
	})
	defer cleanup()

	_ = searchMessagesCmd.Flags().Set("as", "auto")
	_ = searchMessagesCmd.Flags().Set("jq", "not(")
	if err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"}); err == nil {
		t.Fatal("invalid jq must fail")
	}
	if atomic.LoadInt32(&refresh) != 0 || atomic.LoadInt32(&biz) != 0 {
		t.Fatalf("invalid jq must not refresh or call API: refresh=%d biz=%d", refresh, biz)
	}

	resetCobraFlags(searchMessagesCmd)
	_ = searchMessagesCmd.Flags().Set("as", "auto")
	_ = searchMessagesCmd.Flags().Set("format", "xml")
	if err := searchMessagesCmd.RunE(searchMessagesCmd, []string{"q"}); err == nil {
		t.Fatal("invalid format must fail")
	}
	if atomic.LoadInt32(&refresh) != 0 || atomic.LoadInt32(&biz) != 0 {
		t.Fatalf("invalid format must not refresh or call API: refresh=%d biz=%d", refresh, biz)
	}
}

func TestNegativePageLimitRejectedBeforeNetwork(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		args []string
		prep func(*cobra.Command)
	}{
		{"messages", searchMessagesCmd, []string{"q"}, func(c *cobra.Command) {
			_ = c.Flags().Set("page-all", "true")
			_ = c.Flags().Set("page-limit", "-1")
		}},
		{"chats", searchChatsCmd, nil, func(c *cobra.Command) {
			_ = c.Flags().Set("query", "n")
			_ = c.Flags().Set("page-all", "true")
			_ = c.Flags().Set("page-limit", "-1")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetCobraFlags(tc.cmd)
			t.Cleanup(func() { resetCobraFlags(tc.cmd) })
			var n int32
			cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&n, 1)
				http.Error(w, "nope", http.StatusTeapot)
			}))
			defer cleanup()
			_ = tc.cmd.Flags().Set("as", "bot")
			tc.prep(tc.cmd)
			if err := tc.cmd.RunE(tc.cmd, tc.args); err == nil {
				t.Fatal("negative page-limit must fail")
			}
			if atomic.LoadInt32(&n) != 0 {
				t.Fatalf("must not hit network, got %d", n)
			}
		})
	}
}

func TestEventSearchJSONHasMoreWithoutToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(calendarEventSearchCmd)
	t.Cleanup(func() { resetCobraFlags(calendarEventSearchCmd) })
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/search_event") {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"meta_data":{"event_id":"evt_h","summary":"x"}}],"has_more":true,"page_token":""}}`)
	}))
	defer cleanup()
	_ = calendarEventSearchCmd.Flags().Set("as", "bot")
	_ = calendarEventSearchCmd.Flags().Set("output", "json")
	_ = calendarEventSearchCmd.Flags().Set("query", "x")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := calendarEventSearchCmd.RunE(calendarEventSearchCmd, nil)
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("RunE: %v", runErr)
	}
	if !strings.Contains(string(out), `"has_more": true`) && !strings.Contains(string(out), `"has_more":true`) {
		t.Fatalf("JSON missing has_more true: %s", out)
	}
	if !strings.Contains(string(out), `"events"`) || !strings.Contains(string(out), `"next_page_token"`) {
		t.Fatalf("JSON missing events/next_page_token: %s", out)
	}
}

func TestEventSearchAndAgendaValidateBeforeNetwork(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	resetCobraFlags(calendarEventSearchCmd)
	resetCobraFlags(calendarAgendaCmd)
	t.Cleanup(func() {
		resetCobraFlags(calendarEventSearchCmd)
		resetCobraFlags(calendarAgendaCmd)
	})

	var n int32
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer cleanup()

	_ = calendarEventSearchCmd.Flags().Set("page-size", "31")
	if err := calendarEventSearchCmd.RunE(calendarEventSearchCmd, nil); err == nil {
		t.Fatal("event-search page-size 31 must fail")
	}
	_ = calendarEventSearchCmd.Flags().Set("page-size", "20")
	_ = calendarEventSearchCmd.Flags().Set("start", "2026-04-27")
	_ = calendarEventSearchCmd.Flags().Set("end", "2026-04-20")
	if err := calendarEventSearchCmd.RunE(calendarEventSearchCmd, nil); err == nil {
		t.Fatal("event-search start>end must fail")
	}

	_ = calendarAgendaCmd.Flags().Set("start-date", "2026-04-27")
	_ = calendarAgendaCmd.Flags().Set("end-date", "2026-04-20")
	if err := calendarAgendaCmd.RunE(calendarAgendaCmd, nil); err == nil {
		t.Fatal("agenda start>end must fail")
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("validation must not hit network, got %d", n)
	}
}
