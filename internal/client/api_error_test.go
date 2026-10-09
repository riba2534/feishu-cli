package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/config"
)

func TestParseAPIResponse(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantNil  bool
		wantCode int
		wantSub  []string
	}{
		{name: "HTTP 200 code 0 成功", status: 200, body: `{"code":0,"data":{}}`, wantNil: true},
		{name: "HTTP 200 无信封（如 JSON 数组）成功", status: 200, body: `[1,2]`, wantNil: true},
		{
			name: "HTTP 400 业务信封 → APIError（不是 HTTP 400 文本）", status: 400,
			body:     `{"code":1063004,"msg":"forbidden","error":{"log_id":"L1"}}`,
			wantCode: 1063004, wantSub: []string{"查询密级标签失败: code=1063004, msg=forbidden, log_id=L1"},
		},
		{
			name: "HTTP 200 业务错误", status: 200, body: `{"code":99991679,"msg":"Unauthorized"}`,
			wantCode: 99991679, wantSub: []string{"code=99991679"},
		},
		{
			name: "HTTP 502 非信封 → 带状态码与预览", status: 502, body: `<html>bad gateway</html>`,
			wantSub: []string{"查询密级标签失败: HTTP 502, body: <html>bad gateway</html>"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ParseAPIResponse("查询密级标签", tc.status, nil, []byte(tc.body))
			if tc.wantNil {
				if err != nil {
					t.Fatalf("期望 nil，得到 %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("期望错误，得到 nil")
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("错误 %q 应包含 %q", err.Error(), sub)
				}
			}
			apiErr, ok := AsAPIError(err)
			if tc.wantCode == 0 {
				if ok {
					t.Fatalf("非信封错误不应是 *APIError: %v", err)
				}
				return
			}
			if !ok || apiErr.Code != tc.wantCode || apiErr.HTTPStatus != tc.status {
				t.Fatalf("应返回 *APIError{Code:%d, HTTPStatus:%d}，得到 %#v", tc.wantCode, tc.status, apiErr)
			}
			if !HasAPICode(fmt.Errorf("wrap: %w", err), tc.wantCode) {
				t.Fatal("HasAPICode 应能识别 APIError 的业务码")
			}
		})
	}
}

func TestCheckAPIResponseUsesHeaderLogID(t *testing.T) {
	resp := &larkcore.ApiResp{
		StatusCode: 400,
		Header:     http.Header{"X-Tt-Logid": []string{"HDR"}},
		RawBody:    []byte(`{"code":1254005,"msg":"not found"}`),
	}
	err := CheckAPIResponse("获取资源", resp)
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.LogID != "HDR" {
		t.Fatalf("应从 X-Tt-Logid 头补 log_id，得到 %v", err)
	}
}

func TestAPIErrorHint(t *testing.T) {
	t.Setenv("FEISHU_BASE_URL", "")
	setupTestConfig(t, "https://open.larksuite.com")

	cases := []struct {
		name    string
		info    apidiag.Info
		wantSub []string
		notSub  []string
	}{
		{
			name:    "99991672 指向开放平台开通（按品牌域名），不让重新登录",
			info:    apidiag.Info{Code: 99991672, MissingScopes: []string{"okr:okr:readonly", "okr:okr"}},
			wantSub: []string{"https://open.larksuite.com/app/test_app/auth?q=okr%3Aokr%3Areadonly%2Cokr%3Aokr", "重新 auth login 无法解决"},
		},
		{
			name:    "99991679 给出最窄 scope 的 auth login 命令",
			info:    apidiag.Info{Code: 99991679, MissingScopes: []string{"okr:okr.period:readonly", "okr:okr"}},
			wantSub: []string{`feishu-cli auth login --scope "okr:okr.period:readonly"`, "任选其一"},
		},
		{
			name:    "99991679 无 scope 信息时给出通用命令",
			info:    apidiag.Info{Code: 99991679},
			wantSub: []string{"auth login --scope", "auth check --scope"},
		},
		{name: "99991668 not support → --as bot", info: apidiag.Info{Code: 99991668, Msg: "user access token not support"}, wantSub: []string{"--as bot"}},
		{name: "99991668 无效 token → 登录", info: apidiag.Info{Code: 99991668, Msg: "Invalid access token for authorization."}, wantSub: []string{"auth login"}, notSub: []string{"--as bot"}},
		{name: "refresh 终态", info: apidiag.Info{Code: 20037}, wantSub: []string{"重新 `feishu-cli auth login`"}},
		{name: "领域错误码不给通用提示", info: apidiag.Info{Code: 230001}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hint := APIErrorHint(tc.info)
			if len(tc.wantSub) == 0 && hint != "" {
				t.Fatalf("期望无提示，得到 %q", hint)
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(hint, sub) {
					t.Errorf("提示 %q 应包含 %q", hint, sub)
				}
			}
			for _, sub := range tc.notSub {
				if strings.Contains(hint, sub) {
					t.Errorf("提示 %q 不应包含 %q", hint, sub)
				}
			}
		})
	}
}

// TestPolicyTransportRecordsDiagnostics 验证经受控 HTTP 客户端的错误响应会被旁路记录，
// 旧调用点即使只用 "code=%d, msg=%s" 格式化错误，根命令仍能找回 log_id 与字段校验。
func TestPolicyTransportRecordsDiagnostics(t *testing.T) {
	apidiag.Reset()
	defer apidiag.Reset()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":99992402,"msg":"field validation failed","error":{"log_id":"LOGX","field_violations":[{"field":"document_id","description":"the min len is 27"}]}}`)
	}))
	defer srv.Close()

	resp, err := config.NewHTTPClient(0).Get(srv.URL + "/open-apis/docx/v1/documents/bad")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	opErr := fmt.Errorf("获取文档失败: code=%d, msg=%s", 99992402, "field validation failed")
	info, ok := apidiag.Lookup(func(i apidiag.Info) bool { return HasAPICode(opErr, i.Code) })
	if !ok || info.LogID != "LOGX" || len(info.FieldViolations) != 1 {
		t.Fatalf("应记录错误响应诊断，得到 ok=%v info=%+v", ok, info)
	}
}
