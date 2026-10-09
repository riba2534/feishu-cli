package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// TestMarkdownParamErrorsAreUsageErrors markdown 命令组的参数错误应归类为用法错误（退出码 2），
// 与其它命令一致；此前用 fmt.Errorf 返回，退出码 1 与运行期失败混在一起。
func TestMarkdownParamErrorsAreUsageErrors(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	cases := []struct {
		name  string
		cmd   *cobra.Command
		flags map[string]string
	}{
		{"patch 缺 pattern", markdownPatchCmd, map[string]string{"file-token": "boxcnX", "content": "x"}},
		{"patch 正则非法", markdownPatchCmd, map[string]string{"file-token": "boxcnX", "pattern": "(", "content": "x", "regex": "true"}},
		{"patch --name 非 .md", markdownPatchCmd, map[string]string{"file-token": "boxcnX", "pattern": "a", "content": "b", "name": "a.txt"}},
		{"overwrite 缺内容", markdownOverwriteCmd, map[string]string{"file-token": "boxcnX"}},
		{"create 内容冲突", markdownCreateCmd, map[string]string{"content": "x", "content-file": "a.md", "name": "a.md"}},
		{"fetch 缺 token", markdownFetchCmd, map[string]string{"file-token": " "}},
		{"diff context 为负", markdownDiffCmd, map[string]string{"file-token": "boxcnX", "file": "a.md", "context-lines": "-1"}},
		{"diff format 非法", markdownDiffCmd, map[string]string{"file-token": "boxcnX", "file": "a.md", "format": "xml"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var names []string
			for k, v := range tc.flags {
				if err := tc.cmd.Flags().Set(k, v); err != nil {
					t.Fatalf("set --%s: %v", k, err)
				}
				names = append(names, k)
			}
			defer resetCmdFlag(tc.cmd, names...)
			err := tc.cmd.RunE(tc.cmd, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("参数错误应为 usage（退出码 2）: %v", err)
			}
		})
	}
}

// TestMarkdownPatchRefusesSilentRename 读不到远端文件名时 patch 与 overwrite 一致 fail-closed，
// 不再退化为 <token>.md 静默重命名远端文件；显式 --name 时照常写回。
func TestMarkdownPatchRefusesSilentRename(t *testing.T) {
	_, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	var uploads int32
	var uploadedName atomic.Value
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/tenant_access_token"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot","expire":7200}`)
		case strings.Contains(r.URL.Path, "/preview_download"):
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = fmt.Fprint(w, "# 标题\n\nTODO 项\n")
		case r.URL.Path == "/open-apis/drive/v1/metas/batch_query":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"metas":[]}}`)
		case r.URL.Path == "/open-apis/drive/v1/files/upload_all":
			atomic.AddInt32(&uploads, 1)
			_ = r.ParseMultipartForm(1 << 20)
			uploadedName.Store(r.FormValue("file_name"))
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"boxcnX","version":"2"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()

	cmd := markdownPatchCmd
	_ = cmd.Flags().Set("file-token", "boxcnX")
	_ = cmd.Flags().Set("pattern", "TODO")
	_ = cmd.Flags().Set("content", "DONE")
	setCmdAs(t, cmd, "bot")
	defer resetCmdFlag(cmd, "file-token", "pattern", "content", "name", "as")
	defer setCmdAs(t, cmd, "auto")

	_, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err == nil || !strings.Contains(err.Error(), "拒绝") || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("读不到远端名时应 fail-closed 并提示 --name: %v", err)
	}
	if atomic.LoadInt32(&uploads) != 0 {
		t.Fatal("fail-closed 时不应写回远端")
	}

	_ = cmd.Flags().Set("name", "原文件.md")
	if _, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) }); err != nil {
		t.Fatalf("显式 --name 时应写回: %v", err)
	}
	if atomic.LoadInt32(&uploads) != 1 || uploadedName.Load() != "原文件.md" {
		t.Fatalf("写回文件名 = %v (uploads=%d)", uploadedName.Load(), uploads)
	}
}

// TestMarkdownDiffFormatHelpDefaultIsText diff 缺省输出 unified diff 文本，帮助里 --format 的默认值不能显示为 json。
func TestMarkdownDiffFormatHelpDefaultIsText(t *testing.T) {
	f := markdownDiffCmd.Flags().Lookup("format")
	if f == nil {
		t.Fatal("--format missing")
	}
	if f.DefValue != "" || !strings.Contains(f.Usage, "unified diff") {
		t.Fatalf("--format 默认值应为空（文本 diff）: def=%q usage=%q", f.DefValue, f.Usage)
	}
	if _, structured, err := resolveMarkdownDiffOutput(markdownDiffCmd); err != nil || structured {
		t.Fatalf("未传 --format/--jq 时应输出文本 diff: structured=%v err=%v", structured, err)
	}
}
