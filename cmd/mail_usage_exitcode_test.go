package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// mailStub 假邮箱服务：记录业务请求（不含 tenant token 换取），按路径返回最小成功响应。
type mailStub struct {
	mu      sync.Mutex
	queries []string // "METHOD path?query body"
}

func (s *mailStub) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queries...)
}

func startMailStub(t *testing.T) (*mailStub, string) {
	t.Helper()
	stub := &mailStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		stub.mu.Lock()
		stub.queries = append(stub.queries, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		stub.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages/batch_get"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"messages":[{"message_id":"m1","subject":"s"}]}}`)
		case strings.Contains(r.URL.Path, "/threads/"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"thread":{"thread_id":"th_1","messages":[{"message_id":"m1","subject":"s","internal_date":"1"}]}}}`)
		case strings.Contains(r.URL.Path, "/messages/"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"message":{"message_id":"m1","subject":"s"}}}`)
		default:
			_, _ = fmt.Fprint(w, `{"code":0,"data":{}}`)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	return stub, cfg
}

func assertUsageExit(t *testing.T, err error, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s 应报错", label)
	}
	if !clierr.HasKind(err, clierr.KindUsage) || exitCodeFor(err) != 2 {
		t.Fatalf("%s 应为用法错误 exit 2，实际 exit %d: %v", label, exitCodeFor(err), err)
	}
}

// TestMailReadFormatValidation --format 只接受服务端 enum full/plain_text_full/metadata：
// raw 等非法值本地报用法错误（exit 2，提示 --raw-body），且不发请求；metadata 原样透传。
func TestMailReadFormatValidation(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	stub, cfg := startMailStub(t)
	base := []string{"--as", "bot", "--mailbox", "user@example.com", "--config", cfg}
	cmds := map[string][]string{
		"message":  {"mail", "message", "--message-id", "m1"},
		"messages": {"mail", "messages", "--message-ids", "m1"},
		"thread":   {"mail", "thread", "--thread-id", "th_1"},
	}
	for name, args := range cmds {
		_, _, err := runCLI(t, append(append(append([]string{}, args...), base...), "--format", "raw")...)
		assertUsageExit(t, err, "mail "+name+" --format raw")
		if !strings.Contains(err.Error(), "full / plain_text_full / metadata") || !strings.Contains(err.Error(), "--raw-body") {
			t.Fatalf("mail %s --format raw 应列出可选值并提示 --raw-body: %v", name, err)
		}
	}
	if reqs := stub.requests(); len(reqs) != 0 {
		t.Fatalf("非法 --format 不应发请求: %v", reqs)
	}

	for name, args := range cmds {
		if _, stderr, err := runCLI(t, append(append(append([]string{}, args...), base...), "--format", "metadata", "-o", "json")...); err != nil {
			t.Fatalf("mail %s --format metadata 应被接受: %v\nstderr=%s", name, err, stderr)
		}
	}
	reqs := strings.Join(stub.requests(), "\n")
	for _, want := range []string{"/messages/m1?format=metadata", "/threads/th_1?format=metadata", `"format":"metadata"`} {
		if !strings.Contains(reqs, want) {
			t.Errorf("请求应透传 metadata（缺 %s）:\n%s", want, reqs)
		}
	}
}

// TestMailReadBotMailboxMeIsUsageError Bot 身份 + mailbox=me 是参数组合错误：exit 2、零网络。
// --as auto 且未配置 User Token（回退 Bot）时同样 exit 2，并提示可先登录。
func TestMailReadBotMailboxMeIsUsageError(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	stub, cfg := startMailStub(t)
	cmds := map[string][]string{
		"triage":   {"mail", "triage"},
		"message":  {"mail", "message", "--message-id", "m1"},
		"messages": {"mail", "messages", "--message-ids", "m1"},
		"thread":   {"mail", "thread", "--thread-id", "th_1"},
	}
	for name, args := range cmds {
		for _, as := range []string{"bot", "auto"} {
			full := append(append([]string{}, args...), "--as", as, "--config", cfg)
			_, _, err := runCLI(t, full...)
			assertUsageExit(t, err, fmt.Sprintf("mail %s --as %s（mailbox 默认 me）", name, as))
			if !strings.Contains(err.Error(), `不支持 mailbox="me"`) {
				t.Fatalf("mail %s --as %s 错误信息应说明 mailbox=me 不支持: %v", name, as, err)
			}
			if as == "auto" && !strings.Contains(err.Error(), "auth login") {
				t.Fatalf("auto 回退 Bot 时应提示可先登录: %v", err)
			}
		}
	}
	if reqs := stub.requests(); len(reqs) != 0 {
		t.Fatalf("mailbox=me 校验应在网络请求前: %v", reqs)
	}
}

// TestMailThreadModifyTrashPointsToThreadTrash thread-modify 拒绝 TRASH 时指向 thread-trash（不是 message-trash）。
func TestMailThreadModifyTrashPointsToThreadTrash(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	_, cfg := startMailStub(t)
	_, _, err := runCLI(t, "mail", "thread-modify", "--thread-ids", "th_1", "--folder-id", "trash", "--dry-run", "--config", cfg)
	assertUsageExit(t, err, "thread-modify --folder-id trash")
	if !strings.Contains(err.Error(), "mail thread-trash") || strings.Contains(err.Error(), "message-trash") {
		t.Fatalf("应指向 thread-trash: %v", err)
	}
	// message-modify 仍指向 message-trash
	_, _, err = runCLI(t, "mail", "message-modify", "--message-ids", "m1", "--folder-id", "TRASH", "--config", cfg)
	if err == nil || !strings.Contains(err.Error(), "mail message-trash") {
		t.Fatalf("message-modify 应指向 message-trash: %v", err)
	}
}

// TestMailSendInlineScanHelpNoDriveUpload 内嵌图片直接随 EML 提交，flag 帮助不能再说上传云盘。
func TestMailSendInlineScanHelpNoDriveUpload(t *testing.T) {
	f := mailSendCmd.Flags().Lookup("inline-images-auto-scan")
	if f == nil {
		t.Fatal("缺少 --inline-images-auto-scan")
	}
	if strings.Contains(f.Usage, "上传飞书云盘") || !strings.Contains(f.Usage, "不上传云盘") || !strings.Contains(f.Usage, "cid:") {
		t.Fatalf("flag 帮助与实现不符: %q", f.Usage)
	}
	for _, c := range []string{"message", "messages", "thread"} {
		cmd, _, err := rootCmd.Find([]string{"mail", c})
		if err != nil {
			t.Fatal(err)
		}
		if usage := cmd.Flags().Lookup("format").Usage; strings.Contains(usage, "raw") || !strings.Contains(usage, "metadata") {
			t.Errorf("mail %s --format 帮助应为 full/plain_text_full/metadata: %q", c, usage)
		}
		if strings.Contains(cmd.Long, "/ raw") {
			t.Errorf("mail %s 长帮助不应再列 raw 格式", c)
		}
	}
}
