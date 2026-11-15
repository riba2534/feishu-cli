package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/viper"
)

// storageDryRunFixture 写一个小文件供上传类命令的 --dry-run 使用。
func storageDryRunFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type storageDryRunCase struct {
	name string
	args []string
	// want 是 dry-run 输出中必须出现的片段（method/path/body 关键字段）
	want []string
}

func storageDryRunCases(file string) []storageDryRunCase {
	return []storageDryRunCase{
		{"comment add", []string{"comment", "add", "doxcnA", "--type", "docx", "--text", "hi"},
			[]string{`"POST"`, "/open-apis/drive/v1/files/doxcnA/comments", `"text": "hi"`}},
		{"comment resolve", []string{"comment", "resolve", "doxcnA", "c1", "--type", "docx"},
			[]string{`"PATCH"`, "/comments/c1", `"is_solved": true`}},
		{"comment unresolve", []string{"comment", "unresolve", "doxcnA", "c1"},
			[]string{`"PATCH"`, `"is_solved": false`}},
		{"comment reply add", []string{"comment", "reply", "add", "doxcnA", "c1", "--text", "ok"},
			[]string{`"POST"`, "/comments/c1/replies", `"text": "ok"`}},
		{"comment reply delete", []string{"comment", "reply", "delete", "doxcnA", "c1", "r1"},
			[]string{`"DELETE"`, "/comments/c1/replies/r1"}},
		{"drive add-comment", []string{"drive", "add-comment", "--doc", "https://example.feishu.cn/sheets/shtcnA", "--block-id", "a1b2c3!D6", "--content", `[{"type":"text","text":"x"}]`},
			[]string{"/open-apis/drive/v1/files/shtcnA/new_comments", `"sheet_col": 3`, `"sheet_row": 5`, `"file_type": "sheet"`}},
		{"drive upload", []string{"drive", "upload", "--file", file, "--folder-token", "fldcnA"},
			[]string{"/open-apis/drive/v1/files/upload_all", `"parent_node": "fldcnA"`, `"size": 5`}},
		{"drive upload overwrite", []string{"drive", "upload", "--file", file, "--file-token", "boxcnA"},
			[]string{"/open-apis/drive/v1/files/upload_all", `"file_token": "boxcnA"`, `"overwrite": true`}},
		{"drive secure-label set", []string{"drive", "secure-label", "set", "doxcnA", "--label-id", "7217"},
			[]string{`"PATCH"`, "/open-apis/drive/v2/files/doxcnA/secure_label", `"id": "7217"`}},
		{"file copy", []string{"file", "copy", "doxcnA", "--target", "fldcnA", "--type", "docx", "--name", "n"},
			[]string{"/open-apis/drive/v1/files/doxcnA/copy", `"folder_token": "fldcnA"`, `"name": "n"`}},
		{"file mkdir", []string{"file", "mkdir", "dir1", "--parent", "fldcnA"},
			[]string{"/open-apis/drive/v1/files/create_folder", `"name": "dir1"`}},
		{"file move", []string{"file", "move", "doxcnA", "--target", "fldcnA", "--type", "docx"},
			[]string{"/open-apis/drive/v1/files/doxcnA/move", `"type": "docx"`}},
		{"file shortcut", []string{"file", "shortcut", "doxcnA", "--target", "fldcnA", "--type", "docx"},
			[]string{"/open-apis/drive/v1/files/create_shortcut", `"refer_token": "doxcnA"`}},
		{"file upload", []string{"file", "upload", file, "--parent", "fldcnA"},
			[]string{"/open-apis/drive/v1/files/upload_all", `"file_name": "report.txt"`}},
		{"file version create", []string{"file", "version", "create", "doxcnA", "--name", "v1"},
			[]string{`"POST"`, "/open-apis/drive/v1/files/doxcnA/versions", `"obj_type": "docx"`}},
		{"file version delete", []string{"file", "version", "delete", "doxcnA", "ver1"},
			[]string{`"DELETE"`, "/open-apis/drive/v1/files/doxcnA/versions/ver1"}},
		{"file version revert", []string{"file", "version", "revert", "doxcnA", "7633"},
			[]string{"/open-apis/drive/v1/files/doxcnA/revert", `"version": "7633"`}},
		{"media upload", []string{"media", "upload", file, "--parent-node", "doxcnA"},
			[]string{"/open-apis/drive/v1/medias/upload_all", `"parent_type": "docx_image"`}},
		{"media upload doc-id url", []string{"media", "upload", file, "--parent-node", "blk1", "--doc-id", "https://example.feishu.cn/docx/doxcnB"},
			[]string{"drive_route_token", "doxcnB"}},
		{"minutes apply-permission", []string{"minutes", "apply-permission", "--minute-token", "obcnA", "--perm", "view"},
			[]string{"/open-apis/minutes/v1/minutes/obcnA/permissions/apply", `"perm": "view"`}},
	}
}

// TestDriveFileCommentMediaMinutes_DryRunWithoutCredentials 写命令的 --dry-run 在无凭据 HOME 下 exit 0、
// 输出请求预览；去掉 --dry-run 后仍因缺凭据 exit 3（静态预览不读凭据、不联网）。
func TestDriveFileCommentMediaMinutes_DryRunWithoutCredentials(t *testing.T) {
	withCmdHome(t)
	viper.Reset() // 清掉其他用例 stub 服务器留下的 app_id/base_url，模拟真正无配置的环境
	t.Cleanup(viper.Reset)
	file := storageDryRunFixture(t)
	for _, tc := range storageDryRunCases(file) {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, err := runCLI(t, append(append([]string{}, tc.args...), "--dry-run")...)
			if err != nil {
				t.Fatalf("--dry-run 无凭据应成功: %v\nstderr: %s", err, stderr)
			}
			var plan map[string]any
			if jerr := json.Unmarshal([]byte(out), &plan); jerr != nil || plan["dry_run"] != true {
				t.Fatalf("dry-run 输出应是 dry_run=true 的 JSON: %v\n%s", jerr, out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Fatalf("dry-run 输出缺少 %q:\n%s", w, out)
				}
			}
			_, _, err = runCLI(t, tc.args...)
			if code := exitCodeFor(err); code != clierr.ExitAuth {
				t.Fatalf("去掉 --dry-run 无凭据应 exit 3，got %d: %v", code, err)
			}
		})
	}
}

// TestDriveFileCommentMediaMinutes_UsageErrorsBeforeCredentials 参数错误在凭据检查之前报出（exit 2），dry-run 同样拦截。
func TestDriveFileCommentMediaMinutes_UsageErrorsBeforeCredentials(t *testing.T) {
	file := storageDryRunFixture(t)
	cases := []struct {
		name string
		args []string
	}{
		{"comment reply add empty", []string{"comment", "reply", "add", "doxcnA", "c1", "--text", ""}},
		{"drive add-comment bad json", []string{"drive", "add-comment", "--doc", "doxcnA", "--content", "not-json"}},
		{"drive add-comment sheet no anchor", []string{"drive", "add-comment", "--doc", "https://example.feishu.cn/sheets/shtcnA", "--content", `[{"type":"text","text":"x"}]`}},
		{"drive add-comment unknown url", []string{"drive", "add-comment", "--doc", "https://example.feishu.cn/unknown/x", "--content", `[{"type":"text","text":"x"}]`}},
		{"drive upload exclusive", []string{"drive", "upload", "--file", file, "--file-token", "boxcnA", "--folder-token", "fldcnA"}},
		{"drive upload missing file", []string{"drive", "upload", "--file", filepath.Join(t.TempDir(), "nope.bin")}},
		{"drive secure-label set no label", []string{"drive", "secure-label", "set", "doxcnA"}},
		{"media upload no parent", []string{"media", "upload", file}},
		{"media upload bad doc-id", []string{"media", "upload", file, "--parent-node", "blk1", "--doc-id", "https://example.feishu.cn/sheets/shtcnA"}},
		{"minutes apply-permission bad perm", []string{"minutes", "apply-permission", "--minute-token", "obcnA", "--perm", "admin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, extra := range [][]string{nil, {"--dry-run"}} {
				_, _, err := runCLI(t, append(append([]string{}, tc.args...), extra...)...)
				if code := exitCodeFor(err); code != clierr.ExitUsage {
					t.Fatalf("%v 应 exit 2，got %d: %v", extra, code, err)
				}
			}
		})
	}
}

// TestDriveAddComment_WikiDryRunNeedsCredentials wiki 输入的 dry-run 属于计划预览（需只读解析节点）：无凭据时 exit 3。
func TestDriveAddComment_WikiDryRunNeedsCredentials(t *testing.T) {
	_, _, err := runCLI(t, "drive", "add-comment", "--doc", "https://example.feishu.cn/wiki/wikcnA",
		"--content", `[{"type":"text","text":"x"}]`, "--dry-run")
	if code := exitCodeFor(err); code != clierr.ExitAuth {
		t.Fatalf("wiki 输入的 dry-run 无凭据应 exit 3，got %d: %v", code, err)
	}
}

// TestDriveFileUploadSteps_Multipart 大于 20MB 的文件 dry-run 列出 upload_prepare/part/finish（与真实分支一致）。
func TestDriveFileUploadSteps_Multipart(t *testing.T) {
	steps := driveFileUploadSteps("/tmp/big.bin", "big.bin", "fldcnA", "boxcnA", 21*1024*1024)
	if len(steps) != 3 || !strings.HasSuffix(steps[0].URL, "/files/upload_prepare") ||
		!strings.HasSuffix(steps[1].URL, "/files/upload_part") || !strings.HasSuffix(steps[2].URL, "/files/upload_finish") {
		t.Fatalf("分块上传步骤不符: %+v", steps)
	}
	if steps[0].Body["file_token"] != "boxcnA" {
		t.Fatalf("覆盖上传的 upload_prepare 应携带 file_token: %+v", steps[0].Body)
	}
	small := driveFileUploadSteps("/tmp/s.bin", "s.bin", "", "", 1024)
	if len(small) != 1 || !strings.HasSuffix(small[0].URL, "/files/upload_all") {
		t.Fatalf("小文件应单次 upload_all: %+v", small)
	}
	if _, ok := small[0].Body["file_token"]; ok {
		t.Fatalf("非覆盖上传不应带 file_token: %+v", small[0].Body)
	}
}
