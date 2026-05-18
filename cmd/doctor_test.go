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

// TestRedactProxyURLStripsPassword 验证 redactProxyURL 去掉 userinfo password（修复 codex P2 finding）
func TestRedactProxyURLStripsPassword(t *testing.T) {
	tests := []struct {
		in           string
		mustMask     bool
		mustKeepHost string
	}{
		{"https://user:secret123@proxy.example", true, "proxy.example"},
		{"https://user@proxy.example", false, "proxy.example"},
		{"https://proxy.example:8080", false, "proxy.example"},
		{"", false, ""},
	}
	for _, tc := range tests {
		out := redactProxyURL(tc.in)
		if tc.mustMask {
			if strings.Contains(out, "secret123") {
				t.Errorf("redactProxyURL(%q) = %q still contains secret123", tc.in, out)
			}
		}
		if tc.mustKeepHost != "" && !strings.Contains(out, tc.mustKeepHost) {
			t.Errorf("redactProxyURL(%q) = %q lost host %q", tc.in, out, tc.mustKeepHost)
		}
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
