package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// pathGuardEnv 为本地路径守卫测试准备隔离环境：
//   - HOME 指向临时目录，其中建好 ~/.ssh（敏感目录）与一个"凭证"文件；
//   - 配置指向计数型 mock 服务端：任何请求（含 token 获取 / 刷新）都会被记录，
//     用于断言"敏感路径在第一次网络请求之前就被拒绝"。
type pathGuardEnv struct {
	home     string
	ssh      string
	secret   string // ~/.ssh 下真实存在的文件（输入类用例：拒绝原因必须是敏感目录而不是"不存在"）
	requests *atomic.Int64
}

func newPathGuardEnv(t *testing.T) *pathGuardEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(ssh, "fp-secret.md")
	if err := os.WriteFile(secret, []byte("# secret\n<div>x</div>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if mockAuthHandler(w, r) {
			return
		}
		// 业务请求一律返回错误：本测试只关心"是否发出了请求"，不让命令在 mock 数据上继续走下去
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":1999999,"msg":"fp-test: unexpected request"}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	return &pathGuardEnv{home: home, ssh: ssh, secret: secret, requests: &n}
}

// runGuardedCmd 以干净的 flag 状态执行命令 RunE（不经过 PersistentPreRunE，避免读真实配置）。
func runGuardedCmd(t *testing.T, c *cobra.Command, args []string, flags map[string]string) error {
	t.Helper()
	resetDriveCmdFlags(t, c)
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("设置 --%s 失败: %v", k, err)
		}
	}
	oldStdout := os.Stdout
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devnull
	defer func() {
		os.Stdout = oldStdout
		_ = devnull.Close()
	}()
	return c.RunE(c, args)
}

// assertRejectedBeforeNetwork：用法错误（退出码 2）、零网络请求、敏感目录下没有新文件。
func (e *pathGuardEnv) assertRejectedBeforeNetwork(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("应拒绝敏感 / 无效路径，实际成功")
	}
	if !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("应为用法错误（退出码 2），得到: %v", err)
	}
	if got := exitCodeFor(err); got != clierr.ExitUsage {
		t.Fatalf("退出码 = %d, want 2（err=%v）", got, err)
	}
	if wantMsg != "" && !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("错误信息应包含 %q，得到: %v", wantMsg, err)
	}
	if n := e.requests.Load(); n != 0 {
		t.Fatalf("拒绝前不应发出任何网络请求，实际 %d 次（err=%v）", n, err)
	}
	entries, _ := os.ReadDir(e.ssh)
	for _, en := range entries {
		if en.Name() != filepath.Base(e.secret) {
			t.Fatalf("~/.ssh 下不应出现新文件: %s", en.Name())
		}
	}
}

// TestOutputPathsRejectedBeforeNetwork 输出类命令：-o / --output-path / --output-dir / --assets-dir /
// --snapshot / 下载目录指向敏感目录时，在第一次网络请求（含 token 刷新、wiki 解析、创建导出任务）之前拒绝。
func TestOutputPathsRejectedBeforeNetwork(t *testing.T) {
	type tc struct {
		name  string
		cmd   *cobra.Command
		args  []string
		flags func(ssh string) map[string]string
		// cwdHome 表示以 HOME 为工作目录、用相对路径（.ssh/...）给出输出位置
		cwdHome bool
	}
	out := func(ssh, name string) string { return filepath.Join(ssh, name) }
	cases := []tc{
		{"doc export -o", exportMarkdownCmd, []string{"doxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.md")}
		}, false},
		{"doc export --engine docs_ai -o", exportMarkdownCmd, []string{"https://example.feishu.cn/wiki/wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.md"), "engine": "docs_ai"}
		}, false},
		{"doc export --assets-dir", exportMarkdownCmd, []string{"doxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"download-images": "true", "assets-dir": out(s, "assets")}
		}, false},
		{"doc export-file -o", exportFileCmd, []string{"https://example.feishu.cn/wiki/wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"type": "pdf", "output": out(s, "x.pdf")}
		}, false},
		{"drive export --output-dir", driveExportCmd, nil, func(s string) map[string]string {
			return map[string]string{"token": "doxcnfptestfake", "doc-type": "docx", "file-extension": "pdf", "output-dir": s}
		}, false},
		{"drive export --dry-run --output-dir", driveExportCmd, nil, func(s string) map[string]string {
			return map[string]string{"token": "doxcnfptestfake", "doc-type": "docx", "file-extension": "pdf", "output-dir": s, "dry-run": "true"}
		}, false},
		{"drive export-download --output-dir", driveExportDownloadCmd, nil, func(s string) map[string]string {
			return map[string]string{"file-token": "boxcnfptestfake", "output-dir": s}
		}, false},
		{"markdown fetch --output-path", markdownFetchCmd, nil, func(s string) map[string]string {
			return map[string]string{"file-token": "boxcnfptestfake", "output-path": out(s, "x.md")}
		}, false},
		{"wiki export -o", exportWikiCmd, []string{"wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.md")}
		}, false},
		{"wiki export --assets-dir", exportWikiCmd, []string{"wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"download-images": "true", "assets-dir": out(s, "assets")}
		}, false},
		{"wiki export-tree -o", exportWikiTreeCmd, []string{"wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output-dir": out(s, "tree")}
		}, false},
		{"wiki export-tree --assets-dir", exportWikiTreeCmd, []string{"wikcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output-dir": filepath.Join(filepath.Dir(s), "tree"), "download-images": "true", "assets-dir": out(s, "assets")}
		}, false},
		{"sheet export -o", sheetExportCmd, []string{"shtcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.xlsx")}
		}, false},
		{"board svg-export --output-path", boardSVGExportCmd, []string{"boxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output-path": out(s, "x.svg")}
		}, false},
		{"board export-code --output-path", boardExportCodeCmd, []string{"boxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output-path": out(s, "x.svg")}
		}, false},
		{"board image <output>", getBoardImageCmd, nil, nil, false}, // args 在下方按 ssh 填充
		{"board update --snapshot", boardUpdateCmd, nil, nil, false},
		{"doc media-download -o", docMediaDownloadCmd, []string{"boxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.png")}
		}, false},
		{"media download -o", downloadMediaCmd, []string{"boxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.png")}
		}, false},
		{"file download -o", downloadFileCmd, []string{"boxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"output": out(s, "x.bin")}
		}, false},
		{"drive download --output", driveDownloadCmd, nil, func(s string) map[string]string {
			return map[string]string{"file-token": "boxcnfptestfake", "output": out(s, "x.bin")}
		}, false},
		{"drive version-get --output", driveVersionGetCmd, nil, func(s string) map[string]string {
			return map[string]string{"file-token": "boxcnfptestfake", "version": "7694404161344900828", "output": out(s, "x.bin")}
		}, false},
		{"msg resource-download -o", msgResourceDownloadCmd, []string{"om_fptestfake", "file_fptestfake"}, func(s string) map[string]string {
			return map[string]string{"type": "file", "output": out(s, "x.bin")}
		}, false},
		{"minutes download -o", minutesDownloadCmd, nil, func(s string) map[string]string {
			return map[string]string{"minute-tokens": "obcnfptestfake0000000000", "output": out(s, "x.mp4")}
		}, false},
		{"vc notes --output-dir", vcNotesCmd, nil, func(s string) map[string]string {
			return map[string]string{"minute-tokens": "obcnfptestfake0000000000", "download-transcript": "true", "output-dir": s}
		}, false},
		{"bitable record download-attachment --output", bitableRecordDownloadAttachmentCmd, nil, func(s string) map[string]string {
			return map[string]string{"base-token": "https://example.feishu.cn/wiki/wikcnfptestfake", "table-id": "tblfp", "record-id": "recfp", "file-token": "boxcnfptestfake", "output": out(s, "x.bin")}
		}, false},
		{"doc htmlbox create -o（output 包）", docHtmlboxCreateCmd, []string{"doxcnfptestfake"}, func(s string) map[string]string {
			return map[string]string{"html": "<div>x</div>", "output": out(s, "x.json")}
		}, false},
		{"drive pull --local-dir（相对 cwd 的 .ssh）", drivePullCmd, nil, func(s string) map[string]string {
			return map[string]string{"folder-token": "fldcnfptestfake", "local-dir": ".ssh"}
		}, true},
		{"drive pull --delete-local 未加 --yes 也先报路径", drivePullCmd, nil, func(s string) map[string]string {
			return map[string]string{"folder-token": "fldcnfptestfake", "local-dir": ".ssh", "delete-local": "true"}
		}, true},
		{"event consume --output-dir（相对 cwd 的 .ssh）", eventConsumeCmd, []string{"im.message.receive_v1"}, func(s string) map[string]string {
			return map[string]string{"output-dir": ".ssh/events", "max-events": "1"}
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newPathGuardEnv(t)
			args, flags := c.args, map[string]string{}
			if c.flags != nil {
				flags = c.flags(env.ssh)
			}
			switch c.cmd {
			case getBoardImageCmd:
				args = []string{"boxcnfptestfake", filepath.Join(env.ssh, "board")}
			case boardUpdateCmd:
				nodes := filepath.Join(t.TempDir(), "nodes.json")
				if err := os.WriteFile(nodes, []byte(`[]`), 0o600); err != nil {
					t.Fatal(err)
				}
				args = []string{"boxcnfptestfake", nodes}
				flags = map[string]string{"overwrite": "true", "snapshot": filepath.Join(env.ssh, "snap.json")}
			}
			if c.cwdHome {
				wd, _ := os.Getwd()
				if err := os.Chdir(env.home); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = os.Chdir(wd) }()
			}
			err := runGuardedCmd(t, c.cmd, args, flags)
			env.assertRejectedBeforeNetwork(t, err, "~/.ssh")
		})
	}
}
