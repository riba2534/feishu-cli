package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// guardHome 把 HOME 指向临时目录并建好 ~/.ssh 下的一个"凭证"文件，返回该文件路径。
func guardHome(t *testing.T) (home, secret string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret = filepath.Join(home, ".ssh", "id_test.png")
	if err := os.WriteFile(secret, []byte("\x89PNG\r\n\x1a\n-secret-"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, secret
}

// countingServer 记录任何请求（含 token 获取），用于断言"拒绝发生在网络请求之前"。
func countingServer(t *testing.T) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"t-x","expire":7200,"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)
	return &n
}

func assertUsageNoRequest(t *testing.T, name string, err error, n *atomic.Int64) {
	t.Helper()
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("%s: 应以用法错误拒绝敏感目录，得到 %v", name, err)
	}
	if got := n.Load(); got != 0 {
		t.Fatalf("%s: 拒绝前不应发出请求，实际 %d 次", name, got)
	}
}

// 输出：画板缩略图与预签名下载在发请求前拒绝敏感目录。
func TestDownloadOutputsRejectSensitiveBeforeRequest(t *testing.T) {
	home, _ := guardHome(t)
	n := countingServer(t)
	ssh := filepath.Join(home, ".ssh")

	_, err := GetBoardImage("wbfptestfake", filepath.Join(ssh, "board"), "u-test")
	assertUsageNoRequest(t, "GetBoardImage", err, n)

	_, err = DownloadFromPresignedURL("https://example.com/a.mp4", "a.mp4", DownloadOptions{OutputDir: ssh})
	assertUsageNoRequest(t, "DownloadFromPresignedURL", err, n)

	err = DownloadFromURL("https://example.com/a.png", filepath.Join(ssh, "a.png"))
	assertUsageNoRequest(t, "DownloadFromURL", err, n)

	err = DownloadExportFile("boxcnfptestfake", filepath.Join(ssh, "x.pdf"), "")
	assertUsageNoRequest(t, "DownloadExportFile", err, n)
}

// 输入：上传 / 读取本地文件的 client 入口兜底拒绝敏感目录（命令层漏校验时也不会把凭证发往远端）。
func TestUploadInputsRejectSensitiveBeforeRequest(t *testing.T) {
	_, secret := guardHome(t)
	n := countingServer(t)

	calls := map[string]func() error{
		"UploadIMImage": func() error { _, err := UploadIMImage(secret, ""); return err },
		"UploadIMFileWithOptions": func() error {
			_, err := UploadIMFileWithOptions(secret, "", "", 0)
			return err
		},
		"UploadFileWithToken": func() error { _, err := UploadFileWithToken(secret, "", "", ""); return err },
		"UploadMediaWithExtra": func() error {
			_, _, err := UploadMediaWithExtra(secret, "docx_image", "doxcnfp", "a.png", "")
			return err
		},
		"UploadMediaForImport": func() error {
			_, err := UploadMediaForImport(secret, "a.png", "docx", "png", "")
			return err
		},
		"OverwriteDriveFileFromPath": func() error {
			_, err := OverwriteDriveFileFromPath(secret, "", "a.png", "boxcnfp", "")
			return err
		},
		"UploadDocMedia": func() error {
			_, err := UploadDocMedia(secret, "docx_image", "doxcnfp", "a.png", "doxcnfp", "")
			return err
		},
		"UploadBitableAttachment": func() error { _, err := UploadBitableAttachment(secret, "bascnfp", ""); return err },
		"UploadSheetImageMediaAuto": func() error {
			_, err := UploadSheetImageMediaAuto(secret, "shtcnfp", "a.png")
			return err
		},
		"WriteSheetImage": func() error {
			return WriteSheetImage(context.Background(), "shtcnfp", "s!A1:A1", secret, "a.png")
		},
		"UploadOKRImage": func() error {
			_, err := UploadOKRImage(secret, "7000000000000000001", OKRTargetObjective, "")
			return err
		},
		"UploadTaskAttachment": func() error {
			_, err := UploadTaskAttachment("task", "guid", secret, "")
			return err
		},
		"UploadMarkdownFile": func() error {
			_, err := UploadMarkdownFile(MarkdownUploadSpec{FileName: "a.md"}, secret, "")
			return err
		},
		"ReadLocalMarkdownLimited": func() error {
			_, err := ReadLocalMarkdownLimited(secret, 1<<20)
			return err
		},
		"ImportDiagram": func() error {
			_, _, err := ImportDiagram("wbfp", secret, ImportDiagramOptions{Syntax: "mermaid"})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			assertUsageNoRequest(t, name, call(), n)
		})
	}
}

// 邮件内嵌图片：home 子树属于安全根，但 ~/.ssh 等凭证目录仍须拒绝。
func TestLoadInlineImageBytesRejectsSensitiveHomeDir(t *testing.T) {
	_, secret := guardHome(t)
	err := LoadInlineImageBytes(&MailInlineImageRef{LocalPath: secret})
	if err == nil || !strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("~/.ssh 下的图片应被拒绝，得到 %v", err)
	}
}
