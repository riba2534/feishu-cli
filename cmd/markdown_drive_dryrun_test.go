package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func resetCmdFlag(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if f := cmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}

func captureCmdStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), runErr
}

func TestMarkdownFetchDryRunPreviewDownload(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	cmd := markdownFetchCmd
	_ = cmd.Flags().Set("file-token", "boxcnMarkdownDryRun")
	_ = cmd.Flags().Set("version", "7633658129540910621")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "file-token", "version", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run fetch: %v", err)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/medias/boxcnMarkdownDryRun/preview_download") {
		t.Fatalf("missing preview_download: %s", out)
	}
	if !strings.Contains(out, `"preview_type": "16"`) && !strings.Contains(out, `"preview_type":"16"`) {
		t.Fatalf("missing preview_type=16: %s", out)
	}
	if !strings.Contains(out, "7633658129540910621") {
		t.Fatalf("missing version: %s", out)
	}
}

func TestMarkdownCreateDryRunWikiTarget(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	cmd := markdownCreateCmd
	_ = cmd.Flags().Set("name", "README.md")
	_ = cmd.Flags().Set("content", "# hello")
	_ = cmd.Flags().Set("wiki-token", "wikcnMarkdownDryRun")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "name", "content", "wiki-token", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run create: %v\n%s", err, out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/files/upload_all") {
		t.Fatalf("missing upload_all: %s", out)
	}
	if !strings.Contains(out, `"parent_type": "wiki"`) && !strings.Contains(out, `"parent_type":"wiki"`) {
		t.Fatalf("missing parent_type wiki: %s", out)
	}
}

func TestDriveExportDryRunDocsAI(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	cmd := driveExportCmd
	_ = cmd.Flags().Set("token", "docxMdDryRun")
	_ = cmd.Flags().Set("doc-type", "docx")
	_ = cmd.Flags().Set("file-extension", "markdown")
	_ = cmd.Flags().Set("file-name", "my-notes")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "token", "doc-type", "file-extension", "file-name", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run export: %v\n%s", err, out)
	}
	if !strings.Contains(out, "/open-apis/docs_ai/v1/documents/docxMdDryRun/fetch") {
		t.Fatalf("missing docs_ai fetch: %s", out)
	}
	if strings.Contains(out, "extra_param") {
		t.Fatalf("must not enable extra_param: %s", out)
	}
	if !strings.Contains(out, "my-notes.md") {
		t.Fatalf("missing file_name metadata: %s", out)
	}
}

func TestDriveImportDryRunPointAndUploadAll(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# dry run\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := driveImportCmd
	_ = cmd.Flags().Set("file", path)
	_ = cmd.Flags().Set("type", "docx")
	_ = cmd.Flags().Set("folder-token", "fldcnImportDryRunTarget")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "file", "type", "folder-token", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "/open-apis/wiki/v2/spaces/node_by_token") {
		t.Fatalf("missing wiki probe: %s", out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_all") {
		t.Fatalf("missing upload_all: %s", out)
	}
	if strings.Contains(out, `"parent_node": "ccm_import_open"`) {
		t.Fatalf("upload_all must not send parent_node=ccm_import_open: %s", out)
	}
	if !strings.Contains(out, `"mount_type": 1`) && !strings.Contains(out, `"mount_type":1`) {
		t.Fatalf("missing point.mount_type: %s", out)
	}
}

func TestDriveImportDryRunMultipartBoundary(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "large.xlsx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(20*1024*1024 + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	cmd := driveImportCmd
	_ = cmd.Flags().Set("file", path)
	_ = cmd.Flags().Set("type", "sheet")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "file", "type", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run large import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/medias/upload_prepare") {
		t.Fatalf("missing upload_prepare: %s", out)
	}
	if !strings.Contains(out, `"parent_node": ""`) && !strings.Contains(out, `"parent_node":""`) {
		t.Fatalf("prepare must send empty parent_node: %s", out)
	}
}

func TestDriveMoveDryRunRootToken(t *testing.T) {
	cleanup := setupCmdTestConfig(t, "http://127.0.0.1:1")
	defer cleanup()

	cmd := driveMoveCmd
	_ = cmd.Flags().Set("file-token", "boxcnMove")
	_ = cmd.Flags().Set("type", "docx")
	_ = cmd.Flags().Set("dry-run", "true")
	defer resetCmdFlag(cmd, "file-token", "type", "dry-run")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("dry-run move: %v\n%s", err, out)
	}
	if !strings.Contains(out, "/open-apis/drive/explorer/v2/root_folder/meta") {
		t.Fatalf("missing root_folder/meta: %s", out)
	}
	var plan struct {
		API []struct {
			URL string `json:"url"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(plan.API) < 2 {
		t.Fatalf("expected root + move, got %#v", plan.API)
	}
}
