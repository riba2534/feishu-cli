package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/registry"
)

// embeddedMethodRoute 返回内嵌 catalog（远端 overlay 关闭）中某方法的 HTTP 动词与完整路径。
// resources 按嵌套链给出，不做点号拆分。
func embeddedMethodRoute(t *testing.T, service string, resources []string, method string) (string, string) {
	t.Helper()
	spec := registry.LoadFromMeta(service)
	if spec == nil {
		t.Fatalf("内嵌 catalog 缺少 service %s", service)
	}
	cur := spec
	for _, r := range resources {
		children, _ := cur["resources"].(map[string]interface{})
		next, _ := children[r].(map[string]interface{})
		if next == nil {
			t.Fatalf("内嵌 catalog 缺少 resource %s.%s", service, strings.Join(resources, "."))
		}
		cur = next
	}
	methods, _ := cur["methods"].(map[string]interface{})
	m, _ := methods[method].(map[string]interface{})
	if m == nil {
		t.Fatalf("内嵌 catalog 缺少 method %s.%s.%s", service, strings.Join(resources, "."), method)
	}
	return registry.GetStrFromMap(m, "httpMethod"), registry.GetStrFromMap(spec, "servicePath") + "/" + registry.GetStrFromMap(m, "path")
}

var catalogPathParam = regexp.MustCompile(`\{[^}]+\}`)

func useEmbeddedCatalogOnly(t *testing.T) {
	t.Helper()
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	registry.ResetForTest()
	t.Cleanup(registry.ResetForTest)
}

// TestSchemaCatalogMatchesApprovalDryRunPaths 审批写命令 --dry-run 预览的请求路径必须与内嵌 catalog 一致，
// 防止内嵌 catalog 回退到已下线的 uat_* 路径（离线 schema + api 透传会打到旧端点）。
func TestSchemaCatalogMatchesApprovalDryRunPaths(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	useEmbeddedCatalogOnly(t)
	cases := []struct {
		resource, method string
		args             []string
	}{
		{"tasks", "approve", []string{"approval", "task", "approve", "--instance-code", "IC", "--task-id", "T"}},
		{"tasks", "reject", []string{"approval", "task", "reject", "--instance-code", "IC", "--task-id", "T"}},
		{"tasks", "transfer", []string{"approval", "task", "transfer", "--instance-code", "IC", "--task-id", "T", "--transfer-user-id", "ou_xxx"}},
		{"tasks", "rollback", []string{"approval", "task", "rollback", "--instance-code", "IC", "--task-id", "T", "--node-ids", "START"}},
		{"tasks", "add_sign", []string{"approval", "task", "add-sign", "--instance-code", "IC", "--task-id", "T", "--type", "after", "--user-ids", "ou_xxx"}},
		{"tasks", "remind", []string{"approval", "task", "remind", "--instance-code", "IC", "--task-ids", "T1"}},
		{"instances", "cancel", []string{"approval", "instance", "cancel", "--instance-code", "IC"}},
		{"instances", "cc", []string{"approval", "instance", "cc", "--instance-code", "IC", "--cc-user-ids", "ou_xxx"}},
	}
	for _, c := range cases {
		stdout, stderr, err := runCLI(t, append(c.args, "--dry-run")...)
		if err != nil {
			t.Fatalf("%v --dry-run 失败: %v\nstderr=%s", c.args, err, stderr)
		}
		var plan struct {
			API []struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"api"`
		}
		if err := json.Unmarshal([]byte(stdout), &plan); err != nil || len(plan.API) == 0 {
			t.Fatalf("%v --dry-run 输出无法解析: %v\n%s", c.args, err, stdout)
		}
		wantVerb, wantPath := embeddedMethodRoute(t, "approval", []string{c.resource}, c.method)
		gotPath := strings.SplitN(plan.API[0].URL, "?", 2)[0]
		if plan.API[0].Method != wantVerb || gotPath != wantPath {
			t.Errorf("approval.%s.%s: 命令请求 %s %s，内嵌 catalog 为 %s %s", c.resource, c.method, plan.API[0].Method, gotPath, wantVerb, wantPath)
		}
	}
}

// TestSchemaCatalogMatchesReadCommandPaths 审批/妙记只读命令实际发出的请求路径与内嵌 catalog 一致。
func TestSchemaCatalogMatchesReadCommandPaths(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	useEmbeddedCatalogOnly(t)

	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	t.Cleanup(srv.Close)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		service   string
		resources []string
		method    string
		pathValue string
		args      []string
	}{
		{"approval", []string{"instances"}, "get", "", []string{"approval", "instance", "get", "--instance-code", "IC"}},
		{"approval", []string{"instances"}, "initiated", "", []string{"approval", "instance", "initiated"}},
		{"approval", []string{"tasks"}, "query", "", []string{"approval", "task", "query", "--topic", "todo"}},
		{"minutes", []string{"minutes"}, "get", "obcnxxxxxx", []string{"minutes", "get", "obcnxxxxxx"}},
	}
	for _, c := range cases {
		mu.Lock()
		seen = nil
		mu.Unlock()
		// 响应体是空 data，命令可能因字段缺失报错；这里只关心发出的第一个业务请求。
		_, _, _ = runCLI(t, append(c.args, "--config", cfg)...)
		wantVerb, wantPath := embeddedMethodRoute(t, c.service, c.resources, c.method)
		if c.pathValue != "" {
			wantPath = catalogPathParam.ReplaceAllString(wantPath, c.pathValue)
		}
		mu.Lock()
		got := append([]string(nil), seen...)
		mu.Unlock()
		if len(got) == 0 {
			t.Errorf("%v 未发出业务请求", c.args)
			continue
		}
		if want := wantVerb + " " + wantPath; got[0] != want {
			t.Errorf("%s.%s.%s: 命令请求 %s，内嵌 catalog 为 %s（全部请求 %v）", c.service, strings.Join(c.resources, "."), c.method, got[0], want, got)
		}
	}
}

// TestSchemaCatalogPrettyShowsSupplements 补丁层生效时 schema status 文本行标出来源与数量；未生效时行尾不变。
func TestSchemaCatalogPrettyShowsSupplements(t *testing.T) {
	base := registry.CatalogInfo{Source: "cache", Brand: "feishu", ServiceCount: 15, MethodCount: 255}
	if got := formatCatalogPretty(base); strings.Contains(got, "supplements") {
		t.Errorf("无补回方法时不应输出 supplements: %s", got)
	}
	base.Supplements = []string{"task.comments.create", "task.comments.list"}
	if got := formatCatalogPretty(base); !strings.HasSuffix(got, " supplements=2(local_supplements)") {
		t.Errorf("应标出补丁层数量与来源: %s", got)
	}
}
