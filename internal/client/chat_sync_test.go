package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

func TestChatSyncProtocol(t *testing.T) {
	encoded := nativeString(nil, 1, "123")
	encoded = nativeNumber(encoded, 2, 300)
	encoded = nativeString(encoded, 1, "456")
	encoded = append(encoded, 25, 0, 0, 0, 0, 0, 0, 0, 0, 37, 0, 0, 0, 0)
	fields, err := parseNative(encoded)
	if err != nil || fields.text(1) != "123" || fields.number(2) != 300 || len(fields[1]) != 2 {
		t.Fatalf("decode = %v, %v", fields, err)
	}
	for _, malformed := range [][]byte{{0}, {8, 128}, {10, 4, 1}, {25, 1}, {37, 1}, {15}, {128}} {
		if _, err := parseNative(malformed); err == nil {
			t.Fatalf("accepted malformed protobuf: %v", malformed)
		}
	}
}

func TestValidateChatSyncOptions(t *testing.T) {
	valid := ChatSyncOptions{Name: "同步测试", FromChatID: "100", LastMessages: 3, SessionFile: filepath.Join(t.TempDir(), "missing.json"), IdempotencyKey: "test"}
	if err := ValidateChatSyncOptions(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ChatSyncOptions){
		"open-id":         func(options *ChatSyncOptions) { options.FromChatID = "oc_xxx" },
		"no-selection":    func(options *ChatSyncOptions) { options.LastMessages = 0 },
		"both-selections": func(options *ChatSyncOptions) { options.MessageIDs = []string{"101"} },
		"too-many":        func(options *ChatSyncOptions) { options.LastMessages = 51 },
		"empty-key":       func(options *ChatSyncOptions) { options.IdempotencyKey = "" },
		"duplicates":      func(options *ChatSyncOptions) { options.LastMessages = 0; options.MessageIDs = []string{"101", "101"} },
	} {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			if err := ValidateChatSyncOptions(options); err == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
}

func TestLoadNativeSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	for name, contents := range map[string]string{
		"malformed":    "not-json",
		"oauth-only":   `{"region":"feishu","user_id":"10","user_access_token":"u-test"}`,
		"wrong-region": `{"region":"other","user_id":"10","cookies":{"session":"test"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadNativeSession(path); err == nil {
				t.Fatal("accepted invalid session")
			}
		})
	}
	if err := os.WriteFile(path, []byte(`{"region":"lark","user_id":"10","cookies":{"session":"test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := loadNativeSession(path)
	if err != nil || session.baseURL != "https://internal-api-lark-api.larksuite.com" {
		t.Fatalf("session = %v, %v", session, err)
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := loadNativeSession(link); err == nil {
		t.Fatal("accepted symlink")
	}
}

type nativeSyncFixture struct {
	identity       string
	sourceChat     string
	sender         string
	createCalls    int
	unknown        bool
	badDestination bool
	missingMember  bool
}

func nativeTestMessage(chatID, messageID, sender string, position uint64, text string) []byte {
	message := nativeString(nil, 1, messageID)
	message = nativeNumber(message, 2, 4)
	message = nativeString(message, 3, sender)
	message = nativeNumber(message, 4, 1700000000+position)
	message = nativeBytes(message, 5, nativeString(nil, 1, text))
	message = nativeString(message, 10, chatID)
	message = nativeNumber(message, 13, position)
	return nativeNumber(message, 62, 1700000000000+position)
}

func nativeTestMap(data []byte, key string, value []byte) []byte {
	entry := nativeString(nil, 1, key)
	return nativeBytes(data, 1, nativeBytes(entry, 2, value))
}

func (fixture *nativeSyncFixture) session(t *testing.T) *nativeSession {
	t.Helper()
	if fixture.identity == "" {
		fixture.identity = "10"
	}
	if fixture.sourceChat == "" {
		fixture.sourceChat = "100"
	}
	if fixture.sender == "" {
		fixture.sender = "20"
	}
	transport := mockRoundTripper(func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.Header.Get("Cookie"), "session=test") {
			t.Fatal("missing session cookie")
		}
		if request.URL.Path == "/accounts/web/user" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":{"user":{"id":"%s"}}}`, fixture.identity))), Header: http.Header{}, Request: request}, nil
		}
		data, _ := io.ReadAll(request.Body)
		packet, err := parseNative(data)
		if err != nil {
			t.Fatal(err)
		}
		body, err := parseNative(packet.bytes(5))
		if err != nil {
			t.Fatal(err)
		}
		var response []byte
		switch packet.number(3) {
		case 64:
			chatID := body.text(1)
			chat := nativeString(nil, 1, chatID)
			kind := uint64(1)
			if chatID == "200" {
				kind = 2
			}
			chat = nativeNumber(chat, 2, kind)
			chat = nativeString(chat, 4, "同步测试")
			chat = nativeString(chat, 6, "10")
			chat = nativeNumber(chat, 15, 3)
			response = nativeTestMap(nil, chatID, chat)
		case 44:
			response = nativeTestMap(nil, "10", nativeString(nil, 1, "10"))
			if !(fixture.missingMember && body.text(1) == "200") {
				response = nativeTestMap(response, "20", nativeString(nil, 1, "20"))
			}
		case 8, 43:
			chatID := fixture.sourceChat
			if body.text(1) == "200" {
				chatID = "200"
			}
			for index := uint64(1); index <= 3; index++ {
				messageID := fmt.Sprint(100 + index)
				if chatID == "200" {
					messageID = fmt.Sprint(200 + index)
				}
				if packet.number(3) == 8 {
					selected := false
					for _, field := range body[1] {
						selected = selected || string(field.data) == messageID
					}
					if !selected {
						continue
					}
				}
				text := fmt.Sprintf("消息%d", index)
				if fixture.badDestination && chatID == "200" {
					text = "被复制的不同正文"
				}
				message := nativeTestMessage(chatID, messageID, fixture.sender, index, text)
				response = nativeTestMap(response, messageID, message)
			}
		case 13:
			fixture.createCalls++
			if body.number(1) != 2 || body.text(3) != "同步测试" || body.text(8) != "100" || body.number(14) != 1 || body.text(16) == "" {
				t.Fatalf("unexpected native create request: %v", body)
			}
			if len(body[6]) != 2 || len(body[9]) != 3 || string(body[9][0].data) != "101" {
				t.Fatalf("wrong member/message selection: %v", body)
			}
			if fixture.unknown {
				return nil, fmt.Errorf("simulated transport failure")
			}
			chat := nativeString(nil, 1, "200")
			chat = nativeString(chat, 4, "同步测试")
			response = nativeBytes(nil, 1, nativeString(chat, 6, "10"))
		default:
			t.Fatalf("unexpected command: %d", packet.number(3))
		}
		encoded := nativeBytes(nativeNumber(nil, 4, 0), 5, response)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(encoded)), Header: http.Header{}, Request: request}, nil
	})
	return &nativeSession{Region: "feishu", UserID: "10", Cookies: map[string]string{"session": "test"}, baseURL: "https://internal-api-lark-api.feishu.cn", httpClient: config.NewHTTPClientWithTransport(transport, 0)}
}

func nativeTestOptions(t *testing.T) ChatSyncOptions {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FEISHU_PROFILE", "")
	return ChatSyncOptions{Name: "同步测试", FromChatID: "100", LastMessages: 3, IdempotencyKey: "test", SessionFile: filepath.Join(t.TempDir(), "session.json")}
}

func TestPrepareNativeChatSyncBoundaries(t *testing.T) {
	for name, fixture := range map[string]*nativeSyncFixture{
		"identity": {identity: "30"}, "wrong-chat": {sourceChat: "999"}, "wrong-sender": {sender: "30"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fixture.session(t).prepareChatSync(nativeTestOptions(t)); err == nil {
				t.Fatal("accepted unsafe source")
			}
			if fixture.createCalls != 0 {
				t.Fatal("created during preview")
			}
		})
	}
	fixture := &nativeSyncFixture{}
	options := nativeTestOptions(t)
	options.LastMessages = 0
	options.MessageIDs = []string{"103", "101"}
	plan, err := fixture.session(t).prepareChatSync(options)
	if err != nil || len(plan.Messages) != 2 || plan.Messages[0].ID != "101" || plan.Messages[1].ID != "103" {
		t.Fatalf("explicit selection = %v, %v", plan, err)
	}
}

func TestCreateNativeChatSyncIdempotency(t *testing.T) {
	fixture := &nativeSyncFixture{}
	session := fixture.session(t)
	options := nativeTestOptions(t)
	plan, err := session.prepareChatSync(options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.createChatWithHistory(options, plan)
	if err != nil || !result.Verified || result.Replayed || len(result.MessageIDs) != 3 {
		t.Fatalf("create = %v, %v", result, err)
	}
	result, err = session.createChatWithHistory(options, plan)
	if err != nil || !result.Verified || !result.Replayed || fixture.createCalls != 1 {
		t.Fatalf("replay = %v, %v, calls=%d", result, err, fixture.createCalls)
	}
	plan.Messages[0].Content = nativeString(nil, 1, "changed")
	if _, err := session.createChatWithHistory(options, plan); err == nil || fixture.createCalls != 1 {
		t.Fatal("accepted changed plan")
	}
}

func TestCreateNativeChatSyncUnknownResult(t *testing.T) {
	fixture := &nativeSyncFixture{unknown: true}
	session := fixture.session(t)
	options := nativeTestOptions(t)
	plan, err := session.prepareChatSync(options)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := session.createChatWithHistory(options, plan); err == nil {
			t.Fatal("accepted unknown result")
		}
	}
	if fixture.createCalls != 1 {
		t.Fatalf("retried write: %d", fixture.createCalls)
	}
	files, _ := filepath.Glob(filepath.Join(os.Getenv("HOME"), ".feishu-cli", "native-chat-sync", "*.json"))
	if len(files) != 1 {
		t.Fatalf("missing durable journal: %v", files)
	}
	data, _ := os.ReadFile(files[0])
	var state chatSyncState
	if err := json.Unmarshal(data, &state); err != nil || state.Status != "request_sent" {
		t.Fatalf("journal = %v, %v", state, err)
	}
}

func TestCreateNativeChatSyncFailedReadback(t *testing.T) {
	for name, fixture := range map[string]*nativeSyncFixture{"wrong-body": {badDestination: true}, "missing-member": {missingMember: true}} {
		t.Run(name, func(t *testing.T) {
			session := fixture.session(t)
			options := nativeTestOptions(t)
			plan, err := session.prepareChatSync(options)
			if err != nil {
				t.Fatal(err)
			}
			result, err := session.createChatWithHistory(options, plan)
			if err == nil || result == nil || result.Verified || fixture.createCalls != 1 {
				t.Fatalf("readback = %v, %v", result, err)
			}
		})
	}
}

func TestNativeChatSyncRejectsRedirects(t *testing.T) {
	calls := 0
	transport := mockRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://open.feishu.cn/other"}}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})
	session := &nativeSession{baseURL: "https://internal-api-lark-api.feishu.cn", Cookies: map[string]string{"session": "test"}, httpClient: config.NewHTTPClientWithTransport(transport, 0)}
	if _, err := session.request(http.MethodGet, "/accounts/web/user", nil, 0); err == nil || calls != 1 {
		t.Fatalf("redirect followed: calls=%d, err=%v", calls, err)
	}
}
