package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
)

func legacyInternalTokenRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://open.feishu.cn"+legacyTenantTokenInternalPath,
		strings.NewReader(`{"app_id":"cli_x","app_secret":"secret_x"}`))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// 换取 tenant_access_token 时的网络错误要原样作为传输层错误返回（保留 net.Error），不再合成 99991663。
func TestLegacyTokenBridge_NetworkErrorIsTransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	deadURL := srv.URL
	srv.Close() // 端口已关闭：connection refused

	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return deadURL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	resp, err := fulfillLegacyInternalTokenViaAccountsV3(legacyInternalTokenRequest(t))
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("网络错误应作为传输错误返回，实际合成了响应: %s", body)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("应保留 net.Error 链: %T %v", err, err)
	}
}

// 服务端业务拒绝仍按原样合成 99991663 业务信封（错误文本与退出码不变）。
func TestLegacyTokenBridge_BusinessRejectStillSynthesized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	}))
	t.Cleanup(srv.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return srv.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	resp, err := fulfillLegacyInternalTokenViaAccountsV3(legacyInternalTokenRequest(t))
	if err != nil {
		t.Fatalf("业务拒绝不应变成传输错误: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "99991663") {
		t.Fatalf("应合成 99991663: %s", body)
	}
}

func TestIsTokenFetchNetworkError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net", fmt.Errorf("请求 tenant token 失败: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")}), true},
		{"deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), true},
		{"clierr network", clierr.Network(errors.New("x")), true},
		{"tat reject", &auth.TATError{Code: 20002}, false},
		{"plain", errors.New("缺少 app_id 或 app_secret 配置"), false},
	}
	for _, c := range cases {
		if got := isTokenFetchNetworkError(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestSDKLogger_SilentUnlessDebug(t *testing.T) {
	var buf bytes.Buffer
	orig := sdkLogWriter
	sdkLogWriter = &buf
	t.Cleanup(func() { sdkLogWriter = orig })

	newSDKLogger(false).Warn(context.Background(), "x")
	if buf.Len() != 0 {
		t.Fatalf("非 debug 应静默: %q", buf.String())
	}
	newSDKLogger(true).Warn(context.Background(), "hello")
	if !strings.Contains(buf.String(), "[Warn]") || !strings.Contains(buf.String(), "hello") {
		t.Fatalf("debug 应输出到 sdkLogWriter: %q", buf.String())
	}
}

// 端到端：换票被拒时 SDK 会打 [Warn] 日志，不得写到 stdout（-o json 管道安全）。
func TestGetClient_SDKWarnNotOnStdout(t *testing.T) {
	resetClient()
	resetConfig()
	t.Cleanup(func() { resetClient(); resetConfig() })
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	}))
	t.Cleanup(accounts.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return accounts.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })
	sdkTestTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected %s", req.URL)
	})

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("app_id: \"cli_x\"\napp_secret: \"secret_x\"\nbase_url: \"https://open.feishu.cn\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}

	// SDK 默认 logger 在 NewClient 时绑定 os.Stdout，因此先换 stdout 再建 client
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	captured := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		captured <- string(b)
	}()

	cli, getErr := GetClient()
	var callErr error
	if getErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, callErr = cli.Do(ctx, &larkcore.ApiReq{
			HttpMethod:                http.MethodGet,
			ApiPath:                   "/open-apis/im/v1/chats",
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
		})
		cancel()
	}
	os.Stdout = origStdout
	_ = w.Close()
	out := <-captured

	if getErr != nil {
		t.Fatal(getErr)
	}
	if callErr == nil || !strings.Contains(callErr.Error(), "99991663") {
		t.Fatalf("应返回换票失败错误: %v", callErr)
	}
	if out != "" {
		t.Fatalf("SDK 日志不得写 stdout: %q", out)
	}
}
