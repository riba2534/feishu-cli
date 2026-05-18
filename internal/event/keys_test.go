package event

import (
	"strings"
	"testing"
)

func TestListAll_NotEmpty(t *testing.T) {
	all := ListAll()
	if len(all) == 0 {
		t.Fatal("ListAll() 返回空，至少应包含 im.message.receive_v1")
	}
}

func TestListAll_AllDomainsSet(t *testing.T) {
	for _, def := range ListAll() {
		if def.Key == "" {
			t.Errorf("EventKey 缺少 Key: %+v", def)
		}
		if def.EventType == "" {
			t.Errorf("EventKey %s 缺少 EventType", def.Key)
		}
		if def.Domain == "" {
			t.Errorf("EventKey %s 缺少 Domain", def.Key)
		}
		if def.Description == "" {
			t.Errorf("EventKey %s 缺少 Description", def.Key)
		}
	}
}

func TestLookup_KnownKey(t *testing.T) {
	def, ok := Lookup("im.message.receive_v1")
	if !ok {
		t.Fatal("Lookup(im.message.receive_v1) 应返回 true")
	}
	if def.EventType != "im.message.receive_v1" {
		t.Errorf("EventType 期望 im.message.receive_v1，实际 %q", def.EventType)
	}
	if def.Domain != "im" {
		t.Errorf("Domain 期望 im，实际 %q", def.Domain)
	}
}

func TestLookup_UnknownKey(t *testing.T) {
	_, ok := Lookup("does.not.exist_v999")
	if ok {
		t.Fatal("Lookup 对未知 key 应返回 false")
	}
}

func TestDomains_Unique(t *testing.T) {
	domains := Domains()
	seen := map[string]bool{}
	for _, d := range domains {
		if seen[d] {
			t.Errorf("Domain %q 重复出现", d)
		}
		seen[d] = true
	}
	// 至少应有 im / contact / calendar 三个 domain
	for _, must := range []string{"im", "contact", "calendar"} {
		if !seen[must] {
			t.Errorf("Domains() 缺少必备 domain %q", must)
		}
	}
}

func TestSanitizeAppID_RejectsBadChars(t *testing.T) {
	cases := map[string]string{
		"cli_a77d84747fa6500b":  "cli_a77d84747fa6500b",
		"cli_../../etc/passwd": "cli_etcpasswd",
		"cli_/abs/path":         "cli_abspath",
		"":                      "unknown",
		"  ":                    "unknown",
		"cli-test-app":          "cli-test-app",
	}
	for in, want := range cases {
		got := sanitizeAppID(in)
		if got != want {
			t.Errorf("sanitizeAppID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyDefinition_ScopesContainsExpected(t *testing.T) {
	def, _ := Lookup("im.message.receive_v1")
	if len(def.Scopes) == 0 {
		t.Fatal("im.message.receive_v1 应至少有一个 scope")
	}
	hasIm := false
	for _, s := range def.Scopes {
		if strings.HasPrefix(s, "im:") {
			hasIm = true
		}
	}
	if !hasIm {
		t.Errorf("im.message.receive_v1 至少应有一个 im: 开头的 scope，实际 %v", def.Scopes)
	}
}
