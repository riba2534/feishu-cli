package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCreateServer struct {
	mu       sync.Mutex
	posts    int
	gets     int
	postBody map[string]any
	getResps []string // 依次返回的 GET 响应（HTTP 200 body）；"500" 表示返回 HTTP 500
	postResp string
}

func (f *fakeCreateServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "log-create")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/docs_ai/v1/documents":
			f.posts++
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &f.postBody)
			fmt.Fprint(w, f.postResp)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/open-apis/docs_ai/v1/async_tasks/"):
			idx := f.gets
			f.gets++
			if idx >= len(f.getResps) {
				idx = len(f.getResps) - 1
			}
			if f.getResps[idx] == "500" {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"code":500,"msg":"internal error"}`)
				return
			}
			fmt.Fprint(w, f.getResps[idx])
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func withFastPolling(t *testing.T) {
	t.Helper()
	orig := docsAICreateSleep
	docsAICreateSleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }
	t.Cleanup(func() { docsAICreateSleep = orig })
}

// TestCreateDocsAIAsyncPollsUntilSucceeded：空状态按 processing 继续、GET 5xx 只重试读取、
// 成功后解码 create_document；整个过程只发出一次 POST。
func TestCreateDocsAIAsyncPollsUntilSucceeded(t *testing.T) {
	withFastPolling(t)
	f := &fakeCreateServer{
		postResp: `{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"processing","poll_after_ms":1}}}`,
		getResps: []string{
			`{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":""}}}`,
			"500",
			`{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"succeeded","result":{"create_document":"{\"document\":{\"document_id\":\"docNew\",\"revision_id\":2,\"url\":\"https://example.feishu.cn/docx/docNew\"},\"warnings\":[]}"}}}}`,
		},
	}
	server := httptest.NewServer(f.handler(t))
	defer server.Close()
	setupTestConfig(t, server.URL)

	data, err := CreateDocsAIDocument(map[string]any{"format": "markdown", "content": "# T"}, "")
	if err != nil {
		t.Fatalf("CreateDocsAIDocument 失败: %v", err)
	}
	doc, _ := data["document"].(map[string]any)
	if doc["document_id"] != "docNew" {
		t.Fatalf("data = %#v", data)
	}
	if f.posts != 1 {
		t.Fatalf("POST 次数 = %d，绝不能重放创建请求", f.posts)
	}
	if f.gets != 3 {
		t.Fatalf("GET 次数 = %d，期望 3（空状态继续 + 5xx 重试 + 成功）", f.gets)
	}
	if f.postBody["extra_param"] != `{"open_create_async":true}` {
		t.Fatalf("请求体缺少 open_create_async: %#v", f.postBody)
	}
}

func TestCreateDocsAIExpiredMapsToTimeoutWithBatchHint(t *testing.T) {
	withFastPolling(t)
	for _, final := range []string{
		`{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"expired"}}}`,
		`{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"failed","failure":{"code":"execution_interrupted","message":"interrupted"}}}}`,
	} {
		f := &fakeCreateServer{
			postResp: `{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"processing"}}}`,
			getResps: []string{final},
		}
		server := httptest.NewServer(f.handler(t))
		setupTestConfig(t, server.URL)
		_, err := CreateDocsAIDocument(map[string]any{"format": "markdown", "content": "x"}, "")
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "处理时间过长") || !strings.Contains(err.Error(), "--mode append") {
			t.Fatalf("expired/execution_interrupted 应映射为超时并提示分批，得到: %v", err)
		}
		if f.posts != 1 {
			t.Fatalf("POST 次数 = %d", f.posts)
		}
	}
}

func TestCreateDocsAIFailedTaskAndDirectResponse(t *testing.T) {
	withFastPolling(t)
	f := &fakeCreateServer{
		postResp: `{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"failed","failure":{"code":"bad_content","message":"invalid xml"}}}}`,
	}
	server := httptest.NewServer(f.handler(t))
	setupTestConfig(t, server.URL)
	_, err := CreateDocsAIDocument(map[string]any{"format": "xml", "content": "<p>"}, "")
	server.Close()
	if err == nil || !strings.Contains(err.Error(), "invalid xml") || !strings.Contains(err.Error(), "bad_content") {
		t.Fatalf("失败任务应透出 failure 信息，得到: %v", err)
	}

	f = &fakeCreateServer{postResp: `{"code":0,"msg":"","data":{"document":{"document_id":"docD","revision_id":1}}}`}
	server = httptest.NewServer(f.handler(t))
	defer server.Close()
	setupTestConfig(t, server.URL)
	data, err := CreateDocsAIDocument(map[string]any{"format": "markdown", "content": "x"}, "")
	if err != nil || data["log_id"] != "log-create" || f.gets != 0 {
		t.Fatalf("同步返回不应轮询: data=%#v err=%v gets=%d", data, err, f.gets)
	}
}

func TestCreateDocsAIWaitDeadline(t *testing.T) {
	withFastPolling(t)
	orig := docsAICreateMaxWaitOverride
	docsAICreateMaxWaitOverride = 50 * time.Millisecond
	defer func() { docsAICreateMaxWaitOverride = orig }()
	docsAICreateSleep = func(ctx context.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
			return nil
		}
	}
	f := &fakeCreateServer{
		postResp: `{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"processing"}}}`,
		getResps: []string{`{"code":0,"msg":"","data":{"task":{"task_id":"t1","status":"processing"}}}`},
	}
	server := httptest.NewServer(f.handler(t))
	defer server.Close()
	setupTestConfig(t, server.URL)
	_, err := CreateDocsAIDocument(map[string]any{"format": "markdown", "content": "x"}, "")
	if err == nil || !strings.Contains(err.Error(), "处理时间过长") || f.posts != 1 {
		t.Fatalf("超时应报处理时间过长且不重放 POST: err=%v posts=%d", err, f.posts)
	}
}
