package client

import (
	"fmt"
	"os"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// setupTestConfig 让 client 指向 httptest 服务器（共用：rawapi / bitable_v1 / search_enrich 测试）。
func setupTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	resetClient()
	resetConfig()
	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := fmt.Sprintf("app_id: \"test_app\"\napp_secret: \"test_secret\"\nbase_url: \"%s\"\n", baseURL)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
}
