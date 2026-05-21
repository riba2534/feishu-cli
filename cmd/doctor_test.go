package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestParseOnly(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
		nilMap   bool
	}{
		{"", nil, true},
		{"   ", nil, true},
		{"user_token", []string{"user_token"}, false},
		{"user_token,endpoint_open", []string{"user_token", "endpoint_open"}, false},
		{" user_token , endpoint_open ", []string{"user_token", "endpoint_open"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseOnly(tc.input)
			if err != nil {
				t.Fatalf("parseOnly(%q) unexpected error: %v", tc.input, err)
			}
			if tc.nilMap {
				if got != nil {
					t.Errorf("parseOnly(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if len(got) != len(tc.expected) {
				t.Errorf("parseOnly(%q) size = %d, want %d", tc.input, len(got), len(tc.expected))
			}
			for _, name := range tc.expected {
				if !got[name] {
					t.Errorf("parseOnly(%q) missing %q", tc.input, name)
				}
			}
		})
	}
}

// TestParseOnlyRejectsUnknown 验证 --only 包含未知 check 名时报错（修复 codex P2 finding）
func TestParseOnlyRejectsUnknown(t *testing.T) {
	cases := []string{"user_tokn", "user_token,unknown_check", "totally_made_up"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := parseOnly(in)
			if err == nil {
				t.Errorf("parseOnly(%q) should return error for unknown check name", in)
			}
		})
	}
}

// TestRedactProxyURLStripsUserinfo 验证 redactProxyURL 把所有形态的 userinfo
// （user+password / token-only / username-only）统一替换成 ***，凭证字符串不残留。
// v1 PR 三轮 rv 加固：早期实现只在 has-password 时 mask，token-only
// （`https://abc123@proxy.example`）和 username-only 会原样泄露到 doctor 输出。
func TestRedactProxyURLStripsUserinfo(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantMask  bool     // 输出应包含 ***
		wantHost  string   // 输出应保留的 host[:port]
		forbidden []string // 输出绝不能包含的子串（凭证残留）
	}{
		{
			name:      "user+password",
			in:        "https://user:secret123@proxy.example",
			wantMask:  true,
			wantHost:  "proxy.example",
			forbidden: []string{"secret123", "user:", ":secret"},
		},
		{
			name:      "token only (no colon)",
			in:        "https://abc123def456@proxy.example",
			wantMask:  true,
			wantHost:  "proxy.example",
			forbidden: []string{"abc123def456"},
		},
		{
			name:      "username only",
			in:        "https://user@proxy.example",
			wantMask:  true,
			wantHost:  "proxy.example",
			forbidden: []string{"user@"},
		},
		{
			// 关键回归：密码含裸 @ 时必须按 authority **最后一个** @ 分隔（RFC + Go net/url 标准）
			// 早期实现用 IndexAny 取第一个 @，会切出 userinfo="user:p" + 错误"host"="ssword@proxy.example"，
			// 输出 "https://***@ssword@proxy.example/path@q" 半泄密码——三轮 rv codex 抓到的真 bug
			name:      "password with literal @ (authority uses last @)",
			in:        "https://user:p@ssword@proxy.example/path@q",
			wantMask:  true,
			wantHost:  "proxy.example/path@q",
			forbidden: []string{"p@ssword", "ssword"},
		},
		{
			name:      "IPv6 host with userinfo",
			in:        "https://user:secret@[::1]:8080/path",
			wantMask:  true,
			wantHost:  "[::1]:8080/path",
			forbidden: []string{"secret"},
		},
		{
			name:     "IPv6 host without userinfo",
			in:       "https://[::1]:8080",
			wantMask: false,
			wantHost: "[::1]:8080",
		},
		{
			name:     "no userinfo (host:port only)",
			in:       "https://proxy.example:8080",
			wantMask: false,
			wantHost: "proxy.example:8080",
		},
		{
			// 路径里的 @ 不算 userinfo 分隔符；纯 host 无 userinfo 时不该 mask
			name:     "@ in path only",
			in:       "https://proxy.example/path@q?x=y@z",
			wantMask: false,
			wantHost: "proxy.example/path@q?x=y@z",
		},
		{
			// 含 userinfo 但 host 部分 malformed (`[::1` 缺右括号) 仍 best-effort mask；
			// 不能因为 url.Parse 会失败就放弃遮蔽——defense-in-depth
			name:      "malformed host with userinfo still masked",
			in:        "https://user:secret@[::1",
			wantMask:  true,
			wantHost:  "[::1",
			forbidden: []string{"secret"},
		},
		{
			name:     "empty input returns empty",
			in:       "",
			wantMask: false,
			wantHost: "",
		},
		{
			name:     "no scheme (not a URL form)",
			in:       "not a url at all",
			wantMask: false,
			wantHost: "not a url at all",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := redactProxyURL(tc.in)
			if tc.wantMask && !strings.Contains(out, "***") {
				t.Errorf("redactProxyURL(%q) = %q, want *** mask", tc.in, out)
			}
			if !tc.wantMask && strings.Contains(out, "***") {
				t.Errorf("redactProxyURL(%q) = %q, should not contain *** mask", tc.in, out)
			}
			if tc.wantHost != "" && !strings.Contains(out, tc.wantHost) {
				t.Errorf("redactProxyURL(%q) = %q lost host %q", tc.in, out, tc.wantHost)
			}
			for _, f := range tc.forbidden {
				if strings.Contains(out, f) {
					t.Errorf("redactProxyURL(%q) = %q leaked credential substring %q", tc.in, out, f)
				}
			}
		})
	}
}

func TestShouldRun(t *testing.T) {
	if !shouldRun("user_token", nil) {
		t.Error("shouldRun(_, nil) should always be true")
	}
	only := map[string]bool{"user_token": true}
	if !shouldRun("user_token", only) {
		t.Error("shouldRun in only should be true")
	}
	if shouldRun("endpoint_open", only) {
		t.Error("shouldRun not in only should be false")
	}
}

func TestCheckProxy_NoProxy(t *testing.T) {
	// 清掉所有 proxy env
	envs := []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"}
	saved := make(map[string]string)
	for _, e := range envs {
		saved[e] = os.Getenv(e)
		os.Unsetenv(e)
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	r := checkProxy()
	if r.Status != "pass" {
		t.Errorf("无代理时应 pass, got %s: %s", r.Status, r.Message)
	}
}

func TestCheckProxy_WithProxyMissingNoProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")

	r := checkProxy()
	if r.Status != "warn" {
		t.Errorf("有代理但 NO_PROXY 缺飞书域应 warn, got %s: %s", r.Status, r.Message)
	}
	if !strings.Contains(r.Hint, "feishu.cn") {
		t.Errorf("hint 应提到 feishu.cn, got: %s", r.Hint)
	}
}

func TestCheckProxy_WithProxyAndCorrectNoProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
	t.Setenv("NO_PROXY", "localhost,*.feishu.cn,*.larkoffice.com,*.larksuite.com")

	r := checkProxy()
	if r.Status != "pass" {
		t.Errorf("NO_PROXY 包含飞书域应 pass, got %s: %s", r.Status, r.Message)
	}
}

// TestCheckProxy_NoProxyFormats 验证 NO_PROXY 多种合法书写格式都被识别为"已覆盖飞书域"。
// v1 PR 三轮 rv 加固：早期实现用 strings.Contains 子串匹配，把 `feishu.cn`（无前导点）
// 误判为缺失（因为找不到 `.feishu.cn`）。fix 改用按逗号 split + 精确 + suffix 比对，
// 这里覆盖 4 种实际用户会写的格式。
func TestCheckProxy_NoProxyFormats(t *testing.T) {
	cases := []struct {
		name    string
		noProxy string
	}{
		{"bare domain", "feishu.cn,larkoffice.com,larksuite.com"},
		{"leading dot", ".feishu.cn,.larkoffice.com,.larksuite.com"},
		{"with port", "feishu.cn:443,larkoffice.com:443,larksuite.com:443"},
		{"mixed whitespace", "feishu.cn , .larkoffice.com ,larksuite.com:443"},
		// Go net/http httpproxy 标准：单独 `*` 表示所有请求不走代理；早期 noProxyCovers 不识别
		{"asterisk wildcard", "*"},
		{"asterisk among others", "localhost,*,127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7890")
			t.Setenv("NO_PROXY", tc.noProxy)
			r := checkProxy()
			if r.Status != "pass" {
				t.Errorf("NO_PROXY=%q 应被识别为已覆盖飞书域，got %s: %s (hint: %s)",
					tc.noProxy, r.Status, r.Message, r.Hint)
			}
		})
	}
}

func TestCheckDependencies(t *testing.T) {
	r := checkDependencies()
	if r.Status != "pass" {
		t.Errorf("dependencies 检查应总是 pass, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "go=") {
		t.Errorf("message 应含 go 版本, got: %s", r.Message)
	}
}

func TestCheckResultHelpers(t *testing.T) {
	if checkPass("x", "m").Status != "pass" {
		t.Error("checkPass status mismatch")
	}
	if checkFail("x", "m", "h").Status != "fail" {
		t.Error("checkFail status mismatch")
	}
	if checkFail("x", "m", "h").Hint != "h" {
		t.Error("checkFail hint mismatch")
	}
	if checkWarn("x", "m", "h").Status != "warn" {
		t.Error("checkWarn status mismatch")
	}
	if checkSkip("x", "m").Status != "skip" {
		t.Error("checkSkip status mismatch")
	}
}
