package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestInputPathsRejectedBeforeNetwork 输入类命令：本地文件位于敏感目录时拒绝读取（防止把凭证发往远端），
// 且发生在第一次网络请求之前。
func TestInputPathsRejectedBeforeNetwork(t *testing.T) {
	type tc struct {
		name  string
		cmd   *cobra.Command
		args  func(secret string) []string
		flags func(secret string) map[string]string
	}
	none := func(string) map[string]string { return nil }
	cases := []tc{
		{"doc import", importMarkdownCmd, func(s string) []string { return []string{s} }, none},
		{"doc add --content-type markdown", addContentCmd, func(s string) []string {
			return []string{"https://example.feishu.cn/wiki/wikcnfptestfake", s}
		}, func(string) map[string]string { return map[string]string{"content-type": "markdown"} }},
		{"doc update --content-file", updateBlockCmd, func(string) []string {
			return []string{"https://example.feishu.cn/wiki/wikcnfptestfake", "blkfp"}
		}, func(s string) map[string]string { return map[string]string{"content-file": s} }},
		{"doc batch-update", batchUpdateBlocksCmd, func(s string) []string {
			return []string{"https://example.feishu.cn/wiki/wikcnfptestfake", s}
		}, none},
		{"doc htmlbox create --html-file", docHtmlboxCreateCmd, func(string) []string { return []string{"doxcnfptestfake"} },
			func(s string) map[string]string { return map[string]string{"html-file": s} }},
		{"doc media-insert --file", docMediaInsertCmd, func(string) []string { return []string{"doxcnfptestfake"} },
			func(s string) map[string]string { return map[string]string{"file": s, "type": "file"} }},
		{"markdown create --content-file", markdownCreateCmd, func(string) []string { return nil },
			func(s string) map[string]string { return map[string]string{"content-file": s} }},
		{"markdown overwrite --content-file", markdownOverwriteCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"file-token": "boxcnfptestfake", "content-file": s}
			}},
		{"markdown diff --file", markdownDiffCmd, func(string) []string { return nil },
			func(s string) map[string]string { return map[string]string{"file-token": "boxcnfptestfake", "file": s} }},
		{"markdown diff --file --dry-run", markdownDiffCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"file-token": "boxcnfptestfake", "file": s, "dry-run": "true"}
			}},
		{"board update <nodes_file>", boardUpdateCmd, func(s string) []string { return []string{"boxcnfptestfake", s} }, none},
		{"board import <file>", importDiagramCmd, func(s string) []string { return []string{"boxcnfptestfake", s} }, none},
		{"board import --dry-run <file>", importDiagramCmd, func(s string) []string { return []string{"boxcnfptestfake", s} },
			func(string) map[string]string { return map[string]string{"dry-run": "true"} }},
		{"board create-notes <file>", createBoardNotesCmd, func(s string) []string { return []string{"boxcnfptestfake", s} }, none},
		{"board svg-import <file>", boardSVGImportCmd, func(s string) []string { return []string{"boxcnfptestfake", s} }, none},
		{"board upload-image <img>", boardUploadImageCmd, func(s string) []string { return []string{"boxcnfptestfake", s} }, none},
		{"drive upload --file", driveUploadCmd, func(string) []string { return nil },
			func(s string) map[string]string { return map[string]string{"file": s} }},
		{"drive import --file", driveImportCmd, func(string) []string { return nil },
			func(s string) map[string]string { return map[string]string{"file": s, "type": "docx"} }},
		{"file upload", uploadFileCmd, func(s string) []string { return []string{s} }, none},
		{"media upload", uploadMediaCmd, func(s string) []string { return []string{s} },
			func(string) map[string]string { return map[string]string{"parent-node": "doxcnfptestfake"} }},
		{"doc import-file", importFileCmd, func(s string) []string { return []string{s} },
			func(string) map[string]string { return map[string]string{"type": "docx"} }},
		{"task upload-attachment --file", taskUploadAttachmentCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"task-guid": "8d3c8a8f-0000-4000-8000-000000000000", "file": s}
			}},
		{"slides media-upload --file", slidesMediaUploadCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"file": s, "presentation-token": "https://example.feishu.cn/wiki/wikcnfptestfake"}
			}},
		{"okr upload-image --file", okrUploadImageCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"file": s, "objective-id": "7000000000000000001"}
			}},
		{"bitable record upload-attachment --file", bitableRecordUploadAttachmentCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"base-token": "https://example.feishu.cn/wiki/wikcnfptestfake", "table-id": "tblfp", "record-id": "recfp", "field-id": "fldfp", "file": s}
			}},
		{"sheet import-md", sheetImportMDCmd, func(s string) []string { return []string{s} }, none},
		{"apps html-publish --path", appsHTMLPublishCmd, func(string) []string { return nil },
			func(s string) map[string]string { return map[string]string{"app-id": "app_fptestfake", "path": s} }},
		{"apps html-publish --path 家目录（含 ~/.ssh）", appsHTMLPublishCmd, func(string) []string { return nil },
			func(s string) map[string]string {
				return map[string]string{"app-id": "app_fptestfake", "path": filepath.Dir(filepath.Dir(s)), "dry-run": "true", "allow-sensitive": "true"}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newPathGuardEnv(t)
			flags := c.flags(env.secret)
			if flags == nil {
				flags = map[string]string{}
			}
			err := runGuardedCmd(t, c.cmd, c.args(env.secret), flags)
			env.assertRejectedBeforeNetwork(t, err, "拒绝读取")
		})
	}
}

// TestLocalInputFileUsageErrors 输入文件不存在 / 是目录时为用法错误（退出码 2，带路径），且不发请求。
func TestLocalInputFileUsageErrors(t *testing.T) {
	type tc struct {
		name  string
		cmd   *cobra.Command
		args  func(p string) []string
		flags func(p string) map[string]string
	}
	none := func(string) map[string]string { return nil }
	cases := []tc{
		{"doc import", importMarkdownCmd, func(p string) []string { return []string{p} }, none},
		{"doc add", addContentCmd, func(p string) []string { return []string{"doxcnfptestfake", p} },
			func(string) map[string]string { return map[string]string{"content-type": "markdown"} }},
		{"markdown create", markdownCreateCmd, func(string) []string { return nil },
			func(p string) map[string]string { return map[string]string{"content-file": p} }},
		{"markdown overwrite", markdownOverwriteCmd, func(string) []string { return nil },
			func(p string) map[string]string {
				return map[string]string{"file-token": "boxcnfptestfake", "content-file": p}
			}},
		{"markdown diff", markdownDiffCmd, func(string) []string { return nil },
			func(p string) map[string]string { return map[string]string{"file-token": "boxcnfptestfake", "file": p} }},
		{"drive upload", driveUploadCmd, func(string) []string { return nil },
			func(p string) map[string]string { return map[string]string{"file": p} }},
		{"file upload", uploadFileCmd, func(p string) []string { return []string{p} }, none},
	}
	for _, c := range cases {
		for _, kind := range []string{"不存在", "是目录"} {
			t.Run(c.name+"/"+kind, func(t *testing.T) {
				env := newPathGuardEnv(t)
				dir := t.TempDir()
				p := filepath.Join(dir, "missing.md")
				want := "不存在"
				if kind == "是目录" {
					p = dir
					want = "是目录"
				}
				flags := c.flags(p)
				if flags == nil {
					flags = map[string]string{}
				}
				err := runGuardedCmd(t, c.cmd, c.args(p), flags)
				env.assertRejectedBeforeNetwork(t, err, want)
				if !strings.Contains(err.Error(), p) {
					t.Fatalf("错误信息应带路径 %q: %v", p, err)
				}
			})
		}
	}
}

// TestResolveMediaSourceRejectsSensitiveLocalImage Markdown 引用的本地图片位于敏感目录时拒绝（计入该图片 failures），
// 相对 Markdown 目录的 ../ 引用照常放行。
func TestResolveMediaSourceRejectsSensitiveLocalImage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(ssh, "id_test.png")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveMediaSource(secret, t.TempDir(), ".png"); err == nil || !strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("绝对路径指向 ~/.ssh 应拒绝: %v", err)
	}
	if _, _, _, err := resolveMediaSource("../.ssh/id_test.png", filepath.Join(home, "docs"), ".png"); err == nil || !strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("相对 Markdown 目录跳到 ~/.ssh 应拒绝: %v", err)
	}

	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(work, "images", "a.png")
	if err := os.MkdirAll(filepath.Dir(img), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, _, err := resolveMediaSource("../images/a.png", filepath.Join(work, "docs"), ".png")
	if err != nil || filepath.Clean(got) != img {
		t.Fatalf("../images/a.png 应放行: got=%q err=%v", got, err)
	}
}
