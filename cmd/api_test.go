package cmd

import (
	"strings"
	"testing"
)

func TestNormalizeAPIPath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantPath  string
		wantQuery map[string]string
		wantErr   bool
	}{
		{
			name:     "短路径自动补 /open-apis/",
			input:    "/im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:     "已有 /open-apis/ 前缀保持原样",
			input:    "/open-apis/im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:     "完整 URL 自动剥前缀",
			input:    "https://open.feishu.cn/open-apis/authen/v1/user_info",
			wantPath: "/open-apis/authen/v1/user_info",
		},
		{
			name:     "larksuite.com 前缀也剥",
			input:    "https://open.larksuite.com/open-apis/contact/v3/users",
			wantPath: "/open-apis/contact/v3/users",
		},
		{
			name:     "larkoffice.com 前缀也剥",
			input:    "https://open.larkoffice.com/open-apis/im/v1/chats",
			wantPath: "/open-apis/im/v1/chats",
		},
		{
			name:     "缺斜杠自动补",
			input:    "im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:      "path 内嵌 query 自动拆解",
			input:     "/open-apis/foo?a=1&b=2",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:      "完整 URL + query 同时处理",
			input:     "https://open.feishu.cn/open-apis/foo?x=y",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"x": "y"},
		},
		{
			name:     "fragment 被忽略",
			input:    "/open-apis/foo#section",
			wantPath: "/open-apis/foo",
		},
		{
			name:    "空字符串报错",
			input:   "",
			wantErr: true,
		},
		{
			name:    "纯空白报错",
			input:   "   ",
			wantErr: true,
		},
		{
			name:     "path 中含真实 ID（如 message_id）保持原样",
			input:    "/open-apis/im/v1/messages/om_xxxxxx",
			wantPath: "/open-apis/im/v1/messages/om_xxxxxx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, q, err := normalizeAPIPath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 err，实际 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期 err: %v", err)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q，期望 %q", path, tc.wantPath)
			}
			for k, v := range tc.wantQuery {
				if got := q.Get(k); got != v {
					t.Errorf("query[%s] = %q，期望 %q", k, got, v)
				}
			}
		})
	}
}

func TestParseQueryParams(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{"空字符串", "", map[string]string{}, false},
		{"纯空白", "   ", map[string]string{}, false},
		{"简单字符串", `{"a":"1","b":"x"}`, map[string]string{"a": "1", "b": "x"}, false},
		{"整数自动转字符串", `{"page_size":100}`, map[string]string{"page_size": "100"}, false},
		{"小数保留", `{"x":1.5}`, map[string]string{"x": "1.5"}, false},
		{"布尔值", `{"flag":true}`, map[string]string{"flag": "true"}, false},
		{"null 被跳过", `{"a":"1","b":null}`, map[string]string{"a": "1"}, false},
		{"嵌套对象序列化为 JSON", `{"obj":{"k":"v"}}`, map[string]string{"obj": `{"k":"v"}`}, false},
		{"非 JSON 报错", `not-json`, nil, true},
		{"数组而非对象报错", `[1,2]`, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseQueryParams(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 err，实际 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期 err: %v", err)
			}
			for k, want := range tc.want {
				if got := q.Get(k); got != want {
					t.Errorf("query[%s] = %q，期望 %q", k, got, want)
				}
			}
			// 检查多余的 key（null 应被跳过）
			if tc.name == "null 被跳过" {
				if _, exists := q["b"]; exists {
					t.Errorf("key b 不应存在（null 应被跳过）")
				}
			}
		})
	}
}

func TestParseQueryParamsArray(t *testing.T) {
	q, err := parseQueryParams(`{"fields":["name","email","mobile"]}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := q["fields"]
	want := []string{"name", "email", "mobile"}
	if len(got) != len(want) {
		t.Fatalf("len = %d，期望 %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("fields[%d] = %q，期望 %q", i, got[i], v)
		}
	}
}

func TestIsValidHTTPMethod(t *testing.T) {
	valid := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, m := range valid {
		if !isValidHTTPMethod(m) {
			t.Errorf("isValidHTTPMethod(%q) = false，期望 true", m)
		}
	}
	invalid := []string{"BOGUS", "get", "", "HEAD", "OPTIONS"} // 小写在 caller 处 ToUpper 后才到这
	for _, m := range invalid {
		if isValidHTTPMethod(m) {
			t.Errorf("isValidHTTPMethod(%q) = true，期望 false", m)
		}
	}
}

func TestDetectFeishuBizError(t *testing.T) {
	tests := []struct {
		name       string
		body       []byte
		wantHint   string // 期望 hint 包含此子串
		wantNohint bool   // 期望返回空字符串
	}{
		{
			name:       "code 0 不提示",
			body:       []byte(`{"code":0,"msg":"success"}`),
			wantNohint: true,
		},
		{
			name:       "非 JSON 不提示",
			body:       []byte(`<html>nope</html>`),
			wantNohint: true,
		},
		{
			name:     "scope 不足 99991679 给登录提示",
			body:     []byte(`{"code":99991679,"msg":"Unauthorized."}`),
			wantHint: "auth login",
		},
		{
			name:     "限流 99991400 给限流提示",
			body:     []byte(`{"code":99991400,"msg":"frequency limit"}`),
			wantHint: "限流",
		},
		{
			name:     "外部群 232033 指向现有排错文档",
			body:     []byte(`{"code":232033,"msg":"forbidden"}`),
			wantHint: "skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md",
		},
		{
			name:     "未知 code 至少打印 code+msg",
			body:     []byte(`{"code":12345,"msg":"unknown"}`),
			wantHint: "code=12345",
		},
		{
			name:       "空 body 不提示",
			body:       []byte{},
			wantNohint: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectFeishuBizError(200, tc.body)
			if tc.wantNohint {
				if got != "" {
					t.Errorf("期望空字符串，得到 %q", got)
				}
				return
			}
			if got == "" {
				t.Errorf("期望 hint 包含 %q，得到空", tc.wantHint)
				return
			}
			if !strings.Contains(got, tc.wantHint) {
				t.Errorf("hint = %q，期望包含 %q", got, tc.wantHint)
			}
		})
	}
}
