package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func isolateMarkdownDriveHome(t *testing.T) (string, func()) {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	restoreHome := profile.SetHomeFunc(func() (string, error) { return tmpHome, nil })
	return tmpHome, restoreHome
}

func writeExpiredUserToken(t *testing.T, homeDir string) {
	t.Helper()
	tokenDir := filepath.Join(homeDir, ".feishu-cli")
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		t.Fatalf("mkdir token dir: %v", err)
	}
	store := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     "broken-refresh-token",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-1 * time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "drive:drive",
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tokenDir, "token.json"), data, 0o600); err != nil {
		t.Fatalf("write token.json: %v", err)
	}
}

func setCmdAs(t *testing.T, cmd *cobra.Command, as string) {
	t.Helper()
	found := false
	seen := map[*pflag.Flag]struct{}{}
	for _, f := range []*pflag.Flag{
		cmd.Flags().Lookup("as"),
		cmd.InheritedFlags().Lookup("as"),
		cmd.PersistentFlags().Lookup("as"),
		markdownCmd.PersistentFlags().Lookup("as"),
	} {
		if f == nil {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		if err := f.Value.Set(as); err != nil {
			t.Fatalf("set --as %s: %v", as, err)
		}
		f.Changed = as != "" && as != "auto"
		found = true
	}
	if !found {
		t.Fatalf("%s 缺少 --as flag", cmd.Name())
	}
}

func identityPreviewHandler(t *testing.T, bizHits *int32, capturedAuth *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tenant_access_token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot-token","expire":7200}`)
		case r.URL.Path == "/open-apis/authen/v2/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token is invalid"}`)
		case strings.Contains(r.URL.Path, "/preview_download"):
			atomic.AddInt32(bizHits, 1)
			if capturedAuth != nil {
				*capturedAuth = r.Header.Get("Authorization")
			}
			w.Header().Set("Content-Type", "text/markdown")
			w.Header().Set("Content-Disposition", `attachment; filename="notes.md"`)
			_, _ = fmt.Fprint(w, "# identity")
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func TestMarkdownDriveAsFlagsRegistered(t *testing.T) {
	if markdownCmd.PersistentFlags().Lookup("as") == nil {
		t.Fatal("markdown 命令组缺少 persistent --as")
	}
	if markdownCmd.PersistentFlags().Lookup("as").DefValue != "auto" {
		t.Fatalf("markdown --as 默认值 = %q, want auto", markdownCmd.PersistentFlags().Lookup("as").DefValue)
	}
	for _, cmd := range []*cobra.Command{
		markdownFetchCmd, markdownCreateCmd, markdownOverwriteCmd, markdownPatchCmd, markdownDiffCmd,
		driveImportCmd, driveExportCmd, driveExportDownloadCmd, driveMoveCmd,
	} {
		if cmd.Flags().Lookup("as") == nil && cmd.InheritedFlags().Lookup("as") == nil {
			t.Errorf("%s 缺少 --as flag", cmd.Name())
		}
	}
}

func TestMarkdownFetch_AsBotUsesTenantToken(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	_ = home

	var bizHits int32
	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, identityPreviewHandler(t, &bizHits, &capturedAuth))
	defer cleanup()

	cmd := markdownFetchCmd
	_ = cmd.Flags().Set("file-token", "boxcnIdentityBot")
	setCmdAs(t, cmd, "bot")
	defer resetCmdFlag(cmd, "file-token", "as")
	defer setCmdAs(t, cmd, "auto")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("markdown fetch --as bot: %v\n%s", err, out)
	}
	if !strings.Contains(out, "# identity") {
		t.Fatalf("stdout = %q", out)
	}
	if hits := atomic.LoadInt32(&bizHits); hits != 1 {
		t.Fatalf("preview_download hits = %d", hits)
	}
	if !strings.HasPrefix(capturedAuth, "Bearer t-") {
		t.Fatalf("Authorization = %q, want Bearer t-...", capturedAuth)
	}
}

func TestMarkdownFetch_AsUserRequiresToken(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	_ = home

	var bizHits int32
	cleanup := stubCmdFeishuServer(t, identityPreviewHandler(t, &bizHits, nil))
	defer cleanup()

	cmd := markdownFetchCmd
	_ = cmd.Flags().Set("file-token", "boxcnIdentityUser")
	setCmdAs(t, cmd, "user")
	defer resetCmdFlag(cmd, "file-token", "as")
	defer setCmdAs(t, cmd, "auto")
	if got, _ := cmd.Flags().GetString("as"); strings.ToLower(strings.TrimSpace(got)) != "user" {
		t.Fatalf("cmd --as = %q, want user", got)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("--as user 未配置 User Token 应报错")
	}
	if !strings.Contains(err.Error(), "User Access Token") {
		t.Fatalf("error = %v", err)
	}
	if hits := atomic.LoadInt32(&bizHits); hits != 0 {
		t.Fatalf("未配置 User Token 时不应请求 preview_download, hits=%d", hits)
	}
}

func TestMarkdownFetch_AsAutoFallsBackToBotWhenNoUserToken(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	_ = home

	var bizHits int32
	var capturedAuth string
	cleanup := stubCmdFeishuServer(t, identityPreviewHandler(t, &bizHits, &capturedAuth))
	defer cleanup()

	cmd := markdownFetchCmd
	_ = cmd.Flags().Set("file-token", "boxcnIdentityAuto")
	setCmdAs(t, cmd, "auto")
	defer resetCmdFlag(cmd, "file-token", "as")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("markdown fetch --as auto: %v\n%s", err, out)
	}
	if hits := atomic.LoadInt32(&bizHits); hits != 1 {
		t.Fatalf("preview_download hits = %d", hits)
	}
	if !strings.HasPrefix(capturedAuth, "Bearer t-") {
		t.Fatalf("Authorization = %q, want Bot/Tenant token", capturedAuth)
	}
}

func TestMarkdownFetch_AsAutoFailClosedOnRefreshError(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	writeExpiredUserToken(t, home)

	var bizHits int32
	var tenantHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tenant_access_token") {
			atomic.AddInt32(&tenantHits, 1)
			t.Errorf("User refresh 失败时不应请求 tenant access token")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"tenant_access_token":"t-should-not-use","expire":7200}`)
			return
		}
		identityPreviewHandler(t, &bizHits, nil)(w, r)
	})
	defer cleanup()

	cmd := markdownFetchCmd
	_ = cmd.Flags().Set("file-token", "boxcnIdentityFailClosed")
	setCmdAs(t, cmd, "auto")
	defer resetCmdFlag(cmd, "file-token", "as")

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("--as auto 在 User Token 已配置但刷新失败时应 fail-closed")
	}
	if hits := atomic.LoadInt32(&bizHits); hits != 0 {
		t.Fatalf("fail-closed 后仍请求了 preview_download %d 次", hits)
	}
	if hits := atomic.LoadInt32(&tenantHits); hits != 0 {
		t.Fatalf("fail-closed 后仍请求了 tenant token %d 次", hits)
	}
}

func TestMarkdownDriveDryRunDoesNotResolveToken(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	_ = home

	var bizHits int32
	cleanup := stubCmdFeishuServer(t, identityPreviewHandler(t, &bizHits, nil))
	defer cleanup()

	cases := []struct {
		name  string
		cmd   *cobra.Command
		set   func(*cobra.Command)
		reset []string
	}{
		{
			name: "markdown fetch",
			cmd:  markdownFetchCmd,
			set: func(c *cobra.Command) {
				_ = c.Flags().Set("file-token", "boxcnDryRunAs")
				_ = c.Flags().Set("dry-run", "true")
				setCmdAs(t, c, "user")
			},
			reset: []string{"file-token", "dry-run", "as"},
		},
		{
			name: "drive export",
			cmd:  driveExportCmd,
			set: func(c *cobra.Command) {
				_ = c.Flags().Set("token", "doxcnDryRunAs")
				_ = c.Flags().Set("doc-type", "docx")
				_ = c.Flags().Set("file-extension", "markdown")
				_ = c.Flags().Set("dry-run", "true")
				setCmdAs(t, c, "user")
			},
			reset: []string{"token", "doc-type", "file-extension", "dry-run", "as"},
		},
		{
			name: "drive move",
			cmd:  driveMoveCmd,
			set: func(c *cobra.Command) {
				_ = c.Flags().Set("file-token", "boxcnMoveAs")
				_ = c.Flags().Set("type", "docx")
				_ = c.Flags().Set("dry-run", "true")
				setCmdAs(t, c, "user")
			},
			reset: []string{"file-token", "type", "dry-run", "as"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.set(tc.cmd)
			defer resetCmdFlag(tc.cmd, tc.reset...)
			defer setCmdAs(t, tc.cmd, "auto")
			out, err := captureCmdStdout(t, func() error { return tc.cmd.RunE(tc.cmd, nil) })
			if err != nil {
				t.Fatalf("dry-run --as user 不应解析 token: %v\n%s", err, out)
			}
		})
	}
	if hits := atomic.LoadInt32(&bizHits); hits != 0 {
		t.Fatalf("dry-run 不应打业务端点, hits=%d", hits)
	}
}

func TestDriveImportDryRunAsUserWithoutToken(t *testing.T) {
	home, restoreHome := isolateMarkdownDriveHome(t)
	defer restoreHome()
	_ = home

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run 不应发请求: %s %s", r.Method, r.URL.Path)
		http.Error(w, "no request", http.StatusInternalServerError)
	})
	defer cleanup()

	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# import identity\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := driveImportCmd
	_ = cmd.Flags().Set("file", path)
	_ = cmd.Flags().Set("type", "docx")
	_ = cmd.Flags().Set("dry-run", "true")
	setCmdAs(t, cmd, "user")
	defer resetCmdFlag(cmd, "file", "type", "dry-run", "as")
	defer setCmdAs(t, cmd, "auto")

	out, err := captureCmdStdout(t, func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatalf("drive import dry-run --as user: %v\n%s", err, out)
	}
}
