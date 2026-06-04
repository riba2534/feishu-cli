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
		"cli_xxxx":             "cli_xxxx",
		"cli_../../etc/passwd": "cli_etcpasswd",
		"cli_/abs/path":        "cli_abspath",
		"":                     "unknown",
		"  ":                   "unknown",
		"cli-test-app":         "cli-test-app",
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
	if !containsString(def.Scopes, "im:message.p2p_msg:readonly") {
		t.Fatalf("im.message.receive_v1 scopes = %v, want im:message.p2p_msg:readonly", def.Scopes)
	}
	if !containsString(def.AuthTypes, "bot") {
		t.Fatalf("im.message.receive_v1 auth_types = %v, want bot", def.AuthTypes)
	}
	if !containsString(def.RequiredConsoleEvents, "im.message.receive_v1") {
		t.Fatalf("im.message.receive_v1 console events = %v, want im.message.receive_v1", def.RequiredConsoleEvents)
	}
}

func TestIMKeyDefinitionsIncludeOfficialMetadata(t *testing.T) {
	for _, def := range ListAll() {
		if !strings.HasPrefix(def.Key, "im.") {
			continue
		}
		if !containsString(def.AuthTypes, "bot") {
			t.Errorf("%s AuthTypes = %v, want bot", def.Key, def.AuthTypes)
		}
		if !containsString(def.RequiredConsoleEvents, def.EventType) {
			t.Errorf("%s RequiredConsoleEvents = %v, want %s", def.Key, def.RequiredConsoleEvents, def.EventType)
		}
	}
}

func TestValidateDotPathExpr(t *testing.T) {
	valid := []string{"", ".", ".event", ".event.message", ".event.message_id", ".event.message-type"}
	for _, expr := range valid {
		if err := ValidateDotPathExpr(expr); err != nil {
			t.Errorf("ValidateDotPathExpr(%q) unexpected error: %v", expr, err)
		}
	}
	invalid := []string{"event", ".event[0]", ".event | .header", ".event..message", ".event.message."}
	for _, expr := range invalid {
		if err := ValidateDotPathExpr(expr); err == nil {
			t.Errorf("ValidateDotPathExpr(%q) expected error", expr)
		}
	}
}

func TestValidateOutputDir(t *testing.T) {
	valid := []string{"", ".", "./events", "events/today"}
	for _, dir := range valid {
		if err := ValidateOutputDir(dir); err != nil {
			t.Errorf("ValidateOutputDir(%q) unexpected error: %v", dir, err)
		}
	}
	invalid := []string{"~/events", "/tmp/events", "../events", "events/../outside"}
	for _, dir := range invalid {
		if err := ValidateOutputDir(dir); err == nil {
			t.Errorf("ValidateOutputDir(%q) expected error", dir)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
