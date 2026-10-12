package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

// gatewayEnv 准备网关模式环境：只有 app_id，没有 app_secret，base_url 指向本地假网关。
func gatewayEnv(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	// 其他用例的 viper.Set（--bot-app-* 覆盖）会跨用例残留，先复位
	viper.Reset()
	t.Cleanup(viper.Reset)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_GATEWAY", "1")
	t.Setenv("FEISHU_APP_ID", "cli_gateway")
	t.Setenv("FEISHU_APP_SECRET", "")
	t.Setenv("FEISHU_BASE_URL", srv.URL)
	config.SetBotFlagCredentials("", "")
	if err := os.MkdirAll(filepath.Join(home, ".feishu-cli"), 0700); err != nil {
		t.Fatal(err)
	}
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string {
		t.Error("网关模式不应解析 Accounts 端点")
		return "http://127.0.0.1:9/unused"
	}
	t.Cleanup(func() { auth.TATEndpointFunc = orig })
}

func TestAuthToken_GatewayModeFetchesThroughBaseURL(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	gatewayEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-gateway","expire":7200}`))
	})

	stdout, _, err := runCLI(t, "auth", "token", "--as", "bot")
	if err != nil {
		t.Fatalf("auth token --as bot: %v", err)
	}
	if strings.TrimSpace(stdout) != "t-gateway" {
		t.Fatalf("stdout = %q", stdout)
	}
	if gotPath != "/open-apis/auth/v3/tenant_access_token/internal" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotBody["app_id"] != "cli_gateway" {
		t.Fatalf("body = %v", gotBody)
	}
	if _, ok := gotBody["app_secret"]; ok {
		t.Fatalf("网关模式未配置 secret 时不应发送 app_secret: %v", gotBody)
	}
}

func TestAuthLogin_RejectedInGatewayMode(t *testing.T) {
	gatewayEnv(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("auth login 在网关模式下不应发出请求: %s", r.URL.Path)
	})
	_, _, err := runCLI(t, "auth", "login", "--scope", "im:message", "--no-wait", "--json")
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "网关模式") {
		t.Fatalf("网关模式下 auth login 应报用法错误，得到 %v", err)
	}
}

func TestDoctor_GatewayModeChecksGatewayOnly(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	gatewayEnv(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-gateway","expire":7200}`))
		}
	})

	stdout, _, err := runCLI(t, "doctor", "--json", "--only", "config_file,bot_identity,endpoint_open,endpoint_larksuite")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, stdout)
	}
	var out struct {
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("解析 doctor JSON: %v\n%s", err, stdout)
	}
	status := map[string]string{}
	for _, r := range out.Checks {
		status[r.Name] = r.Status
	}
	want := map[string]string{"config_file": "pass", "bot_identity": "pass", "endpoint_open": "pass", "endpoint_larksuite": "skip"}
	for name, s := range want {
		if status[name] != s {
			t.Fatalf("%s = %q, want %q（%s）", name, status[name], s, stdout)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	got := strings.Join(paths, ",")
	if !strings.Contains(got, "POST /open-apis/auth/v3/tenant_access_token/internal") || !strings.Contains(got, "HEAD /") {
		t.Fatalf("doctor 应经网关换 token 并探测网关: %s", got)
	}
}
