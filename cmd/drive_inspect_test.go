package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/viper"
)

func initDriveInspectTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// TestDriveInspectWikiDoesNotSendObjTypeWiki 验证 Wiki 节点 inspect 绝不发送 obj_type=wiki 参数
func TestDriveInspectWikiDoesNotSendObjTypeWiki(t *testing.T) {
	getNodeCalled := false
	var gotObjTypeQuery string
	var hasObjTypeQuery bool
	var gotTokenQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			getNodeCalled = true
			gotTokenQuery = r.URL.Query().Get("token")
			hasObjTypeQuery = r.URL.Query().Has("obj_type")
			gotObjTypeQuery = r.URL.Query().Get("obj_type")

			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"node": {
						"space_id": "sp-123",
						"node_token": "wikcnInspect123",
						"obj_token": "doxcnRealDoc",
						"obj_type": "docx"
					}
				}
			}`)
		case strings.Contains(r.URL.Path, "metas/batch_query"):
			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"metas": [
						{
							"title": "测试文档标题",
							"doc_token": "doxcnRealDoc",
							"doc_type": "docx"
						}
					]
				}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDriveInspectTestConfig(t, server.URL)

	_ = driveInspectCmd.Flags().Set("url", "https://sample.feishu.cn/wiki/wikcnInspect123")
	defer func() {
		_ = driveInspectCmd.Flags().Set("url", "")
	}()

	err := driveInspectCmd.RunE(driveInspectCmd, []string{})
	if err != nil {
		t.Fatalf("driveInspectCmd 运行失败: %v", err)
	}

	if !getNodeCalled {
		t.Fatal("未调用 /open-apis/wiki/v2/spaces/node_by_token")
	}
	if gotTokenQuery != "wikcnInspect123" {
		t.Fatalf("Query token = %q, 期望 wikcnInspect123", gotTokenQuery)
	}
	if hasObjTypeQuery {
		t.Fatalf("Wiki node inspect 请求 query 绝不应包含 obj_type 参数！实际包含: obj_type=%q", gotObjTypeQuery)
	}
}
