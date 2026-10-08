package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// stubConfirmIO 注入确认门禁的交互判定与输入输出，返回提示输出缓冲区。
func stubConfirmIO(t *testing.T, interactive bool, input string) *bytes.Buffer {
	t.Helper()
	origInteractive, origInput, origOut, origYes := confirmIsInteractive, confirmInput, confirmPromptOut, assumeYes
	t.Cleanup(func() {
		confirmIsInteractive, confirmInput, confirmPromptOut, assumeYes = origInteractive, origInput, origOut, origYes
	})
	var prompt bytes.Buffer
	confirmIsInteractive = func() bool { return interactive }
	confirmInput = strings.NewReader(input)
	confirmPromptOut = &prompt
	assumeYes = false
	return &prompt
}

func newConfirmTestCmd(t *testing.T, flags ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatalf("解析 flag 失败: %v", err)
	}
	return cmd
}

func TestConfirmDangerousAction(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		input       string
		flags       []string
		globalYes   bool
		wantExit    int // 0 表示放行
		wantPrompt  bool
	}{
		{name: "非交互未确认 → 需要确认", interactive: false, input: "y\n", wantExit: clierr.ExitConfirmationRequired},
		{name: "非交互 + --yes 放行", interactive: false, flags: []string{"--yes"}},
		{name: "非交互 + --force 放行", interactive: false, flags: []string{"--force"}},
		{name: "非交互 + 全局 --yes 放行", interactive: false, globalYes: true},
		{name: "交互输入 y 放行", interactive: true, input: "y\n", wantPrompt: true},
		{name: "交互输入 YES 放行", interactive: true, input: "YES\n", wantPrompt: true},
		{name: "交互输入 n → 已取消", interactive: true, input: "n\n", wantExit: clierr.ExitGeneral, wantPrompt: true},
		{name: "交互直接 EOF → 已取消", interactive: true, input: "", wantExit: clierr.ExitGeneral, wantPrompt: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := stubConfirmIO(t, tc.interactive, tc.input)
			assumeYes = tc.globalYes
			err := confirmDangerousAction(newConfirmTestCmd(t, tc.flags...), "确定要删除 X 吗？")
			if tc.wantExit == 0 {
				if err != nil {
					t.Fatalf("期望放行，得到错误: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("期望返回错误（非 0 退出），实际放行")
				}
				if got := exitCodeFor(err); got != tc.wantExit {
					t.Fatalf("退出码 = %d, want %d（err=%v）", got, tc.wantExit, err)
				}
			}
			if got := prompt.Len() > 0; got != tc.wantPrompt {
				t.Fatalf("是否打印提示 = %v, want %v（提示内容 %q）", got, tc.wantPrompt, prompt.String())
			}
		})
	}
}

func TestConfirmDangerousActionNonInteractiveMessageMentionsYes(t *testing.T) {
	stubConfirmIO(t, false, "")
	err := confirmDangerousAction(newConfirmTestCmd(t), "确定要删除文件 tok 吗？")
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "未执行") {
		t.Fatalf("非交互错误应提示 --yes 且说明未执行，得到: %v", err)
	}
}

// TestFileDeleteNonInteractiveDoesNotDelete 回归：非交互 `file delete` 过去打印"操作已取消"并 exit 0，
// Agent 会误以为删除成功。现在必须以退出码 10 失败，且不发出删除请求；--force 才真正删除。
func TestFileDeleteNonInteractiveDoesNotDelete(t *testing.T) {
	var deleteHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/open-apis/drive/v1/files/doxcnFAKE":
			atomic.AddInt32(&deleteHits, 1)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"task_id":"task_1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/drive/v1/files/task_check":
			// file delete 返回 task_id 后会轮询 task_check
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"status":"success"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	setFlag := func(name, value string) {
		t.Helper()
		if err := deleteFileCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("设置 --%s 失败: %v", name, err)
		}
	}
	setFlag("type", "docx")
	t.Cleanup(func() {
		_ = deleteFileCmd.Flags().Set("type", "")
		_ = deleteFileCmd.Flags().Set("force", "false")
	})

	stubConfirmIO(t, false, "")
	stdout := captureStdout(t, func() {
		err := deleteFileCmd.RunE(deleteFileCmd, []string{"doxcnFAKE"})
		if got := exitCodeFor(err); got != clierr.ExitConfirmationRequired {
			t.Fatalf("非交互未确认的删除应以退出码 10 失败，得到 %d（err=%v）", got, err)
		}
	})
	if hits := atomic.LoadInt32(&deleteHits); hits != 0 {
		t.Fatalf("未确认时不应发出删除请求，实际 %d 次", hits)
	}
	if strings.Contains(stdout, "操作已取消") {
		t.Fatalf("stdout 不应再出现误导性的「操作已取消」: %q", stdout)
	}

	setFlag("force", "true")
	captureStdout(t, func() {
		if err := deleteFileCmd.RunE(deleteFileCmd, []string{"doxcnFAKE"}); err != nil {
			t.Fatalf("--force 应跳过确认并删除，得到错误: %v", err)
		}
	})
	if hits := atomic.LoadInt32(&deleteHits); hits != 1 {
		t.Fatalf("--force 后应发出 1 次删除请求，实际 %d 次", hits)
	}
}
