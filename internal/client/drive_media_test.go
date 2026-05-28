package client

import (
	"net/http"
	"testing"
)

// TestParseDownloadJSONError 验证 download HTTP 200 响应的业务错误体识别：
// JSON 错误体（code!=0）应判为错误；正常二进制（含恰好以 '{' 开头的 JSON 文件）不应误判。
func TestParseDownloadJSONError(t *testing.T) {
	jsonCT := http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}
	octetCT := http.Header{"Content-Type": []string{"application/octet-stream"}}

	cases := []struct {
		name     string
		header   http.Header
		body     []byte
		wantErr  bool
		wantCode int
	}{
		{"JSON 错误体 + json CT", jsonCT, []byte(`{"code":1254043,"msg":"permission denied"}`), true, 1254043},
		{"JSON 错误体 code=0 不算错误", jsonCT, []byte(`{"code":0,"msg":"ok"}`), false, 0},
		{"json CT 但 parse 失败 → 保守当二进制", jsonCT, []byte(`not-json`), false, 0},
		{"二进制 octet-stream", octetCT, []byte{0x89, 0x50, 0x4e, 0x47}, false, 0},
		{"无 header 的纯二进制", nil, []byte{0x00, 0x01, 0x02}, false, 0},
		// 关键边界：恰好以 '{' 开头的二进制 JSON 文件（无 json CT），code=0/无 code → 不误判
		{"以 { 开头的 JSON 文件无 code 字段", octetCT, []byte(`{"name":"foo","value":42}`), false, 0},
		{"无 CT 但 { 开头且 code!=0 → 辅助判错", nil, []byte(`{"code":99991663,"msg":"token invalid"}`), true, 99991663},
		{"无 CT 且 { 开头但 code=0 → 不误判", nil, []byte(`{"code":0,"data":"x"}`), false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, isErr := parseDownloadJSONError(tc.header, tc.body)
			if isErr != tc.wantErr {
				t.Fatalf("isErr = %v, want %v (code=%d)", isErr, tc.wantErr, code)
			}
			if tc.wantErr && code != tc.wantCode {
				t.Errorf("code = %d, want %d", code, tc.wantCode)
			}
		})
	}
}

func TestBuildDownloadMediaExtra(t *testing.T) {
	tests := []struct {
		name string
		opts DownloadMediaOptions
		want string
	}{
		{
			name: "empty options",
			opts: DownloadMediaOptions{},
			want: "",
		},
		{
			name: "doc token defaults to docx",
			opts: DownloadMediaOptions{DocToken: "doc_token_123"},
			want: `{"doc_token":"doc_token_123","doc_type":"docx"}`,
		},
		{
			name: "doc type can be overridden",
			opts: DownloadMediaOptions{DocToken: "doc_token_123", DocType: "doc"},
			want: `{"doc_token":"doc_token_123","doc_type":"doc"}`,
		},
		{
			name: "raw extra wins",
			opts: DownloadMediaOptions{
				DocToken: "doc_token_123",
				DocType:  "docx",
				Extra:    `{"doc_token":"override","doc_type":"docx"}`,
			},
			want: `{"doc_token":"override","doc_type":"docx"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDownloadMediaExtra(tt.opts); got != tt.want {
				t.Fatalf("buildDownloadMediaExtra() = %q, want %q", got, tt.want)
			}
		})
	}
}
