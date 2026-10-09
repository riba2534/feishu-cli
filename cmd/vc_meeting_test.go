package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

func newVCMeetingListActiveTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	addVCReadAsFlag(cmd)
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func TestVCMeetingGroupHasNoRunE(t *testing.T) {
	if vcMeetingCmd.RunE != nil || vcMeetingCmd.Run != nil {
		t.Fatal("vc meeting 是纯分组命令，不应手写 RunE")
	}
	found := false
	for _, sub := range vcMeetingCmd.Commands() {
		if sub == vcMeetingListActiveCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("list-active 未挂到 vc meeting 下")
	}
	if f := vcMeetingListActiveCmd.Flags().Lookup("as"); f == nil || f.DefValue != "user" {
		t.Fatal("list-active 应注册 --as，默认 user")
	}
}

// TestVCMeetingListActiveIdentity User 身份不带 user_id；Bot 身份必须带 user_id（open_id）。
func TestVCMeetingListActiveIdentity(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		var gotAuth, gotQuery string
		cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/open-apis/vc/v1/bots/user_active_meeting" {
				http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
				return
			}
			gotAuth = r.Header.Get("Authorization")
			gotQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"meetings":[{"meeting_id":"6911188411932033028","meeting_no":"123456789","meeting_title":"周会"},{"meeting_id":"6911188411932033029","meeting_title":"评审"}]}}`)
		})
		defer cleanup()
		cmd := newVCMeetingListActiveTestCmd()
		mustSetFlag(t, cmd, "user-access-token", testUserToken)
		out, err := captureVCBotStdout(t, func() error { return vcMeetingListActiveCmd.RunE(cmd, nil) })
		if err != nil {
			t.Fatalf("list-active 返回错误: %v", err)
		}
		if gotAuth != "Bearer "+testUserToken || gotQuery != "" {
			t.Fatalf("auth=%q query=%q，User 身份不应带 user_id", gotAuth, gotQuery)
		}
		for _, want := range []string{"共 2 个", "6911188411932033028", "123456789", "同时在多个会议中"} {
			if !strings.Contains(out, want) {
				t.Fatalf("输出缺少 %q:\n%s", want, out)
			}
		}
	})

	t.Run("bot 缺 user-id 前置报错不联网", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		var hits int32
		cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			http.Error(w, "不应联网", http.StatusInternalServerError)
		})
		defer cleanup()
		cmd := newVCMeetingListActiveTestCmd()
		mustSetFlag(t, cmd, "as", "bot")
		err := vcMeetingListActiveCmd.RunE(cmd, nil)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "--user-id") {
			t.Fatalf("应为用法错误，实际 %v", err)
		}
		mustSetFlag(t, cmd, "user-id", "u123")
		if err := vcMeetingListActiveCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "ou_") {
			t.Fatalf("非 open_id 应报错，实际 %v", err)
		}
		if n := atomic.LoadInt32(&hits); n != 0 {
			t.Fatalf("不应联网，实际 %d 次", n)
		}
	})

	t.Run("bot 带 user-id", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
		var gotAuth, gotUserID string
		cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotUserID = r.URL.Query().Get("user_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"meetings":[]}}`)
		}))
		defer cleanup()
		cmd := newVCMeetingListActiveTestCmd()
		mustSetFlag(t, cmd, "as", "bot")
		mustSetFlag(t, cmd, "user-id", "ou_xxx")
		out, err := captureVCBotStdout(t, func() error { return vcMeetingListActiveCmd.RunE(cmd, nil) })
		if err != nil {
			t.Fatalf("list-active --as bot 返回错误: %v", err)
		}
		if gotAuth != testTenantAuth || gotUserID != "ou_xxx" {
			t.Fatalf("auth=%q user_id=%q", gotAuth, gotUserID)
		}
		if !strings.Contains(out, "没有进行中的会议") {
			t.Fatalf("空结果提示缺失: %s", out)
		}
	})
}
