package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewClientTokenIsUUIDv4AndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok := NewClientToken()
		if !uuidV4Re.MatchString(tok) {
			t.Fatalf("client_token %q 不是 UUIDv4 形态", tok)
		}
		if seen[tok] {
			t.Fatalf("client_token 重复: %q", tok)
		}
		seen[tok] = true
	}
}

// TestCreateBlockWithRetryReusesClientToken 模拟"首次请求服务端 5xx"后自动重试：
// 两次 POST children 必须携带同一个非空 client_token，服务端才能把重放识别为同一逻辑请求，
// 避免首包其实已落库时重试再插入一份重复块。
func TestCreateBlockWithRetryReusesClientToken(t *testing.T) {
	cases := []struct {
		name      string
		firstCode int
		firstBody string
	}{
		{"服务端内部错误", http.StatusInternalServerError, `{"code":500,"msg":"internal error"}`},
		{"限流", http.StatusTooManyRequests, `{"code":99991400,"msg":"request trigger frequency limit"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var tokens []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
					fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
					return
				}
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/blocks/doc1/children") {
					http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
					return
				}
				mu.Lock()
				tokens = append(tokens, r.URL.Query().Get("client_token"))
				n := len(tokens)
				mu.Unlock()
				if n == 1 {
					w.WriteHeader(tc.firstCode)
					fmt.Fprint(w, tc.firstBody)
					return
				}
				fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"children":[{"block_id":"blk_1","block_type":2}],"client_token":"x","document_revision_id":3}}`)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			bt := 2
			content := "段落"
			res := CreateBlockWithRetry("doc1", "doc1", []*larkdocx.Block{{
				BlockType: &bt,
				Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &content}}}},
			}}, -1, RetryConfig{MaxRetries: 3, MaxTotalAttempts: 5, RetryOnRateLimit: true}, "")
			if res.Err != nil {
				t.Fatalf("CreateBlockWithRetry 返回错误: %v", res.Err)
			}
			if len(res.Value) != 1 || StringVal(res.Value[0].BlockId) != "blk_1" {
				t.Fatalf("返回块异常: %#v", res.Value)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(tokens) != 2 {
				t.Fatalf("请求次数 = %d，期望 2（首包失败 + 一次重试）", len(tokens))
			}
			if tokens[0] == "" {
				t.Fatal("首个请求未携带 client_token")
			}
			if tokens[0] != tokens[1] {
				t.Fatalf("重试未复用 client_token：首包 %q，重试 %q", tokens[0], tokens[1])
			}
		})
	}
}

// TestCreateBlockWithoutClientTokenOmitsParam 单次 CreateBlock 不下发空 client_token 参数。
func TestCreateBlockWithoutClientTokenOmitsParam(t *testing.T) {
	var got []string
	var present bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		got, present = r.URL.Query()["client_token"]
		fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"children":[{"block_id":"blk_1","block_type":2}]}}`)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	bt := 2
	if _, _, err := CreateBlock("doc1", "doc1", []*larkdocx.Block{{BlockType: &bt, Text: &larkdocx.Text{}}}, -1); err != nil {
		t.Fatalf("CreateBlock 返回错误: %v", err)
	}
	if present {
		t.Fatalf("单次 CreateBlock 不应下发 client_token，实际 %v", got)
	}
}

// TestAddBoardWithClientTokenPassesToken 画板块创建同样透传 client_token。
func TestAddBoardWithClientTokenPassesToken(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		got = r.URL.Query().Get("client_token")
		fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"children":[{"block_id":"blk_b","block_type":43,"board":{"token":"wb_1"}}]}}`)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	res, _, err := AddBoardWithClientToken("doc1", "", -1, "tok-123456")
	if err != nil {
		t.Fatalf("AddBoardWithClientToken 返回错误: %v", err)
	}
	if got != "tok-123456" {
		t.Fatalf("client_token = %q，期望 tok-123456", got)
	}
	if res.WhiteboardID != "wb_1" || res.BlockID != "blk_b" {
		t.Fatalf("返回值异常: %#v", res)
	}
}
