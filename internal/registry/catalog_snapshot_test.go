package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// useEmbeddedOnly 只加载内嵌 catalog（关闭远端 overlay）。
func useEmbeddedOnly(t *testing.T) {
	t.Helper()
	ResetForTest()
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	t.Cleanup(ResetForTest)
	Init()
}

// 内嵌 catalog 由 scripts/sync_official_catalog.py 从官方 lark-cli v1.0.99 生成；重新同步后按脚本输出更新计数。
const (
	wantEmbeddedServices = 15
	wantEmbeddedMethods  = 256
)

func TestEmbeddedCatalogCounts(t *testing.T) {
	useEmbeddedOnly(t)
	info := Status()
	if info.Source != "embedded" {
		t.Fatalf("source = %q, want embedded", info.Source)
	}
	if info.ServiceCount != wantEmbeddedServices || info.MethodCount != wantEmbeddedMethods {
		t.Fatalf("内嵌 catalog = %d service / %d method，want %d / %d（重新同步后请同步更新常量与文档计数）",
			info.ServiceCount, info.MethodCount, wantEmbeddedServices, wantEmbeddedMethods)
	}
	if len(info.Supplements) != 0 {
		t.Errorf("离线模式不应有补回方法（它们本就在内嵌基线里），got %v", info.Supplements)
	}
}

// TestEmbeddedCatalogHasNoUATPaths 审批 uat_* 是已下线的旧端点，内嵌 catalog 不得残留。
func TestEmbeddedCatalogHasNoUATPaths(t *testing.T) {
	useEmbeddedOnly(t)
	for name, svc := range embeddedServices {
		resources, _ := svc["resources"].(map[string]interface{})
		walkResourceMethods(resources, func(m map[string]interface{}) {
			if p := GetStrFromMap(m, "path"); strings.Contains(p, "uat_") {
				t.Errorf("%s.%s path 仍为旧 uat_ 端点: %s", name, GetStrFromMap(m, "id"), p)
			}
		})
	}
}

// TestEmbeddedApprovalTaskQueryTopicEnum topic=3（已发起）已被官方下线，请求参数枚举不得包含 3。
func TestEmbeddedApprovalTaskQueryTopicEnum(t *testing.T) {
	useEmbeddedOnly(t)
	m := lookupMethod(LoadFromMeta("approval"), []string{"tasks"}, "query")
	if m == nil {
		t.Fatal("内嵌 catalog 缺少 approval.tasks.query")
	}
	if p := GetStrFromMap(m, "path"); p != "tasks" {
		t.Errorf("approval.tasks.query path = %q, want tasks", p)
	}
	params, _ := m["parameters"].(map[string]interface{})
	topic, _ := params["topic"].(map[string]interface{})
	if topic == nil {
		t.Fatal("approval.tasks.query 缺少 topic 参数")
	}
	var values []string
	for _, key := range []string{"options", "enum"} {
		list, _ := topic[key].([]interface{})
		for _, v := range list {
			if om, ok := v.(map[string]interface{}); ok {
				v = om["value"]
			}
			b, _ := json.Marshal(v)
			values = append(values, strings.Trim(string(b), `"`))
		}
	}
	if len(values) == 0 {
		t.Fatal("topic 参数没有枚举值")
	}
	for _, v := range values {
		if v == "3" {
			t.Fatalf("topic 枚举不应包含已下线的 3: %v", values)
		}
	}
}

// TestEmbeddedCatalogKeyMethods 离线可查的关键方法（此前内嵌基线缺失、只能联网 overlay 获得）。
// resource 键本身可能带点（如 "xml_presentation.history"），按 catalog 原样给出。
func TestEmbeddedCatalogKeyMethods(t *testing.T) {
	useEmbeddedOnly(t)
	cases := []struct {
		service   string
		resources []string
		method    string
		path      string
	}{
		{"approval", []string{"tasks"}, "approve", "tasks/pass"},
		{"approval", []string{"instances"}, "get", "instances/detail"},
		{"slides", []string{"xml_presentation.history"}, "list", ""},
		{"slides", []string{"xml_presentation.slide"}, "replace", ""},
		{"mindnotes", []string{"nodes"}, "list", ""},
		{"okr", []string{"cycles"}, "list", ""},
		{"okr", []string{"objectives"}, "get", ""},
		{"okr", []string{"cycles"}, "objectives_weight", ""},
		{"task", []string{"sections"}, "create", ""},
		{"task", []string{"comments"}, "list", "comments"},
		{"drive", []string{"permission.public"}, "get", ""},
		{"im", []string{"threads"}, "forward", ""},
		{"im", []string{"messages"}, "read_status", ""},
		{"mail", []string{"user_mailbox.templates"}, "create", ""},
	}
	for _, c := range cases {
		m := lookupMethod(LoadFromMeta(c.service), c.resources, c.method)
		id := c.service + "." + strings.Join(c.resources, ".") + "." + c.method
		if m == nil {
			t.Errorf("离线 catalog 缺少 %s", id)
			continue
		}
		if c.path != "" && GetStrFromMap(m, "path") != c.path {
			t.Errorf("%s path = %q, want %q", id, GetStrFromMap(m, "path"), c.path)
		}
	}
}

// TestLocalSupplementsDefinedInEmbedded 白名单里的每个方法都必须能在内嵌基线里找到定义。
func TestLocalSupplementsDefinedInEmbedded(t *testing.T) {
	useEmbeddedOnly(t)
	sups := loadLocalSupplements()
	if len(sups) == 0 {
		t.Fatal("local_supplements.json 为空或解析失败")
	}
	for _, s := range sups {
		if strings.TrimSpace(s.Reason) == "" {
			t.Errorf("%s 缺少 reason", s.ID())
		}
		if lookupMethod(embeddedServices[s.Service], s.Resources, s.Method) == nil {
			t.Errorf("%s 不在内嵌 catalog 中", s.ID())
		}
	}
}

// remoteTaskWithoutComments 模拟公开 meta 端点：task service 没有 comments，
// 但带一个与内嵌同名的 tasks.get（用于验证远端优先）。
const remoteTaskWithoutComments = `{"msg":"succeeded","data":{"version":"1.0.0","services":[{"name":"task","version":"v2","title":"task","servicePath":"/open-apis/task/v2","resources":{"tasks":{"methods":{"get":{"id":"tasks.get","path":"tasks/{task_guid}","httpMethod":"GET","description":"remote-tasks-get"}}}}}]}}`

// TestLocalSupplementsSurviveOverlay overlay 按 service 整体替换后，白名单方法从内嵌补回；
// 非白名单的内嵌独有方法不补回（防止陈旧方法回流）；远端已有的方法以远端为准。
func TestLocalSupplementsSurviveOverlay(t *testing.T) {
	isolateRemote(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(remoteTaskWithoutComments))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	Init()
	if overlaySource != "runtime" {
		t.Fatalf("overlaySource = %q, want runtime", overlaySource)
	}
	task := LoadFromMeta("task")
	for _, s := range loadLocalSupplements() {
		if s.Service != "task" {
			continue
		}
		if lookupMethod(task, s.Resources, s.Method) == nil {
			t.Errorf("overlay 后应补回 %s", s.ID())
		}
	}
	if lookupMethod(task, []string{"sections"}, "create") != nil {
		t.Error("非白名单的内嵌独有方法 task.sections.create 不应被补回")
	}
	if got := GetStrFromMap(lookupMethod(task, []string{"tasks"}, "get"), "description"); got != "remote-tasks-get" {
		t.Errorf("远端已有的 tasks.get 应以远端为准，got description=%q", got)
	}
	info := Status()
	if len(info.Supplements) != 5 || !strings.HasPrefix(info.Supplements[0], "task.comments.") {
		t.Errorf("Status().Supplements = %v，want 5 个 task.comments.*", info.Supplements)
	}

	// 下一进程走 cache overlay，同样补回
	reinitKeepingHooks()
	Init()
	if overlaySource != "cache" {
		t.Fatalf("overlaySource = %q, want cache", overlaySource)
	}
	if lookupMethod(LoadFromMeta("task"), []string{"comments"}, "patch") == nil {
		t.Error("cache overlay 后应补回 task.comments.patch")
	}
}

// TestLocalSupplementsSkippedWhenRemoteHasMethod 远端已提供白名单方法时不补、不计数。
func TestLocalSupplementsSkippedWhenRemoteHasMethod(t *testing.T) {
	isolateRemote(t)
	payload := `{"msg":"succeeded","data":{"version":"1.0.0","services":[{"name":"task","servicePath":"/open-apis/task/v2","resources":{"comments":{"methods":{"create":{"path":"comments","httpMethod":"POST","description":"remote"},"list":{"path":"comments","httpMethod":"GET"},"get":{"path":"comments/{comment_id}","httpMethod":"GET"},"patch":{"path":"comments/{comment_id}","httpMethod":"PATCH"},"delete":{"path":"comments/{comment_id}","httpMethod":"DELETE"}}}}}]}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if got := GetStrFromMap(lookupMethod(LoadFromMeta("task"), []string{"comments"}, "create"), "description"); got != "remote" {
		t.Errorf("远端已有 comments.create 时应保留远端定义，got %q", got)
	}
	if n := len(Status().Supplements); n != 0 {
		t.Errorf("远端已覆盖全部白名单时 Supplements 应为空，got %d", n)
	}
}
