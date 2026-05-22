package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// stubFeishuServer 起一个本地 httptest server 替代真实飞书 OAPI；
// 通过 base_url 配置注入到 cli。
func stubFeishuServer(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)

	resetClient()
	resetConfig()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := fmt.Sprintf(`app_id: "test_app_id"
app_secret: "test_app_secret"
base_url: "%s"
`, srv.URL)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}

	return srv.URL, srv.Close
}

func TestGetMessageWithUserToken_CardContentType(t *testing.T) {
	tests := []struct {
		name             string
		cardContentType  string
		expectedQueryArg string
	}{
		{"空 → 不传 query", "", ""},
		{"user_card_content", CardMsgContentTypeUser, "user_card_content"},
		{"raw_card_content", CardMsgContentTypeRaw, "raw_card_content"},
	}

	const messageID = "om_x100b512ca9a404b8b2432e156aa8895"
	const userToken = "u-test-token"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery url.Values
			var capturedAuth string
			handler := func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = r.URL.Query()
				capturedAuth = r.Header.Get("Authorization")
				expectedPath := "/open-apis/im/v1/messages/" + messageID
				if r.URL.Path != expectedPath {
					t.Errorf("path 不符: got %q, want %q", r.URL.Path, expectedPath)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"interactive","body":{"content":"{}"}}]}}`, messageID)
			}

			_, cleanup := stubFeishuServer(t, handler)
			defer cleanup()

			result, err := getMessageWithUserToken(messageID, userToken, tt.cardContentType)
			if err != nil {
				t.Fatalf("getMessageWithUserToken 返回错误: %v", err)
			}
			if result == nil || result.Message == nil {
				t.Fatalf("返回结果为空")
			}
			if got := capturedQuery.Get("card_msg_content_type"); got != tt.expectedQueryArg {
				t.Errorf("card_msg_content_type query: got %q, want %q", got, tt.expectedQueryArg)
			}
			if capturedAuth != "Bearer "+userToken {
				t.Errorf("Authorization header: got %q, want %q", capturedAuth, "Bearer "+userToken)
			}
		})
	}
}

func TestListMessagesWithUserToken_CardContentType(t *testing.T) {
	const containerID = "oc_test_chat"
	const userToken = "u-test-token"

	var capturedQuery url.Values
	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	opts := ListMessagesOptions{
		ContainerIDType: "chat",
		PageSize:        10,
		CardContentType: CardMsgContentTypeUser,
	}
	if _, err := ListMessages(containerID, opts, userToken); err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}

	if got := capturedQuery.Get("card_msg_content_type"); got != CardMsgContentTypeUser {
		t.Errorf("card_msg_content_type query: got %q, want %q", got, CardMsgContentTypeUser)
	}
	if got := capturedQuery.Get("container_id"); got != containerID {
		t.Errorf("container_id query: got %q, want %q", got, containerID)
	}
	if got := capturedQuery.Get("container_id_type"); got != "chat" {
		t.Errorf("container_id_type query: got %q, want %q", got, "chat")
	}
}

func TestGetMessageWithUserToken_ApiErrorPropagated(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":230002,"msg":"permission denied","data":{}}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	_, err := getMessageWithUserToken("om_xxx", "u-test", CardMsgContentTypeUser)
	if err == nil {
		t.Fatal("API 返回 code != 0 应返回错误")
	}
}

func TestDownloadMessageResourceWithUserToken_RawHTTP(t *testing.T) {
	const (
		messageID = "om_xxx"
		fileKey   = "file_v3_xxx"
		userToken = "u-test-token"
	)
	wantBody := []byte("fake-video")

	var capturedAuth string
	var capturedPath string
	var capturedType string
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatal("显式 User Token 下载资源时不应请求 tenant token")
		}
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedType = r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "video/quicktime")
		_, _ = w.Write(wantBody)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	outputPath := t.TempDir() + "/video.mov"
	if err := DownloadMessageResource(messageID, fileKey, "file", outputPath, userToken); err != nil {
		t.Fatalf("DownloadMessageResource 返回错误: %v", err)
	}
	gotBody, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("读取下载文件失败: %v", err)
	}
	if string(gotBody) != string(wantBody) {
		t.Fatalf("下载内容 = %q, want %q", gotBody, wantBody)
	}
	if capturedPath != "/open-apis/im/v1/messages/"+messageID+"/resources/"+fileKey {
		t.Errorf("path = %q, want resource path", capturedPath)
	}
	if capturedType != "file" {
		t.Errorf("type query = %q, want file", capturedType)
	}
	if capturedAuth != "Bearer "+userToken {
		t.Errorf("Authorization header: got %q, want %q", capturedAuth, "Bearer "+userToken)
	}
}

func TestDownloadMessageResourceWithUserToken_ApiError(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":234040,"msg":"The message is invisible to the operator."}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	err := DownloadMessageResource("om_xxx", "file_v3_xxx", "file", t.TempDir()+"/video.mov", "u-test-token")
	if err == nil {
		t.Fatal("API 返回错误时应返回 error")
	}
	if !strings.Contains(err.Error(), "234040") {
		t.Fatalf("error = %q, want code 234040", err)
	}
}

// tenantRouteHandler 封装 tenant 模式 stub：识别 SDK 自动发起的 tenant_access_token
// 取 token 请求并回 fake token；其他路径转交给业务 handler。
func tenantRouteHandler(t *testing.T, business http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		business(w, r)
	}
}

// TestListMessagesTenantRaw_CardContentType 覆盖 tenant 模式（userAccessToken 为空）
// 走 listMessagesViaRawRequest 的路径——CardContentType 非空时本应触发 raw HTTP 走 SDK
// raw request，把 card_msg_content_type 写入 query params。
func TestListMessagesTenantRaw_CardContentType(t *testing.T) {
	const containerID = "oc_test_chat"

	var capturedQuery url.Values
	var capturedAuth string
	var capturedPath string
	business := func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
	}

	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	opts := ListMessagesOptions{
		ContainerIDType: "chat",
		PageSize:        10,
		CardContentType: CardMsgContentTypeRaw,
	}
	if _, err := ListMessages(containerID, opts, ""); err != nil {
		t.Fatalf("ListMessages tenant 模式返回错误: %v", err)
	}

	if capturedPath != "/open-apis/im/v1/messages" {
		t.Errorf("请求 path: got %q, want %q", capturedPath, "/open-apis/im/v1/messages")
	}
	if got := capturedQuery.Get("card_msg_content_type"); got != CardMsgContentTypeRaw {
		t.Errorf("card_msg_content_type query: got %q, want %q", got, CardMsgContentTypeRaw)
	}
	if got := capturedQuery.Get("container_id"); got != containerID {
		t.Errorf("container_id query: got %q, want %q", got, containerID)
	}
	if got := capturedQuery.Get("container_id_type"); got != "chat" {
		t.Errorf("container_id_type query: got %q, want %q", got, "chat")
	}
	// tenant 模式应使用 tenant token，前缀 "Bearer t-" 来自 stub fake token
	if !strings.HasPrefix(capturedAuth, "Bearer t-") {
		t.Errorf("tenant 模式应使用 tenant_access_token，Authorization=%q", capturedAuth)
	}
}

// TestGetMessageTenantRaw_CardContentType 覆盖 tenant 模式
// 走 getMessageViaRawRequest 的路径。
func TestGetMessageTenantRaw_CardContentType(t *testing.T) {
	const messageID = "om_x100b512ca9a404b8b2432e156aa8895"

	var capturedQuery url.Values
	var capturedAuth string
	var capturedPath string
	business := func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"interactive","body":{"content":"{}"}}]}}`, messageID)
	}

	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	result, err := GetMessage(messageID, "", CardMsgContentTypeUser)
	if err != nil {
		t.Fatalf("GetMessage tenant 模式返回错误: %v", err)
	}
	if result == nil || result.Message == nil {
		t.Fatalf("返回结果为空")
	}

	expectedPath := "/open-apis/im/v1/messages/" + messageID
	if capturedPath != expectedPath {
		t.Errorf("请求 path: got %q, want %q", capturedPath, expectedPath)
	}
	if got := capturedQuery.Get("card_msg_content_type"); got != CardMsgContentTypeUser {
		t.Errorf("card_msg_content_type query: got %q, want %q", got, CardMsgContentTypeUser)
	}
	if !strings.HasPrefix(capturedAuth, "Bearer t-") {
		t.Errorf("tenant 模式应使用 tenant_access_token，Authorization=%q", capturedAuth)
	}
}
