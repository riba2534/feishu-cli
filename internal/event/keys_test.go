package event

import (
	"os"
	"path/filepath"
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

// 相对路径同样可能落进敏感目录（cwd 为家目录时的 .ssh），按 safefile 拒绝名单拒绝。
func TestValidateOutputDirRejectsSensitiveRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	for _, dir := range []string{".ssh", ".ssh/events", "./.feishu-cli/events"} {
		if err := ValidateOutputDir(dir); err == nil || !strings.Contains(err.Error(), "敏感目录") {
			t.Errorf("ValidateOutputDir(%q) 应拒绝敏感目录，得到 %v", dir, err)
		}
	}
	if err := ValidateOutputDir("events"); err != nil {
		t.Errorf("普通相对目录应放行: %v", err)
	}
}

func TestVCEventKeyCatalogOfficial(t *testing.T) {
	for _, old := range []string{"vc.meeting.meeting_started_v1", "vc.meeting.meeting_ended_v1"} {
		if _, ok := Lookup(old); ok {
			t.Errorf("旧 EventKey %s 应替换为 participant_meeting_* / note / recording", old)
		}
	}

	type want struct {
		scope   string
		subPath string
		unsub   string
	}
	required := map[string]want{
		"vc.meeting.participant_meeting_started_v1": {
			scope: "vc:meeting.meetingevent:read", subPath: "/open-apis/vc/v1/meetings/subscription", unsub: "/open-apis/vc/v1/meetings/unsubscription",
		},
		"vc.meeting.participant_meeting_joined_v1": {
			scope: "vc:meeting.meetingevent:read", subPath: "/open-apis/vc/v1/meetings/subscription", unsub: "/open-apis/vc/v1/meetings/unsubscription",
		},
		"vc.meeting.participant_meeting_ended_v1": {
			scope: "vc:meeting.meetingevent:read", subPath: "/open-apis/vc/v1/meetings/subscription", unsub: "/open-apis/vc/v1/meetings/unsubscription",
		},
		"vc.note.generated_v1": {
			scope: "vc:note:read", subPath: "/open-apis/vc/v1/notes/subscription", unsub: "/open-apis/vc/v1/notes/unsubscription",
		},
		"vc.recording.recording_started_v1": {
			scope: "vc:recording:read", subPath: "/open-apis/vc/v1/recordings/subscription", unsub: "/open-apis/vc/v1/recordings/unsubscription",
		},
		"vc.recording.recording_transcript_generated_v1": {
			scope: "vc:recording:read", subPath: "/open-apis/vc/v1/recordings/subscription", unsub: "/open-apis/vc/v1/recordings/unsubscription",
		},
		"vc.recording.recording_ended_v1": {
			scope: "vc:recording:read", subPath: "/open-apis/vc/v1/recordings/subscription", unsub: "/open-apis/vc/v1/recordings/unsubscription",
		},
	}
	for key, w := range required {
		def, ok := Lookup(key)
		if !ok {
			t.Errorf("缺少官方 EventKey %s", key)
			continue
		}
		if def.EventType != key {
			t.Errorf("%s EventType = %q", key, def.EventType)
		}
		if def.Domain != "vc" {
			t.Errorf("%s Domain = %q, want vc", key, def.Domain)
		}
		if !containsString(def.Scopes, w.scope) {
			t.Errorf("%s Scopes = %v, want %s", key, def.Scopes, w.scope)
		}
		if !containsString(def.AuthTypes, "user") {
			t.Errorf("%s AuthTypes = %v, want user", key, def.AuthTypes)
		}
		if !containsString(def.RequiredConsoleEvents, key) {
			t.Errorf("%s RequiredConsoleEvents = %v", key, def.RequiredConsoleEvents)
		}
		if def.SubscribePath != w.subPath {
			t.Errorf("%s SubscribePath = %q, want %q", key, def.SubscribePath, w.subPath)
		}
		if def.UnsubscribePath != w.unsub {
			t.Errorf("%s UnsubscribePath = %q, want %q", key, def.UnsubscribePath, w.unsub)
		}
		if !def.SubscribeEventType {
			t.Errorf("%s 应以 event_type 做 User pre-consume", key)
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
