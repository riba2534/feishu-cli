package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestApiErrorDetail 验证错误文案提取优先级：顶层 msg → data.error.hint → data.error.message。
func TestApiErrorDetail(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{
			name: "msg and hint are joined",
			in:   map[string]any{"msg": "bad", "data": map[string]any{"error": map[string]any{"hint": "h"}}},
			want: "bad: h",
		},
		{
			name: "msg equal to detail not duplicated",
			in:   map[string]any{"msg": "not_found", "data": map[string]any{"error": map[string]any{"message": "not_found"}}},
			want: "not_found",
		},
		{
			name: "path appended to detail",
			in:   map[string]any{"msg": "not_found", "data": map[string]any{"error": map[string]any{"hint": "用已有选项", "path": "update_records.recX.单选"}}},
			want: "not_found: 用已有选项（字段路径: update_records.recX.单选）",
		},
		{
			name: "empty msg falls back to hint",
			in:   map[string]any{"msg": "", "data": map[string]any{"error": map[string]any{"hint": "缺少 name", "message": "m"}}},
			want: "缺少 name",
		},
		{
			name: "hint empty falls back to message",
			in:   map[string]any{"msg": "", "data": map[string]any{"error": map[string]any{"message": "validation failed"}}},
			want: "validation failed",
		},
		{
			name: "no detail anywhere",
			in:   map[string]any{"msg": ""},
			want: "",
		},
		{
			name: "data not a map",
			in:   map[string]any{"msg": "", "data": "x"},
			want: "",
		},
		{
			name: "error not a map",
			in:   map[string]any{"msg": "", "data": map[string]any{"error": "x"}},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apiErrorDetail(tc.in); got != tc.want {
				t.Errorf("apiErrorDetail = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBaseV3CallSurfacesErrorHint 锁住 B3 修复：base/v3 业务错误（HTTP 200 + code!=0 +
// 顶层 msg 空）时，必须把 data.error.hint 透出到错误信息——否则用户只看到 "code=N, msg="（空）
// 不知道是缺了哪个字段。这是 e2e 实测发现的 dashboard/role create 报错不可读问题。
func TestBaseV3CallSurfacesErrorHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 模拟 base/v3 字段校验失败：顶层 msg 空，真正原因在 data.error.hint
		_, _ = io.WriteString(w, `{"code":1254045,"msg":"","data":{"error":{"hint":"name is required","message":"field validation failed"}}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := BaseV3Call("POST", BaseV3Path("bases", "b1", "dashboards"), nil, map[string]any{}, "u-test")
	if err == nil {
		t.Fatal("code!=0 应返回错误")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("错误信息应含 data.error.hint，实为: %v", err)
	}
}
