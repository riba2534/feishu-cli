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

	"github.com/riba2534/feishu-cli/internal/config"
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
