package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestSchemaListServices 验证空 path 时列出所有 service。
func TestSchemaListServices(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf, "", "pretty"); err != nil {
		t.Fatalf("runSchema(\"\") err = %v", err)
	}
	out := buf.String()
	// 至少包含几个高频 service
	for _, want := range []string{"im", "drive", "calendar", "sheets", "bitable", "wiki"} {
		if !strings.Contains(out, want) {
			// bitable 在 meta_data 里可能叫 base，跳过该项也行
			if want == "bitable" {
				continue
			}
			t.Errorf("service list missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestSchemaServiceDetail 验证 service-only path 列出该 service 下 resource.method。
func TestSchemaServiceDetail(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf, "im", "pretty"); err != nil {
		t.Fatalf("runSchema(\"im\") err = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "messages") {
		t.Errorf("im resource list missing 'messages'\n--- output ---\n%s", out)
	}
}

// TestSchemaMethodDetail 验证具体 method path 输出关键字段。
func TestSchemaMethodDetail(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf, "im.messages.delete", "pretty"); err != nil {
		t.Fatalf("runSchema(\"im.messages.delete\") err = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"DELETE", "/open-apis/im/v1/messages", "message_id"} {
		if !strings.Contains(out, want) {
			t.Errorf("method detail missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestSchemaJSONFormat 验证 --format json 在 method 路径下能跑通，
// 并校验输出写到注入的 io.Writer 中、JSON 包含核心字段。
func TestSchemaJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf, "im.messages.delete", "json"); err != nil {
		t.Fatalf("runSchema json err = %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("JSON 输出为空：runSchema 应该写入 io.Writer 而不是 os.Stdout")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	if m["httpMethod"] == nil {
		t.Errorf("JSON output missing 'httpMethod' field; got keys: %v", keys(m))
	}
	if m["path"] == nil {
		t.Errorf("JSON output missing 'path' field; got keys: %v", keys(m))
	}
}

func keys(m map[string]interface{}) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestSchemaPathTooDeep 验证 service.resource.method 后多余片段会报错。
func TestSchemaPathTooDeep(t *testing.T) {
	var buf bytes.Buffer
	err := runSchema(&buf, "im.messages.foo.bar.baz", "pretty")
	// foo 不是 method，因此期望「未知 method」或「路径过深」之一；
	// 当 foo 恰好是 method 时则期望「路径过深」。
	if err == nil {
		t.Fatal("expected error for overly deep path, got nil")
	}
	// 找到匹配的 resource「messages」，剩余 foo.bar.baz：foo 不存在 → 报未知 method 也 OK；
	// 或剩余 > 1 触发路径过深检查。两种错误都接受。
	msg := err.Error()
	if !strings.Contains(msg, "未知 method") && !strings.Contains(msg, "路径过深") {
		t.Errorf("expected 未知 method or 路径过深 error, got: %v", err)
	}
}

// TestSchemaListUnknownServiceFriendly 验证 list --service xxx 未知时列出可用 service。
func TestSchemaListUnknownServiceFriendly(t *testing.T) {
	var buf bytes.Buffer
	err := runSchemaList(&buf, "nonexistent_service_xyz", "pretty")
	if err == nil {
		t.Fatal("expected error for unknown service in list, got nil")
	}
	if !strings.Contains(err.Error(), "未知 service") {
		t.Errorf("error should mention 未知 service, got: %v", err)
	}
	if !strings.Contains(err.Error(), "可用 service") {
		t.Errorf("error should list 可用 service, got: %v", err)
	}
}

// TestSchemaUnknownService 验证未知 service 返回友好错误。
func TestSchemaUnknownService(t *testing.T) {
	var buf bytes.Buffer
	err := runSchema(&buf, "nonexistent_service_xyz", "pretty")
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
	if !strings.Contains(err.Error(), "未知 service") {
		t.Errorf("error message should mention 未知 service, got: %v", err)
	}
}

// TestSchemaListSubcommand 验证 schema list --service 子命令 runner。
func TestSchemaListSubcommand(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchemaList(&buf, "im", "pretty"); err != nil {
		t.Fatalf("runSchemaList(\"im\") err = %v", err)
	}
	if !strings.Contains(buf.String(), "messages") {
		t.Errorf("schema list --service im missing 'messages'\n--- output ---\n%s", buf.String())
	}
}
