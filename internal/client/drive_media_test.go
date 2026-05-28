package client

import (
	"net/http"
	"testing"
)

// TestParseDownloadJSONError 验证 download HTTP 200 响应的业务错误体识别（只信 Content-Type）：
// Content-Type 为 application/json 且 code!=0 判为业务错误；其它（octet-stream / 无 CT）一律当
// 二进制不判错——避免把内容恰为 {code:N} 的合法文件误判。
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
		{"json CT + code!=0 → 业务错误", jsonCT, []byte(`{"code":1254043,"msg":"permission denied"}`), true, 1254043},
		{"json CT + code=0 → 不算错误", jsonCT, []byte(`{"code":0,"msg":"ok"}`), false, 0},
		{"json CT 但 parse 失败 → 当二进制", jsonCT, []byte(`not-json`), false, 0},
		{"二进制 octet-stream", octetCT, []byte{0x89, 0x50, 0x4e, 0x47}, false, 0},
		{"无 header 的纯二进制", nil, []byte{0x00, 0x01, 0x02}, false, 0},
		{"octet CT + { 开头无 code → 不误判", octetCT, []byte(`{"name":"foo","value":42}`), false, 0},
		// 只信 CT：无 Content-Type 时即使内容 {code!=0} 也不判错（不再用 RawBody 前缀辅助）
		{"无 CT + { 开头 + code!=0 → 不判错（只信 CT）", nil, []byte(`{"code":99991663,"msg":"x"}`), false, 0},
		// 合法 .json 文件带 octet CT + {code!=0} 内容 → 不误判（CT 非 application/json）
		{"octet CT + code!=0 的 JSON 文件 → 不误判", octetCT, []byte(`{"code":1,"msg":"domain data"}`), false, 0},
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
