package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBitableV1Path(t *testing.T) {
	got := BitableV1Path("apps", "bascn123", "roles", "rolx", "members")
	want := "/open-apis/bitable/v1/apps/bascn123/roles/rolx/members"
	if got != want {
		t.Errorf("BitableV1Path = %q, want %q", got, want)
	}
	// 空段应被跳过
	if got := BitableV1Path("apps", "", "x"); got != "/open-apis/bitable/v1/apps/x" {
		t.Errorf("空段未跳过: %q", got)
	}
}

func TestBitableV1CallGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/bitable/v1/apps/bascn1/roles/r1/members" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		// X-App-Id 不应被设置（bitable/v1 与 base/v3 区别）
		if r.Header.Get("X-App-Id") != "" {
			t.Errorf("bitable/v1 不应带 X-App-Id, 得 %q", r.Header.Get("X-App-Id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[{"member_id":"ou_a"}]}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	data, err := BitableV1Call("GET", BitableV1Path("apps", "bascn1", "roles", "r1", "members"), nil, nil, "u-test")
	if err != nil {
		t.Fatalf("BitableV1Call error: %v", err)
	}
	if _, ok := data["items"]; !ok {
		t.Errorf("data 缺 items: %v", data)
	}
}

func TestBitableV1CallPOSTBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "Enable") {
			t.Errorf("body 缺 status=Enable: %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"workflow_id":"wf1"}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := BitableV1Call("PUT", BitableV1Path("apps", "b1", "workflows", "wf1"), nil, map[string]any{"status": "Enable"}, "u-test")
	if err != nil {
		t.Fatalf("BitableV1Call error: %v", err)
	}
}

func TestBitableV1CallAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":1254045,"msg":"FieldNameNotFound"}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := BitableV1Call("GET", BitableV1Path("apps", "b1"), nil, nil, "u-test")
	if err == nil {
		t.Errorf("code!=0 应返回错误")
	}
}
