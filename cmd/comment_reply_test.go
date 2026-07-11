package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newDeleteReplyTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "docx", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func initDeleteReplyTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\nuser_access_token: u-config-token\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

func TestDeleteReplyIdentity(t *testing.T) {
	tests := []struct {
		name      string
		flagToken string
		wantAuth  string
	}{
		{
			name:     "默认忽略配置中的 User Token 并使用 Bot",
			wantAuth: "Bearer t-test-token",
		},
		{
			name:      "显式 User Token 使用用户身份",
			flagToken: "u-flag-token",
			wantAuth:  "Bearer u-flag-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test-token","expire":7200}`)
				case "/open-apis/drive/v1/files/doc_test/comments/cmt_test/replies/rep_test":
					gotAuth = r.Header.Get("Authorization")
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
				default:
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
				}
			}))
			defer server.Close()

			initDeleteReplyTestConfig(t, server.URL)
			cmd := newDeleteReplyTestCmd()
			if tt.flagToken != "" {
				if err := cmd.Flags().Set("user-access-token", tt.flagToken); err != nil {
					t.Fatalf("设置 User Token 失败: %v", err)
				}
			}

			if err := deleteReplyCmd.RunE(cmd, []string{"doc_test", "cmt_test", "rep_test"}); err != nil {
				t.Fatalf("删除回复失败: %v", err)
			}
			if gotAuth != tt.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

func TestAddReplyIdentity(t *testing.T) {
	tests := []struct {
		name      string
		flagToken string
		wantAuth  string
	}{
		{
			name:     "默认忽略配置中的 User Token 并使用 Bot",
			wantAuth: "Bearer t-test-token",
		},
		{
			name:      "显式 User Token 使用用户身份",
			flagToken: "u-flag-token",
			wantAuth:  "Bearer u-flag-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test-token","expire":7200}`)
				case "/open-apis/drive/v1/files/doc_test/comments/cmt_test/replies":
					gotAuth = r.Header.Get("Authorization")
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"reply_id":"rep_test"}}`)
				default:
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
				}
			}))
			defer server.Close()

			initDeleteReplyTestConfig(t, server.URL)
			cmd := newDeleteReplyTestCmd()
			cmd.Flags().String("text", "测试回复", "")
			cmd.Flags().String("output", "", "")
			if tt.flagToken != "" {
				if err := cmd.Flags().Set("user-access-token", tt.flagToken); err != nil {
					t.Fatalf("设置 User Token 失败: %v", err)
				}
			}

			if err := addReplyCmd.RunE(cmd, []string{"doc_test", "cmt_test"}); err != nil {
				t.Fatalf("添加回复失败: %v", err)
			}
			if gotAuth != tt.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}
