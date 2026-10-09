package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/profile"
	"github.com/spf13/viper"
)

func initWikiNodeDeleteTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	tempHome := t.TempDir()
	restoreHome := profile.SetHomeFunc(func() (string, error) {
		return tempHome, nil
	})
	t.Cleanup(restoreHome)
	t.Setenv("FEISHU_APP_ID", "cli_test")
	t.Setenv("FEISHU_APP_SECRET", "test_secret")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// writeWikiNodeByTokenEcho 模拟 node_by_token：把请求里的 token 原样作为 node_token / obj_token 返回，space_id 固定。
// 删除命令无论是否传 --space-id 都会先解析节点并核对空间，测试服务端必须响应该端点。
func writeWikiNodeByTokenEcho(w http.ResponseWriter, r *http.Request, spaceID string) {
	tok := r.URL.Query().Get("token")
	_, _ = fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"node":{"space_id":%q,"node_token":%q,"obj_token":%q,"obj_type":"docx","node_type":"origin","title":"测试"}}}`, spaceID, tok, tok)
}

// TestDeleteWikiNodePathBodyAndAsyncPoll 验证 Wiki 节点删除走正确的 OpenAPI 路径、Body 以及异步任务轮询
func TestDeleteWikiNodePathBodyAndAsyncPoll(t *testing.T) {
	deleteCalled := false
	var gotPath string
	var gotBody struct {
		ObjType         string `json:"obj_type"`
		IncludeChildren bool   `json:"include_children"`
	}
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			// 自动解析 space_id
			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"node": {
						"space_id": "space-999",
						"node_token": "wikcnTestNode",
						"obj_token": "doxcnTestObj",
						"title": "测试节点",
						"obj_type": "docx"
					}
				}
			}`)
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/space-999/nodes/wikcnTestNode":
			deleteCalled = true
			gotPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			// 返回异步 task_id
			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"task_id": "task-async-777"
				}
			}`)
		case r.Method == "GET" && r.URL.Path == "/open-apis/wiki/v2/tasks/task-async-777":
			pollCount++
			taskType := r.URL.Query().Get("task_type")
			if taskType != "delete_node" {
				http.Error(w, "task_type 必须为 delete_node", http.StatusBadRequest)
				return
			}
			if pollCount == 1 {
				// 第一次轮询中
				_, _ = fmt.Fprint(w, `{
					"code": 0,
					"msg": "ok",
					"data": {
						"task": {
							"task_id": "task-async-777",
							"simple_task_result": {
								"status": "processing"
							}
						}
					}
				}`)
			} else {
				// 第二次成功
				_, _ = fmt.Fprint(w, `{
					"code": 0,
					"msg": "ok",
					"data": {
						"task": {
							"task_id": "task-async-777",
							"simple_task_result": {
								"status": "success"
							}
						}
					}
				}`)
			}
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	origAttempts := wikiDeleteNodePollAttempts
	origInterval := wikiDeleteNodePollInterval
	wikiDeleteNodePollAttempts = 3
	wikiDeleteNodePollInterval = 5 * time.Millisecond
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		wikiDeleteNodePollAttempts = origAttempts
		wikiDeleteNodePollInterval = origInterval
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnTestNode"})
	if err != nil {
		t.Fatalf("deleteWikiNodeCmd 运行失败: %v", err)
	}

	if !deleteCalled {
		t.Fatal("未调用 DELETE /wiki/v2/spaces/{space}/nodes/{node}")
	}
	wantPath := "/open-apis/wiki/v2/spaces/space-999/nodes/wikcnTestNode"
	if gotPath != wantPath {
		t.Fatalf("请求路径 = %q, 期望 %q", gotPath, wantPath)
	}
	if gotBody.ObjType != "wiki" {
		t.Fatalf("请求 Body 中的 obj_type = %q, 期望 wiki", gotBody.ObjType)
	}
	if !gotBody.IncludeChildren {
		t.Fatalf("请求 Body 中的 include_children = false, 期望 true")
	}
	if pollCount < 2 {
		t.Fatalf("异步任务轮询次数 = %d, 期望 >= 2", pollCount)
	}
}

// TestDeleteWikiNodeSyncCompletion 验证同步完成（无 task_id）时不进行任务轮询
func TestDeleteWikiNodeSyncCompletion(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-1")
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-1/nodes/wikcnSync":
			deleteCalled = true
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":""}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp-1")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnSync"})
	if err != nil {
		t.Fatalf("deleteWikiNodeCmd 运行失败: %v", err)
	}
	if !deleteCalled {
		t.Fatal("未发起同步删除请求")
	}
}

// TestDeleteWikiNodeAllFailedReturnsError 验证所有状态轮询均失败时返回非零退出，并保留 task_id 及 resume 提示，且绝不泄露 token 字节
func TestDeleteWikiNodeAllFailedReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-fail")
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-fail/nodes/node-fail":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-failed-999"}}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			// 模拟所有轮询均失败
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	origAttempts := wikiDeleteNodePollAttempts
	origInterval := wikiDeleteNodePollInterval
	wikiDeleteNodePollAttempts = 2
	wikiDeleteNodePollInterval = 1 * time.Millisecond
	defer func() {
		wikiDeleteNodePollAttempts = origAttempts
		wikiDeleteNodePollInterval = origInterval
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
		_ = deleteWikiNodeCmd.Flags().Set("user-access-token", "")
		_ = deleteWikiNodeCmd.Flags().Set("as", "auto")
	}()

	secretToken := "u-secret-token-super-private-9999"
	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp-fail")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	_ = deleteWikiNodeCmd.Flags().Set("user-access-token", secretToken)
	_ = deleteWikiNodeCmd.Flags().Set("as", "user")

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"node-fail"})
	if err == nil {
		t.Fatal("状态查询全部失败时必须返回非零错误，绝不能返回 nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "task-failed-999") {
		t.Fatalf("错误信息必须保留 task_id (task-failed-999)，实际得到: %s", errMsg)
	}
	if !strings.Contains(errMsg, "feishu-cli drive task-result --scenario wiki_delete_node") {
		t.Fatalf("错误信息必须包含有效的 resume 查询提示命令，实际得到: %s", errMsg)
	}
	if !strings.Contains(errMsg, "--as user") {
		t.Fatalf("resume 命令必须携带身份标志 --as user，实际得到: %s", errMsg)
	}
	// 严防凭证泄漏：断言 token 字节绝不在错误输出中
	if strings.Contains(errMsg, secretToken) {
		t.Fatalf("严重违规：错误信息中泄漏了明文 token 字节！%s", errMsg)
	}
}

// TestDeleteWikiNodeTimeoutReturnsError 验证轮询超时（仍为 processing）时返回非零退出，保留 task_id 与 resume 提示且不泄露 token
func TestDeleteWikiNodeTimeoutReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-timeout")
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-timeout/nodes/node-timeout":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-timeout-888"}}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			// 模拟一直为 processing
			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"task": {
						"task_id": "task-timeout-888",
						"simple_task_result": {
							"status": "processing"
						}
					}
				}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	origAttempts := wikiDeleteNodePollAttempts
	origInterval := wikiDeleteNodePollInterval
	wikiDeleteNodePollAttempts = 2
	wikiDeleteNodePollInterval = 1 * time.Millisecond
	defer func() {
		wikiDeleteNodePollAttempts = origAttempts
		wikiDeleteNodePollInterval = origInterval
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
		_ = deleteWikiNodeCmd.Flags().Set("user-access-token", "")
		_ = deleteWikiNodeCmd.Flags().Set("as", "auto")
	}()

	secretToken := "u-secret-token-timeout-check-8888"
	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp-timeout")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	_ = deleteWikiNodeCmd.Flags().Set("user-access-token", secretToken)
	_ = deleteWikiNodeCmd.Flags().Set("as", "user")

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"node-timeout"})
	if err == nil {
		t.Fatal("轮询超时仍未完成时必须返回非零错误，绝不能返回 nil 谎报成功")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "task-timeout-888") {
		t.Fatalf("错误信息必须保留 task_id (task-timeout-888)，实际得到: %s", errMsg)
	}
	if !strings.Contains(errMsg, "feishu-cli drive task-result --scenario wiki_delete_node") {
		t.Fatalf("错误信息必须包含有效 resume 命令，实际得到: %s", errMsg)
	}
	if strings.Contains(errMsg, secretToken) {
		t.Fatalf("严重违规：错误信息中泄漏了明文 token 字节！%s", errMsg)
	}
}

// TestDeleteWikiNodeCancelReturnsError 验证上下文取消时返回非零退出，保留 task_id 与 resume 提示且不泄露 token
func TestDeleteWikiNodeCancelReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-cancel/nodes/node-cancel":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-cancel-777"}}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task":{"task_id":"task-cancel-777","simple_task_result":{"status":"processing"}}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消
	cancel()

	secretToken := "u-secret-token-cancel-check-7777"
	status, err := pollDeleteWikiNodeTask(ctx, "task-cancel-777", secretToken, "user")
	if err == nil {
		t.Fatal("取消时必须返回非零错误")
	}
	if status == nil {
		t.Fatal("status 不应为 nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "task-cancel-777") {
		t.Fatalf("错误信息必须保留 task_id (task-cancel-777)，得到: %s", errMsg)
	}
	if !strings.Contains(errMsg, "feishu-cli drive task-result --scenario wiki_delete_node") {
		t.Fatalf("错误信息必须保留有效 resume 命令，得到: %s", errMsg)
	}
	if strings.Contains(errMsg, secretToken) {
		t.Fatalf("严重违规：错误信息中泄漏了明文 token 字节！%s", errMsg)
	}
}

// TestDeleteWikiNodeURLValidation 验证严格的 URL 解析、域名白名单、Scheme 与路径边界校验（table-driven tests）
func TestDeleteWikiNodeURLValidation(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantErr   bool
		errSubstr string
		wantToken string
		wantObj   string
	}{
		{
			name:      "伪造域名 evilfeishu.cn 必须被拒绝",
			rawURL:    "https://evilfeishu.cn/wiki/wikcnTarget",
			wantErr:   true,
			errSubstr: "不支持的域名",
		},
		{
			name:      "伪造域名 notlarksuite.com 必须被拒绝",
			rawURL:    "https://notlarksuite.com/wiki/wikcnTarget",
			wantErr:   true,
			errSubstr: "不支持的域名",
		},
		{
			name:      "第三方域名嵌入 query 假路径必须被拒绝",
			rawURL:    "https://attacker.com/evil?redirect=/wiki/wikcnTarget",
			wantErr:   true,
			errSubstr: "不支持的域名",
		},
		{
			name:      "外部非 loopback 域名使用 HTTP 协议必须被拒绝",
			rawURL:    "http://sample.feishu.cn/wiki/wikcnTarget",
			wantErr:   true,
			errSubstr: "必须使用 HTTPS 协议",
		},
		{
			name:      "包含 userinfo 凭证嵌入必须被拒绝",
			rawURL:    "https://user:pass@sample.feishu.cn/wiki/wikcnTarget",
			wantErr:   true,
			errSubstr: "用户信息",
		},
		{
			name:      "包含额外 path 段必须被拒绝",
			rawURL:    "https://sample.feishu.cn/wiki/node123/extra/segment",
			wantErr:   true,
			errSubstr: "URL 路径格式无效",
		},
		{
			name:      "包含 encoded slash (%2f) 必须被拒绝",
			rawURL:    "https://sample.feishu.cn/wiki/node%2fescape",
			wantErr:   true,
			errSubstr: "非法的转义斜杠",
		},
		{
			name:      "包含控制字符 (%00) 必须被拒绝",
			rawURL:    "https://sample.feishu.cn/wiki/node%00null",
			wantErr:   true,
			errSubstr: "非法字符",
		},
		{
			name:      "不支持的路径前缀必须被拒绝",
			rawURL:    "https://sample.feishu.cn/evil_path/wikcnTarget",
			wantErr:   true,
			errSubstr: "不支持的 URL 路径前缀",
		},
		{
			name:      "合法飞书 HTTPS 域名正常解析",
			rawURL:    "https://sample.feishu.cn/wiki/wikcnValidNode?extra=1#frag",
			wantErr:   false,
			wantToken: "wikcnValidNode",
			wantObj:   "wiki",
		},
		{
			name:      "合法官方域名带自定义端口正常解析",
			rawURL:    "https://sample.feishu.cn:8443/docx/doxcnPortDoc",
			wantErr:   false,
			wantToken: "doxcnPortDoc",
			wantObj:   "docx",
		},
		{
			name:      "合法本地 loopback HTTP 测试地址正常解析",
			rawURL:    "http://127.0.0.1:9090/sheets/shtcnSheetToken",
			wantErr:   false,
			wantToken: "shtcnSheetToken",
			wantObj:   "sheet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, objType, err := parseWikiDeleteInput(tt.rawURL, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWikiDeleteInput(%q) err = %v, wantErr = %v", tt.rawURL, err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("错误信息应当包含 %q，实际得到: %v", tt.errSubstr, err)
			}
			if !tt.wantErr {
				if tok != tt.wantToken || objType != tt.wantObj {
					t.Fatalf("parseWikiDeleteInput(%q) = (%q, %q), 期望 (%q, %q)",
						tt.rawURL, tok, objType, tt.wantToken, tt.wantObj)
				}
			}
		})
	}
}

// TestDeleteWikiNodeInvalidServerTaskIDRejected 验证服务端返回非法 task_id 时被校验拦截
func TestDeleteWikiNodeInvalidServerTaskIDRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-123")
		case r.Method == "DELETE":
			// 返回含有路径分隔符的非法 task_id
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task/escape/evil"}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp-123")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnTest"})
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("服务端返回非法 task_id 必须被拦截报错，得到: %v", err)
	}
}

// TestDeleteWikiNodeInvalidParsedSpaceIDRejected 验证 node_by_token 返回非法 space_id 时立即拦截且不发 DELETE 请求
func TestDeleteWikiNodeInvalidParsedSpaceIDRejected(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			// 返回包含路径分隔符的非法 space_id
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{"node":{"space_id":"sp/evil_path_inject","node_token":"wikcnTest","obj_token":"doxcnTest","obj_type":"docx","title":"测试"}}
			}`)
		case r.Method == "DELETE":
			deleteCalled = true
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":""}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnTest"})
	if err == nil || !strings.Contains(err.Error(), "space_id 非法") {
		t.Fatalf("服务端返回非法 space_id 必须被拦截报错，得到: %v", err)
	}
	if deleteCalled {
		t.Fatal("解析出非法 space_id 后绝不能发起 DELETE 请求！")
	}
}

// TestDeleteWikiNodeInvalidOutputZeroNetwork 验证非法 --output 在任何网络请求前 fail closed
func TestDeleteWikiNodeInvalidOutputZeroNetwork(t *testing.T) {
	// 指向不可达端口，确保若发起任何网络请求必将报错
	initWikiNodeDeleteTestConfig(t, "http://127.0.0.1:59998")

	_ = deleteWikiNodeCmd.Flags().Set("output", "yaml")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("output", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnDummy"})
	if err == nil {
		t.Fatal("非法 --output yaml 必须立即报错")
	}
	if !strings.Contains(err.Error(), "不支持的 --output") {
		t.Fatalf("错误信息应说明不支持的 output，得到: %v", err)
	}
}

// TestDeleteWikiNodeSpaceIDValidation 验证显式传入非法 space-id 被拦截
func TestDeleteWikiNodeSpaceIDValidation(t *testing.T) {
	initWikiNodeDeleteTestConfig(t, "http://127.0.0.1:9999")
	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp/with/slash")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnDummy"})
	if err == nil || !strings.Contains(err.Error(), "--space-id") {
		t.Fatalf("非法 space-id 应被校验拒绝，实际得到: %v", err)
	}
}

// TestDeleteWikiNodePathEscaped 验证 space_id、node_token 和 task_id 正确进行 PathEscape 转义
func TestDeleteWikiNodePathEscaped(t *testing.T) {
	var gotDeletePath, gotPollPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			writeWikiNodeByTokenEcho(w, r, "sp-123")
		case r.Method == "DELETE":
			gotDeletePath = r.URL.EscapedPath()
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"task_id":"task-escaped-123"}}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/open-apis/wiki/v2/tasks/"):
			gotPollPath = r.URL.EscapedPath()
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{"task":{"task_id":"task-escaped-123","simple_task_result":{"status":"success"}}}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	origAttempts := wikiDeleteNodePollAttempts
	origInterval := wikiDeleteNodePollInterval
	wikiDeleteNodePollAttempts = 2
	wikiDeleteNodePollInterval = 1 * time.Millisecond
	defer func() {
		wikiDeleteNodePollAttempts = origAttempts
		wikiDeleteNodePollInterval = origInterval
		_ = deleteWikiNodeCmd.Flags().Set("space-id", "")
		_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	_ = deleteWikiNodeCmd.Flags().Set("space-id", "sp-123")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "wiki")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnEscaped"})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	wantDeletePath := "/open-apis/wiki/v2/spaces/sp-123/nodes/wikcnEscaped"
	if gotDeletePath != wantDeletePath {
		t.Fatalf("DELETE 路径转义异常: got %q, want %q", gotDeletePath, wantDeletePath)
	}
	wantPollPath := "/open-apis/wiki/v2/tasks/task-escaped-123"
	if gotPollPath != wantPollPath {
		t.Fatalf("Task 轮询路径转义异常: got %q, want %q", gotPollPath, wantPollPath)
	}
}

// TestDeleteWikiNodeBareTokenRequiresObjType 验证裸 token 输入缺 --obj-type 时报错
func TestDeleteWikiNodeBareTokenRequiresObjType(t *testing.T) {
	initWikiNodeDeleteTestConfig(t, "http://127.0.0.1:9999")
	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"wikcnBareToken"})
	if err == nil {
		t.Fatal("裸 token 未指定 --obj-type 必须报错")
	}
	if !strings.Contains(err.Error(), "--obj-type 为必填项") {
		t.Fatalf("错误应提示 --obj-type 为必填项，实际得到: %v", err)
	}
}

// TestDeleteWikiNodeURLInfersObjTypeAndPassesObjType 验证完整 URL 自动推断 obj_type 并传给删除接口；
// 解析 space 时走 node_by_token，由服务端识别 token 类型，不再发送 obj_type
func TestDeleteWikiNodeURLInfersObjTypeAndPassesObjType(t *testing.T) {
	var gotGetNodeQuery string
	var gotDeleteBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			gotGetNodeQuery = r.URL.RawQuery
			_, _ = fmt.Fprint(w, `{
				"code": 0, "msg": "ok",
				"data": {"node": {"space_id": "sp-inferred", "node_token": "wikcnRealNode", "obj_token": "doxcnReal", "obj_type": "docx"}}
			}`)
		case r.Method == "DELETE" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-inferred/nodes/doxcnReal":
			_ = json.NewDecoder(r.Body).Decode(&gotDeleteBody)
			_, _ = fmt.Fprint(w, `{"code": 0, "msg": "ok", "data": {"task_id": ""}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initWikiNodeDeleteTestConfig(t, server.URL)

	_ = deleteWikiNodeCmd.Flags().Set("obj-type", "")
	_ = deleteWikiNodeCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteWikiNodeCmd.Flags().Set("force", "false")
	}()

	err := deleteWikiNodeCmd.RunE(deleteWikiNodeCmd, []string{"https://sample.feishu.cn/docx/doxcnReal"})
	if err != nil {
		t.Fatalf("URL 执行删除失败: %v", err)
	}

	// node_by_token 由服务端识别 token 类型：只传 token，不再传 obj_type
	if gotGetNodeQuery != "token=doxcnReal" {
		t.Fatalf("解析 space 时 node_by_token query 应只含 token=doxcnReal，实际 query: %q", gotGetNodeQuery)
	}
	if gotDeleteBody["obj_type"] != "docx" {
		t.Fatalf("DELETE 请求 body obj_type = %v, 期望 docx", gotDeleteBody["obj_type"])
	}
}

// TestBuildWikiDeleteNodeResumeCmdPOSIXShellSafe 验证 taskID 含 $(), backtick, single quote, whitespace 时不可触发展开且完全安全
func TestBuildWikiDeleteNodeResumeCmdPOSIXShellSafe(t *testing.T) {
	testCases := []struct {
		taskID   string
		identity string
		want     string
	}{
		{
			taskID:   "$(touch /tmp/pwn)",
			identity: "user",
			want:     "feishu-cli drive task-result --scenario wiki_delete_node --task-id '$(touch /tmp/pwn)' --as user",
		},
		{
			taskID:   "task`rm -rf /`",
			identity: "bot",
			want:     "feishu-cli drive task-result --scenario wiki_delete_node --task-id 'task`rm -rf /`' --as bot",
		},
		{
			taskID:   "task'with'quote",
			identity: "user",
			want:     "feishu-cli drive task-result --scenario wiki_delete_node --task-id 'task'\\''with'\\''quote' --as user",
		},
		{
			taskID:   "$USER $HOME task",
			identity: "bot",
			want:     "feishu-cli drive task-result --scenario wiki_delete_node --task-id '$USER $HOME task' --as bot",
		},
	}

	for _, tc := range testCases {
		got := buildWikiDeleteNodeResumeCmd(tc.taskID, tc.identity)
		if got != tc.want {
			t.Errorf("buildWikiDeleteNodeResumeCmd(%q, %q) = %q, want %q", tc.taskID, tc.identity, got, tc.want)
		}
		// 严密断言：命令中不得使用双引号包裹 task_id
		if strings.Contains(got, `"`+tc.taskID+`"`) {
			t.Errorf("resume 命令禁止使用双引号包裹 task_id: %s", got)
		}
	}
}

// TestParseWikiDeleteInput_LoopbackHostnameNotPrefixMatched 验证 HTTP loopback 豁免用
// net.ParseIP 精确判定，而非字符串前缀匹配。
// 回归防护：strings.HasPrefix(hostname, "127.0.0.") 会把攻击者可注册的
// 127.0.0.evil.com 当成本地地址放行，绕过「非本地必须 HTTPS」的约束。
func TestParseWikiDeleteInput_LoopbackHostnameNotPrefixMatched(t *testing.T) {
	rejected := []string{
		"http://127.0.0.evil.com/wiki/wikcnAbcdefg",
		"http://127.0.0.1.evil.com/wiki/wikcnAbcdefg",
		"http://127.0.0.1evil.com/wiki/wikcnAbcdefg",
		"http://localhost.evil.com/wiki/wikcnAbcdefg",
		"http://feishu.cn/wiki/wikcnAbcdefg",
	}
	for _, raw := range rejected {
		if _, _, err := parseWikiDeleteInput(raw, ""); err == nil {
			t.Errorf("%s: 非回环 HTTP 地址应被拒绝", raw)
		}
	}

	accepted := []string{
		"http://127.0.0.1/wiki/wikcnAbcdefg",
		"http://127.0.0.1:8080/wiki/wikcnAbcdefg",
		"http://127.0.0.2/wiki/wikcnAbcdefg",
		"http://localhost/wiki/wikcnAbcdefg",
		"http://[::1]/wiki/wikcnAbcdefg",
	}
	for _, raw := range accepted {
		if _, _, err := parseWikiDeleteInput(raw, ""); err != nil {
			t.Errorf("%s: 真实回环地址应放行，得到: %v", raw, err)
		}
	}
}

// TestIsLoopbackHostname 单测回环判定本身
func TestIsLoopbackHostname(t *testing.T) {
	for _, h := range []string{"localhost", "LOCALHOST", "127.0.0.1", "127.0.0.2", "127.1.2.3", "::1", "[::1]"} {
		if !isLoopbackHostname(h) {
			t.Errorf("%q 应判为回环", h)
		}
	}
	for _, h := range []string{"127.0.0.evil.com", "127.0.0.1.evil.com", "localhost.evil.com", "feishu.cn", "", "8.8.8.8", "0.0.0.0"} {
		if isLoopbackHostname(h) {
			t.Errorf("%q 不应判为回环", h)
		}
	}
}
