package converter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/viper"
)

func initConverterBrandConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "app_id: cli_test\napp_secret: s\nbase_url: \"" + baseURL + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(path); err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
}

// TestNormalizeURL_LarkBrand 验证 feishu:// 内部链接按品牌转换：Lark 租户不能落到飞书国内站。
func TestNormalizeURL_LarkBrand(t *testing.T) {
	t.Setenv("FEISHU_BASE_URL", "")
	t.Cleanup(func() {
		// 恢复为飞书品牌，避免影响同包其他用例
		initConverterBrandConfig(t, config.OfficialFeishuOpen)
		viper.Reset()
	})
	initConverterBrandConfig(t, config.OfficialLarkOpen)
	if got := normalizeURL("feishu://doc/ABC123"); got != "https://www.larksuite.com/docx/ABC123" {
		t.Fatalf("Lark 品牌 normalizeURL = %q，期望 https://www.larksuite.com/docx/ABC123", got)
	}
	initConverterBrandConfig(t, config.OfficialFeishuOpen)
	if got := normalizeURL("feishu://wiki/NODE"); got != "https://www.feishu.cn/wiki/NODE" {
		t.Fatalf("飞书品牌 normalizeURL = %q，期望 https://www.feishu.cn/wiki/NODE", got)
	}
}
