package config

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestGatewayMode_EnvOverridesConfig(t *testing.T) {
	resetConfig()
	t.Setenv(envGateway, "")
	os.Unsetenv(envGateway) // t.Setenv 负责结束后恢复
	if GatewayMode() {
		t.Fatal("默认不应启用网关模式")
	}
	cfg = &Config{Gateway: true}
	t.Cleanup(resetConfig)
	if !GatewayMode() {
		t.Fatal("配置 gateway: true 应启用网关模式")
	}
	t.Setenv(envGateway, "0")
	if GatewayMode() {
		t.Fatal("FEISHU_GATEWAY=0 应覆盖配置文件")
	}
}

func TestCheckBaseURL_GatewayRequiresNonOfficialBase(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCustomBaseURL, "")
	t.Setenv(envAllowInsecureHTTP, "")
	t.Setenv(envGateway, "1")

	for _, raw := range []string{"", OfficialFeishuOpen, OfficialLarkOpen, "https://accounts.feishu.cn", "https://open.feishu.cn:8443"} {
		err := CheckBaseURL(raw)
		if err == nil {
			t.Fatalf("网关模式下 CheckBaseURL(%q) 应失败", raw)
		}
		if !strings.Contains(err.Error(), "网关模式") {
			t.Fatalf("错误应说明网关模式要求: %v", err)
		}
	}
	if err := CheckBaseURL("http://127.0.0.1:18080"); err != nil {
		t.Fatalf("网关模式下 loopback base_url 应允许: %v", err)
	}
	if err := CheckBaseURL("https://gateway.example.com"); err == nil {
		t.Fatal("网关模式不放宽自定义远端 host 的 opt-in 要求")
	}
	t.Setenv(envAllowCustomBaseURL, "1")
	if err := CheckBaseURL("https://gateway.example.com"); err != nil {
		t.Fatalf("opt-in 后自定义 HTTPS 网关应允许: %v", err)
	}
}

func TestCheckRequestURL_GatewayRejectsOfficialHosts(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCustomBaseURL, "")
	t.Setenv(envGateway, "1")

	for _, raw := range []string{
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"https://accounts.feishu.cn/oauth/v3/token",
		"https://open.larksuite.com/open-apis/im/v1/messages",
		"https://accounts.larksuite.com/oauth/v1/device_authorization",
	} {
		u, _ := url.Parse(raw)
		if err := CheckRequestURL(u); err == nil {
			t.Fatalf("网关模式下应拒绝直连官方 host: %s", raw)
		}
	}
	u, _ := url.Parse("http://127.0.0.1:18080/open-apis/auth/v3/tenant_access_token/internal")
	if err := CheckRequestURL(u); err != nil {
		t.Fatalf("网关模式下 loopback 网关应允许: %v", err)
	}

	t.Setenv(envGateway, "")
	u, _ = url.Parse("https://accounts.feishu.cn/oauth/v3/token")
	if err := CheckRequestURL(u); err != nil {
		t.Fatalf("非网关模式下官方 Accounts 应允许: %v", err)
	}
}

func TestValidate_GatewayDoesNotRequireAppSecret(t *testing.T) {
	resetConfig()
	t.Setenv(envGateway, "")
	t.Cleanup(resetConfig)
	cfg = &Config{AppID: "cli_app", BaseURL: "http://127.0.0.1:18080"}
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "app_secret") {
		t.Fatalf("非网关模式缺少 app_secret 应报错，得到 %v", err)
	}
	if cfg.HasAppCredentials() {
		t.Fatal("非网关模式缺少 app_secret 时 HasAppCredentials 应为 false")
	}

	t.Setenv(envGateway, "1")
	if err := Validate(); err != nil {
		t.Fatalf("网关模式下不需要 app_secret: %v", err)
	}
	if !cfg.HasAppCredentials() {
		t.Fatal("网关模式下只有 app_id 时 HasAppCredentials 应为 true")
	}

	cfg.AppID = ""
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "app_id") {
		t.Fatalf("网关模式下仍需要 app_id，得到 %v", err)
	}
	cfg.AppID = "cli_app"
	cfg.BaseURL = OfficialFeishuOpen
	if err := Validate(); err == nil {
		t.Fatal("网关模式下 base_url 为官方地址应报错")
	}
}
