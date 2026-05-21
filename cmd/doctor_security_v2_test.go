package cmd

import (
	"strings"
	"testing"
)

// TestRedactProxyURLMasksUsernameOnly 验证 token-only / username-only userinfo 也被 mask
// (codex 二轮 rv finding 1)
func TestRedactProxyURLMasksUsernameOnly(t *testing.T) {
	cases := []struct {
		in       string
		mustMask []string // must NOT appear in output
	}{
		{"https://secrettoken@proxy.example", []string{"secrettoken"}},
		{"https://user-only@proxy.example", []string{"user-only"}},
		{"https://user:secret@proxy.example", []string{"secret", "user"}}, // user:pass 两边都不留
	}
	for _, tc := range cases {
		out := redactProxyURL(tc.in)
		for _, leak := range tc.mustMask {
			if strings.Contains(out, leak) {
				t.Errorf("redactProxyURL(%q) = %q leaked %q", tc.in, out, leak)
			}
		}
		if !strings.Contains(out, "proxy.example") {
			t.Errorf("redactProxyURL(%q) = %q lost host", tc.in, out)
		}
	}
}

// TestNoProxyCovers 验证按 entry 解析支持精确匹配 + suffix 匹配
// (codex 二轮 rv finding 2)
func TestNoProxyCovers(t *testing.T) {
	entries := splitNoProxyEntries("feishu.cn,larkoffice.com,.larksuite.com")
	cases := []struct {
		domain string
		want   bool
	}{
		{"feishu.cn", true},     // 精确
		{"larksuite.com", true}, // 前导 . 去掉后精确
		{"larkoffice.com", true},
		{"google.com", false},
	}
	for _, tc := range cases {
		got := noProxyCovers(entries, tc.domain)
		if got != tc.want {
			t.Errorf("noProxyCovers(%v, %q) = %v, want %v", entries, tc.domain, got, tc.want)
		}
	}
}

// TestSplitNoProxyEntriesNormalizesPort 验证端口被剥离
func TestSplitNoProxyEntriesNormalizesPort(t *testing.T) {
	entries := splitNoProxyEntries("feishu.cn:443, .larkoffice.com , localhost")
	want := map[string]bool{"feishu.cn": true, "larkoffice.com": true, "localhost": true}
	for _, e := range entries {
		if !want[e] {
			t.Errorf("unexpected entry %q in %v", e, entries)
		}
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(entries), entries)
	}
}
