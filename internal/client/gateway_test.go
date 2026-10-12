package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/config"
)

// 网关模式：未配置 app_secret 时 SDK 仍可用 Bot 身份调用；SDK 的 tenant token 请求经 base_url 换取，
// 发给网关的请求体只有 app_id（SDK 本地占位 secret 不会发出）。
func TestGetClient_GatewayModeWithoutSecret(t *testing.T) {
	var (
		mu         sync.Mutex
		tokenCalls int
		tokenBody  map[string]any
		apiAuth    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCalls++
			b, _ := io.ReadAll(r.Body)
			tokenBody = nil
			_ = json.Unmarshal(b, &tokenBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-gateway","expire":7200}`)
		case "/open-apis/test/v1/ping":
			apiAuth = r.Header.Get("Authorization")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	resetClient()
	resetConfig()
	t.Setenv("FEISHU_GATEWAY", "1")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	configFile := t.TempDir() + "/config.yaml"
	content := fmt.Sprintf("app_id: \"cli_gateway\"\nbase_url: %q\n", srv.URL)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}

	cli, err := GetClient()
	if err != nil {
		t.Fatalf("网关模式下无 app_secret 应能创建 client: %v", err)
	}
	if _, err := cli.Get(Context(), "/open-apis/test/v1/ping", nil, larkcore.AccessTokenTypeTenant); err != nil {
		t.Fatalf("Bot 调用失败: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokenCalls != 1 {
		t.Fatalf("tenant token 请求次数 = %d", tokenCalls)
	}
	if tokenBody["app_id"] != "cli_gateway" {
		t.Fatalf("token 请求体应含 app_id: %v", tokenBody)
	}
	if _, ok := tokenBody["app_secret"]; ok {
		t.Fatalf("SDK 占位 secret 不应发给网关: %v", tokenBody)
	}
	if apiAuth != "Bearer t-gateway" {
		t.Fatalf("API Authorization = %q", apiAuth)
	}
}

func TestGetClient_WithoutSecretOutsideGatewayModeStillFails(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_GATEWAY", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	configFile := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("app_id: \"cli_x\"\nbase_url: \"http://127.0.0.1:9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}
	if _, err := GetClient(); err == nil {
		t.Fatal("非网关模式缺少 app_secret 应报错")
	}
}

// 网关模式：drive 下载自取 tenant token 同样经 base_url，且请求体不含 secret。
func TestCachedTenantAccessToken_GatewayMode(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-drive","expire":7200}`)
	}))
	defer srv.Close()

	resetClient()
	resetConfig()
	resetTenantTokenCacheForTest()
	t.Cleanup(resetTenantTokenCacheForTest)
	t.Setenv("FEISHU_GATEWAY", "1")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	configFile := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configFile, []byte(fmt.Sprintf("app_id: \"cli_gateway\"\nbase_url: %q\n", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}

	tok, err := cachedTenantAccessToken()
	if err != nil || tok != "t-drive" {
		t.Fatalf("token=%q err=%v", tok, err)
	}
	if _, ok := body["app_secret"]; ok || body["app_id"] != "cli_gateway" {
		t.Fatalf("请求体 = %v", body)
	}
}
