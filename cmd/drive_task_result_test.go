package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

func initDriveTaskResultTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\nuser_access_token: u-test-user\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// TestDriveTaskResultWikiDeleteNodeScenario 验证 drive task-result 支持 --scenario wiki_delete_node 并正确轮询与上报状态
func TestDriveTaskResultWikiDeleteNodeScenario(t *testing.T) {
	pollCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/open-apis/wiki/v2/tasks/task-wiki-del-123":
			pollCalled = true
			if r.URL.Query().Get("task_type") != "delete_node" {
				http.Error(w, "task_type must be delete_node", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprint(w, `{
				"code": 0, "msg": "ok",
				"data": {
					"task": {
						"task_id": "task-wiki-del-123",
						"simple_task_result": {
							"status": "success",
							"status_msg": "deleted"
						}
					}
				}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDriveTaskResultTestConfig(t, server.URL)

	_ = driveTaskResultCmd.Flags().Set("scenario", "wiki_delete_node")
	_ = driveTaskResultCmd.Flags().Set("task-id", "task-wiki-del-123")
	_ = driveTaskResultCmd.Flags().Set("user-access-token", "u-test-user")
	defer func() {
		_ = driveTaskResultCmd.Flags().Set("scenario", "")
		_ = driveTaskResultCmd.Flags().Set("task-id", "")
		_ = driveTaskResultCmd.Flags().Set("user-access-token", "")
	}()

	err := driveTaskResultCmd.RunE(driveTaskResultCmd, []string{})
	if err != nil {
		t.Fatalf("driveTaskResultCmd 运行失败: %v", err)
	}

	if !pollCalled {
		t.Fatal("未发起 wiki delete_node task 查询")
	}
}

// TestDriveTaskResultLocalValidationPrecedesAuth 验证本地参数校验前置于身份解析，非法输入零网络、零 token 刷新
func TestDriveTaskResultLocalValidationPrecedesAuth(t *testing.T) {
	// 指向无效地址，确保如果有任何网络请求必然连接失败
	initDriveTaskResultTestConfig(t, "http://127.0.0.1:59999")

	// 1. 非法 scenario
	_ = driveTaskResultCmd.Flags().Set("scenario", "invalid_scenario")
	err1 := driveTaskResultCmd.RunE(driveTaskResultCmd, []string{})
	if err1 == nil {
		t.Fatal("非法 scenario 必须报错")
	}

	// 2. 缺少 task-id
	_ = driveTaskResultCmd.Flags().Set("scenario", "wiki_delete_node")
	_ = driveTaskResultCmd.Flags().Set("task-id", "")
	err2 := driveTaskResultCmd.RunE(driveTaskResultCmd, []string{})
	if err2 == nil {
		t.Fatal("缺少 task-id 必须报错")
	}

	// 3. 非法 task-id (路径穿越 ..)
	_ = driveTaskResultCmd.Flags().Set("task-id", "../task_escape")
	err3 := driveTaskResultCmd.RunE(driveTaskResultCmd, []string{})
	if err3 == nil {
		t.Fatal("非法 task-id 必须报错")
	}
}

// TestDriveTaskResultAsBotSupport 验证 drive task-result 支持 --as bot 模式走 Tenant Token 查询
func TestDriveTaskResultAsBotSupport(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot-token","expire":7200}`)
		case r.Method == "GET" && r.URL.Path == "/open-apis/wiki/v2/tasks/task-bot-123":
			gotAuth = r.Header.Get("Authorization")
			_, _ = fmt.Fprint(w, `{
				"code": 0, "msg": "ok",
				"data": {
					"task": {
						"task_id": "task-bot-123",
						"simple_task_result": {
							"status": "success"
						}
					}
				}
			}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDriveTaskResultTestConfig(t, server.URL)

	_ = driveTaskResultCmd.Flags().Set("scenario", "wiki_delete_node")
	_ = driveTaskResultCmd.Flags().Set("task-id", "task-bot-123")
	_ = driveTaskResultCmd.Flags().Set("as", "bot")
	defer func() {
		_ = driveTaskResultCmd.Flags().Set("scenario", "")
		_ = driveTaskResultCmd.Flags().Set("task-id", "")
		_ = driveTaskResultCmd.Flags().Set("as", "auto")
	}()

	err := driveTaskResultCmd.RunE(driveTaskResultCmd, []string{})
	if err != nil {
		t.Fatalf("driveTaskResultCmd 运行失败: %v", err)
	}

	if !strings.HasPrefix(gotAuth, "Bearer t-") {
		t.Fatalf("Authorization = %q, 期望以 Tenant Token (Bearer t-...) 发起请求代表 Bot 身份，绝不使用 User Token", gotAuth)
	}
}

// runTaskResultJSON 执行 drive task-result -o json 并返回解析后的输出。
func runTaskResultJSON(t *testing.T, scenario, taskID string) (map[string]any, error) {
	t.Helper()
	resetDriveCmdFlags(t, driveTaskResultCmd)
	_ = driveTaskResultCmd.Flags().Set("scenario", scenario)
	_ = driveTaskResultCmd.Flags().Set("task-id", taskID)
	_ = driveTaskResultCmd.Flags().Set("user-access-token", "u-test-user")
	_ = driveTaskResultCmd.Flags().Set("output", "json")
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := driveTaskResultCmd.RunE(driveTaskResultCmd, nil)
	w.Close()
	os.Stdout = oldStdout
	raw, _ := io.ReadAll(r)
	if runErr != nil {
		return nil, runErr
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, raw)
	}
	return out, nil
}

// P0：task_check 删除任务返回 "fail" 时必须视为失败（旧实现只认 "failed"，删除失败一直显示进行中）。
func TestDriveTaskResultTaskCheckFailIsTerminal(t *testing.T) {
	for _, st := range []string{"fail", "failed", "success", "process"} {
		st := st
		t.Run(st, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"data":{"status":%q}}`, st)
			}))
			defer server.Close()
			initDriveTaskResultTestConfig(t, server.URL)
			out, err := runTaskResultJSON(t, "task_check", "task-1")
			if err != nil {
				t.Fatal(err)
			}
			wantFailed := st == "fail" || st == "failed"
			if out["failed"] != wantFailed || out["ready"] != (st == "success") || out["pending"] != (st == "process") {
				t.Fatalf("status=%s → %+v", st, out)
			}
		})
	}
}

// P1：task-result 支持 wiki_move / wiki_move_to_drive / wiki_delete_space（wiki move-to-drive、delete-space 超时提示的续查命令可用）。
func TestDriveTaskResultWikiScenarios(t *testing.T) {
	cases := []struct {
		scenario string
		taskType string
		body     string
		check    func(map[string]any) bool
	}{
		{"wiki_move", "move", `{"code":0,"data":{"task":{"task_id":"t1","move_result":[{"status":0,"status_msg":"success","node":{"node_token":"wikNew","space_id":"sp"}}]}}}`,
			func(o map[string]any) bool { return o["ready"] == true && o["wiki_token"] == "wikNew" }},
		{"wiki_move", "move", `{"code":0,"data":{"task":{"task_id":"t1","move_result":[{"status":0},{"status":-1,"status_msg":"no perm"}]}}}`,
			func(o map[string]any) bool { return o["failed"] == true && o["status_msg"] == "no perm" }},
		{"wiki_move_to_drive", "move_wiki_to_docs", `{"code":0,"data":{"task":{"task_id":"t1","move_wiki_to_docs_result":{"status":0,"obj_token":"boxObj","obj_type":"docx","url":"https://example.feishu.cn/docx/boxObj"}}}}`,
			func(o map[string]any) bool { return o["ready"] == true && o["obj_token"] == "boxObj" }},
		{"wiki_move_to_drive", "move_wiki_to_docs", `{"code":0,"data":{"task":{"task_id":"t1","move_wiki_to_docs_result":{"status":1}}}}`,
			func(o map[string]any) bool { return o["pending"] == true }},
		{"wiki_delete_space", "delete_space", `{"code":0,"data":{"task":{"task_id":"t1","delete_space_result":{"status":"failure","status_msg":"denied"}}}}`,
			func(o map[string]any) bool { return o["failed"] == true && o["status"] == "failure" }},
	}
	for _, c := range cases {
		c := c
		t.Run(c.scenario, func(t *testing.T) {
			var gotType string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/open-apis/wiki/v2/tasks/t1" {
					http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
					return
				}
				gotType = r.URL.Query().Get("task_type")
				_, _ = io.WriteString(w, c.body)
			}))
			defer server.Close()
			initDriveTaskResultTestConfig(t, server.URL)
			out, err := runTaskResultJSON(t, c.scenario, "t1")
			if err != nil {
				t.Fatal(err)
			}
			if gotType != c.taskType {
				t.Fatalf("task_type = %q, want %q", gotType, c.taskType)
			}
			if !c.check(out) {
				t.Fatalf("输出不符: %+v", out)
			}
		})
	}
}

// 业务码随 HTTP 400 下发时保留业务码（不再是 "HTTP 400, body: ..."）。
func TestDriveTaskResultTaskCheckHTTP400BusinessCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":1061002,"msg":"params error"}`)
	}))
	defer server.Close()
	initDriveTaskResultTestConfig(t, server.URL)
	_, err := runTaskResultJSON(t, "task_check", "t1")
	if err == nil || !strings.Contains(err.Error(), "code=1061002") || strings.Contains(err.Error(), "HTTP 400, body") {
		t.Fatalf("err = %v", err)
	}
}
