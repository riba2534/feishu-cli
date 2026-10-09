package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initIsolatedStorageTestConfig 写一份指向 httptest 的配置，并隔离 HOME/profile，
// 保证不会读到开发者真实的 token.json。
func initIsolatedStorageTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	configPath := filepath.Join(home, "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// TestCommentAddHelpMatchesIdentity comment add 的 help 与实际身份解析一致：User 优先，未配置回退 App Token。
func TestCommentAddHelpMatchesIdentity(t *testing.T) {
	if strings.Contains(addCommentCmd.Long, "默认以 App Token") || !strings.Contains(addCommentCmd.Long, "User 优先") {
		t.Fatalf("comment add --help 身份说明与代码不一致:\n%s", addCommentCmd.Long)
	}

	for _, tc := range []struct {
		name, envToken, wantAuth string
	}{
		{"配置了 User Token 时以用户身份创建", "u-env-token", "Bearer u-env-token"},
		{"未配置 User Token 时回退 App Token", "", "Bearer t-test-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test-token","expire":7200}`)
				case r.Method == http.MethodPost && r.URL.Path == "/open-apis/drive/v1/files/doxcnFake/comments":
					gotAuth = r.Header.Get("Authorization")
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"comment_id":"cmt_fake"}}`)
				default:
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
				}
			}))
			defer srv.Close()
			initIsolatedStorageTestConfig(t, srv.URL)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", tc.envToken)

			cmd := &cobra.Command{}
			cmd.Flags().String("type", "docx", "")
			cmd.Flags().String("text", "fp-test", "")
			cmd.Flags().String("output", "json", "")
			cmd.Flags().String("user-access-token", "", "")
			if err := addCommentCmd.RunE(cmd, []string{"doxcnFake"}); err != nil {
				t.Fatalf("comment add 失败: %v", err)
			}
			if gotAuth != tc.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
		})
	}
}

// TestFileVersionRevertHelpPointsToVersionHistory 回滚用的 version 来自 drive version-history，
// 不是 file version list（后者是文档命名版本）。
func TestFileVersionRevertHelpPointsToVersionHistory(t *testing.T) {
	if !strings.Contains(revertVersionCmd.Long, "drive version-history --file-token <file_token>") {
		t.Fatalf("file version revert --help 应指向 drive version-history:\n%s", revertVersionCmd.Long)
	}
	if strings.Contains(revertVersionCmd.Long, "file version list 返回") || strings.Contains(revertVersionCmd.Long, "从 feishu-cli file version list") {
		t.Fatalf("file version revert --help 不应再说版本号来自 file version list:\n%s", revertVersionCmd.Long)
	}
	// 被指向的命令与参数必须真实存在
	c, _, err := rootCmd.Find([]string{"drive", "version-history"})
	if err != nil || c.Name() != "version-history" || c.Flags().Lookup("file-token") == nil {
		t.Fatalf("drive version-history --file-token 不存在: %v", err)
	}
}

// TestDeleteTaskFailedErrorHintsUseRealFlags 异步删除失败的提示命令必须能直接执行：
// drive inspect 只接受 --url，不接受位置参数。
func TestDeleteTaskFailedErrorHintsUseRealFlags(t *testing.T) {
	base := errors.New("删除任务失败: status=fail")
	userHint := deleteTaskFailedError(base, "user").Error()
	if !strings.Contains(userHint, "feishu-cli drive inspect --url <token>") || strings.Contains(userHint, "drive inspect <token>") {
		t.Fatalf("user 身份提示应为 drive inspect --url <token>: %s", userHint)
	}
	if !errors.Is(deleteTaskFailedError(base, "user"), base) {
		t.Fatal("提示应包裹原错误")
	}
	inspect, _, err := rootCmd.Find([]string{"drive", "inspect"})
	if err != nil || inspect.Flags().Lookup("url") == nil {
		t.Fatalf("drive inspect --url 不存在: %v", err)
	}

	botHint := deleteTaskFailedError(base, "bot").Error()
	if !strings.Contains(botHint, "feishu-cli perm list <token> --doc-type <type> --as user") {
		t.Fatalf("bot 身份提示不对: %s", botHint)
	}
	perm, _, err := rootCmd.Find([]string{"perm", "list"})
	if err != nil || perm.Flags().Lookup("doc-type") == nil || perm.InheritedFlags().Lookup("as") == nil {
		t.Fatalf("perm list --doc-type/--as 不存在: %v", err)
	}
}

// TestPasswordCreate1063002HintsPublicLink 分享密码遇 1063002 时提示先把链接设为互联网公开，
// 而不是"改用 --as user"；提示中的命令与参数必须真实存在，原错误码保留。
func TestPasswordCreate1063002HintsPublicLink(t *testing.T) {
	_, reqs := newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":1063002,"msg":"Permission denied"}`)
	})
	for _, c := range []*cobra.Command{passwordCreateCmd, passwordUpdateCmd} {
		t.Run(c.Name(), func(t *testing.T) {
			_, _, err := runCmdWithFlags(t, c, []string{"doxcnFakePwd"}, "--doc-type", "sheet")
			if err == nil {
				t.Fatal("1063002 应返回错误")
			}
			msg := err.Error()
			if !strings.Contains(msg, "互联网公开") ||
				!strings.Contains(msg, "feishu-cli perm public-update doxcnFakePwd --doc-type sheet --external-access=true --link-share-entity anyone_readable") {
				t.Fatalf("提示应指向把链接设为互联网公开: %s", msg)
			}
			if strings.Contains(msg, "--as user") {
				t.Fatalf("分享密码 1063002 不应再提示改用 --as user: %s", msg)
			}
			if !client.HasAPICode(err, 1063002) {
				t.Fatalf("应保留原错误码: %v", err)
			}
		})
	}
	if got := reqs(); len(got) != 2 || got[0].Path != "/open-apis/drive/v1/permissions/doxcnFakePwd/public/password" || got[0].Query != "type=sheet" {
		t.Fatalf("请求不对: %+v", got)
	}
	pu, _, err := rootCmd.Find([]string{"perm", "public-update"})
	if err != nil || pu.Flags().Lookup("external-access") == nil || pu.Flags().Lookup("link-share-entity") == nil || pu.Flags().Lookup("doc-type") == nil {
		t.Fatalf("perm public-update 参数不存在: %v", err)
	}
	// 其他错误码仍走通用权限提示
	other := fmt.Errorf("x: code=1063004, msg=no share permission")
	if err := wrapPasswordError(other, "", "doxcn", "docx"); !strings.Contains(err.Error(), "--as user") {
		t.Fatalf("1063004 应沿用通用提示: %v", err)
	}
}

// TestDriveExportDownloadRetryCarriesIdentity 导出文件下载失败时给出的重试命令带上本次实际使用的身份，
// 文件名与目录按 shell 转义，且命令参数在 export-download 上真实存在。
func TestDriveExportDownloadRetryCarriesIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flags    []string
		wantAs   string
		wantAuth string
	}{
		{"显式 --as bot", []string{"--as", "bot"}, "--as bot", "Bearer t-test-token"},
		{"auto 解析为 User", []string{"--as", "auto", "--user-access-token", "u-fake"}, "--as user", "Bearer u-fake"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var downloadAuth string
			newCommentPermTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/open-apis/drive/v1/export_tasks":
					_, _ = fmt.Fprint(w, `{"code":0,"data":{"ticket":"tk_fake"}}`)
				case r.URL.Path == "/open-apis/drive/v1/export_tasks/tk_fake":
					_, _ = fmt.Fprint(w, `{"code":0,"data":{"result":{"job_status":0,"file_token":"boxFakeExport","file_name":"fp-test 报告","file_extension":"pdf","type":"docx"}}}`)
				case r.URL.Path == "/open-apis/drive/v1/export_tasks/file/boxFakeExport/download":
					downloadAuth = r.Header.Get("Authorization")
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, `{"code":1061004,"msg":"forbidden"}`)
				default:
					http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
				}
			})
			outDir := filepath.Join(t.TempDir(), "out dir")
			flags := append([]string{"--token", "doxcnFake", "--doc-type", "docx", "--file-extension", "pdf", "--output-dir", outDir}, tc.flags...)
			_, _, err := runCmdWithFlags(t, driveExportCmd, nil, flags...)
			if err == nil {
				t.Fatal("下载失败应返回错误")
			}
			if downloadAuth != tc.wantAuth {
				t.Fatalf("下载身份 = %q, want %q", downloadAuth, tc.wantAuth)
			}
			want := "可重试: feishu-cli drive export-download --file-token 'boxFakeExport' --output-dir '" + outDir + "' --file-name 'fp-test 报告.pdf' " + tc.wantAs
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("重试命令不对:\n%s\nwant 包含:\n%s", err.Error(), want)
			}
		})
	}
	dl, _, err := rootCmd.Find([]string{"drive", "export-download"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file-token", "output-dir", "file-name", "as"} {
		if dl.Flags().Lookup(name) == nil {
			t.Fatalf("drive export-download 缺少 --%s", name)
		}
	}
}
