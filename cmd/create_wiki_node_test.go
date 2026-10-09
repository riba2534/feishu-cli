package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/viper"
)

func initCreateWikiNodeTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	// 隔离本机 token.json / 环境变量凭证：wiki create 以 Bot 身份执行后会尝试解析当前登录用户做自动授权
	tempHome := t.TempDir()
	restoreHome := profile.SetHomeFunc(func() (string, error) { return tempHome, nil })
	t.Cleanup(restoreHome)
	t.Setenv("FEISHU_APP_ID", "cli_test")
	t.Setenv("FEISHU_APP_SECRET", "test_secret")
	t.Setenv("FEISHU_PROFILE", "")
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

// TestCreateWikiNodeShortcutOriginNodeTokenValidation 验证 shortcut 必须传 origin-node-token，origin 节点禁止传
func TestCreateWikiNodeShortcutOriginNodeTokenValidation(t *testing.T) {
	initCreateWikiNodeTestConfig(t, "http://127.0.0.1:9999")

	// 1. shortcut 缺少 origin-node-token 必须报错
	_ = createWikiNodeCmd.Flags().Set("space-id", "sp-1")
	_ = createWikiNodeCmd.Flags().Set("title", "测试快捷方式")
	_ = createWikiNodeCmd.Flags().Set("node-type", "shortcut")
	_ = createWikiNodeCmd.Flags().Set("origin-node-token", "")
	err := createWikiNodeCmd.RunE(createWikiNodeCmd, []string{})
	if err == nil {
		t.Fatal("shortcut 未传 --origin-node-token 应报错")
	}

	// 2. origin 传 origin-node-token 必须报错
	_ = createWikiNodeCmd.Flags().Set("node-type", "origin")
	_ = createWikiNodeCmd.Flags().Set("origin-node-token", "wikcnOrigin")
	err = createWikiNodeCmd.RunE(createWikiNodeCmd, []string{})
	if err == nil {
		t.Fatal("origin 传入 --origin-node-token 应报错")
	}
}

// TestCreateWikiNodeShortcutSendsOriginToken 验证 shortcut 创建时请求体包含 origin_node_token
func TestCreateWikiNodeShortcutSendsOriginToken(t *testing.T) {
	createCalled := false
	var gotBody struct {
		NodeType        string `json:"node_type"`
		OriginNodeToken string `json:"origin_node_token"`
		Title           string `json:"title"`
		ObjType         string `json:"obj_type"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "POST" && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-100/nodes":
			createCalled = true
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{
				"code": 0,
				"msg": "ok",
				"data": {
					"node": {
						"space_id": "sp-100",
						"node_token": "wikcnShortcut123",
						"obj_token": "docxRealToken",
						"obj_type": "docx",
						"node_type": "shortcut",
						"origin_node_token": "wikcnOrigin123",
						"title": "快捷方式节点"
					}
				}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initCreateWikiNodeTestConfig(t, server.URL)

	_ = createWikiNodeCmd.Flags().Set("space-id", "sp-100")
	_ = createWikiNodeCmd.Flags().Set("title", "快捷方式节点")
	_ = createWikiNodeCmd.Flags().Set("node-type", "shortcut")
	_ = createWikiNodeCmd.Flags().Set("origin-node-token", "wikcnOrigin123")
	_ = createWikiNodeCmd.Flags().Set("obj-type", "docx")
	defer func() {
		_ = createWikiNodeCmd.Flags().Set("space-id", "")
		_ = createWikiNodeCmd.Flags().Set("title", "")
		_ = createWikiNodeCmd.Flags().Set("node-type", "origin")
		_ = createWikiNodeCmd.Flags().Set("origin-node-token", "")
	}()

	err := createWikiNodeCmd.RunE(createWikiNodeCmd, []string{})
	if err != nil {
		t.Fatalf("createWikiNodeCmd 运行失败: %v", err)
	}

	if !createCalled {
		t.Fatal("未发起 POST /wiki/v2/spaces/:space_id/nodes 请求")
	}
	if gotBody.NodeType != "shortcut" {
		t.Fatalf("请求体 node_type = %q, 期望 shortcut", gotBody.NodeType)
	}
	if gotBody.OriginNodeToken != "wikcnOrigin123" {
		t.Fatalf("请求体 origin_node_token = %q, 期望 wikcnOrigin123", gotBody.OriginNodeToken)
	}
}
