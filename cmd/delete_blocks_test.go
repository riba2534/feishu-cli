package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

func initDeleteBlocksTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// TestDeleteAllBlocksPaginatesAndDeletesTrueTotal 验证 doc delete --all 建立一致性 revision 快照并全量真实删除
func TestDeleteAllBlocksPaginatesAndDeletesTrueTotal(t *testing.T) {
	docInfoCalls := 0
	getChildrenCalls := 0
	deleteCalled := false
	gotStartIndex := -1
	gotEndIndex := -1
	var gotGetChildRevision string
	var gotDeleteRevision string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/doc-123":
			docInfoCalls++
			// 返回当前 revision_id 为 5
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"doc-123","revision_id":5}}}`)
		case strings.HasSuffix(r.URL.Path, "/children") && r.Method == "GET":
			getChildrenCalls++
			gotGetChildRevision = r.URL.Query().Get("document_revision_id")
			pageToken := r.URL.Query().Get("page_token")
			if pageToken == "" {
				// 第一页：返回 2 个块，并且 has_more=true
				_, _ = fmt.Fprint(w, `{
					"code": 0,
					"msg": "ok",
					"data": {
						"items": [{"block_id":"b1"},{"block_id":"b2"}],
						"has_more": true,
						"page_token": "page-2"
					}
				}`)
			} else if pageToken == "page-2" {
				// 第二页：返回 1 个块，has_more=false
				_, _ = fmt.Fprint(w, `{
					"code": 0,
					"msg": "ok",
					"data": {
						"items": [{"block_id":"b3"}],
						"has_more": false
					}
				}`)
			} else {
				http.Error(w, "invalid page token: "+pageToken, http.StatusBadRequest)
			}
		case strings.Contains(r.URL.Path, "batch_delete") && r.Method == "DELETE":
			deleteCalled = true
			gotDeleteRevision = r.URL.Query().Get("document_revision_id")
			var body struct {
				StartIndex int `json:"start_index"`
				EndIndex   int `json:"end_index"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotStartIndex = body.StartIndex
			gotEndIndex = body.EndIndex
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document_revision_id":6}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDeleteBlocksTestConfig(t, server.URL)

	_ = deleteBlocksCmd.Flags().Set("all", "true")
	_ = deleteBlocksCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteBlocksCmd.Flags().Set("all", "false")
		_ = deleteBlocksCmd.Flags().Set("force", "false")
	}()
	err := deleteBlocksCmd.RunE(deleteBlocksCmd, []string{"doc-123", "parent-456"})
	if err != nil {
		t.Fatalf("deleteBlocksCmd 执行失败: %v", err)
	}

	if docInfoCalls == 0 {
		t.Fatal("未建立 revision 快照（未调用获取文档版本接口）")
	}
	if gotGetChildRevision != "5" {
		t.Fatalf("分页读取子块未携带一致性 revision 快照: got %q, want 5", gotGetChildRevision)
	}
	if getChildrenCalls != 2 {
		t.Fatalf("预期分页获取 2 次，实际调用 %d 次（说明未正确分页拉取全量子块）", getChildrenCalls)
	}
	if !deleteCalled {
		t.Fatal("未发起 batch_delete 删除调用")
	}
	if gotDeleteRevision != "5" {
		t.Fatalf("删除未携带一致性 revision 快照: got %q, want 5", gotDeleteRevision)
	}
	if gotStartIndex != 0 || gotEndIndex != 3 {
		t.Fatalf("删除范围预期 [0, 3)（包含跨页的全部 3 个子块），实际得到 [%d, %d)", gotStartIndex, gotEndIndex)
	}
}

// TestDeleteBlocksRevisionConflictAborts 验证发生并发 revision 冲突时非零退出
func TestDeleteBlocksRevisionConflictAborts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/doc-conflict":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"doc-conflict","revision_id":5}}}`)
		case strings.HasSuffix(r.URL.Path, "/children") && r.Method == "GET":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"block_id":"b1"}],"has_more":false}}`)
		case strings.Contains(r.URL.Path, "batch_delete"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":1770034,"msg":"document revision not match"}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDeleteBlocksTestConfig(t, server.URL)

	_ = deleteBlocksCmd.Flags().Set("all", "true")
	_ = deleteBlocksCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteBlocksCmd.Flags().Set("all", "false")
		_ = deleteBlocksCmd.Flags().Set("force", "false")
	}()

	err := deleteBlocksCmd.RunE(deleteBlocksCmd, []string{"doc-conflict", "parent-456"})
	if err == nil {
		t.Fatal("发生 revision 冲突时必须非零报错退出，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "1770034") && !strings.Contains(err.Error(), "revision not match") {
		t.Fatalf("错误信息应包含 revision 冲突信息，得到: %v", err)
	}
}

// TestDeleteBlocksFailsClosedWhenRevisionUnavailable 验证无法取得 revision 时不假装受保护，fail closed
func TestDeleteBlocksFailsClosedWhenRevisionUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/doc-no-rev":
			// 返回 document 但没有 revision_id
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"doc-no-rev"}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDeleteBlocksTestConfig(t, server.URL)

	_ = deleteBlocksCmd.Flags().Set("all", "true")
	_ = deleteBlocksCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteBlocksCmd.Flags().Set("all", "false")
		_ = deleteBlocksCmd.Flags().Set("force", "false")
	}()

	err := deleteBlocksCmd.RunE(deleteBlocksCmd, []string{"doc-no-rev", "parent-456"})
	if err == nil {
		t.Fatal("无法获取 revision 时必须 fail closed 报错，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "版本号") && !strings.Contains(err.Error(), "快照") {
		t.Fatalf("错误信息应说明无法取得版本号，得到: %v", err)
	}
}

// TestDeleteBlocksFailsClosedWhenRevisionZero 验证 revision=0 时不被当作安全快照，fail closed
func TestDeleteBlocksFailsClosedWhenRevisionZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/doc-zero-rev":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"doc-zero-rev","revision_id":0}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDeleteBlocksTestConfig(t, server.URL)

	_ = deleteBlocksCmd.Flags().Set("all", "true")
	_ = deleteBlocksCmd.Flags().Set("force", "true")
	defer func() {
		_ = deleteBlocksCmd.Flags().Set("all", "false")
		_ = deleteBlocksCmd.Flags().Set("force", "false")
	}()

	err := deleteBlocksCmd.RunE(deleteBlocksCmd, []string{"doc-zero-rev", "parent-456"})
	if err == nil {
		t.Fatal("revision=0 时必须 fail closed 报错，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "无法取得有效正整数文档版本号") {
		t.Fatalf("错误应说明无法取得有效正整数版本号，得到: %v", err)
	}
}
