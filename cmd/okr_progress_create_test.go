package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

// TestOKRProgressCreateCmdRegistered 验证 okr progress create 子命令注册
func TestOKRProgressCreateCmdRegistered(t *testing.T) {
	if okrProgressCreateCmd.Use != "create" {
		t.Fatalf("okrProgressCreateCmd.Use = %q, want create", okrProgressCreateCmd.Use)
	}
	if okrProgressCreateCmd.Short == "" {
		t.Fatal("okrProgressCreateCmd.Short should not be empty")
	}
	for _, want := range []string{"objective-id", "key-result-id", "content", "content-json", "progress-percent", "progress-status", "user-id-type"} {
		if okrProgressCreateCmd.Flags().Lookup(want) == nil {
			t.Errorf("--%s flag missing", want)
		}
	}
	userIDType := okrProgressCreateCmd.Flags().Lookup("user-id-type")
	if userIDType == nil || userIDType.DefValue != "open_id" {
		t.Fatalf("--user-id-type default = %v, want open_id", userIDType)
	}
	status := okrProgressCreateCmd.Flags().Lookup("progress-status")
	if status == nil || !strings.Contains(status.Usage, "normal / overdue / done") {
		t.Fatalf("--progress-status help 应对齐官方枚举，实际 %v", status)
	}
	if strings.Contains(status.Usage, "risky") {
		t.Fatal("--progress-status help 不得再宣传 risky 写入")
	}
}

// TestPickOKRTarget 校验 --objective-id / --key-result-id 二选一逻辑
func TestPickOKRTarget(t *testing.T) {
	// 都为空 → 报错
	if _, _, err := pickOKRTarget("", ""); err == nil {
		t.Fatal("expected error when both empty")
	}
	// 都填 → 报错
	if _, _, err := pickOKRTarget("obj1", "kr1"); err == nil {
		t.Fatal("expected error when both filled")
	}
	// 仅 objective
	id, typ, err := pickOKRTarget("obj1", "")
	if err != nil || id != "obj1" {
		t.Fatalf("expected (obj1, objective, nil), got (%q, %d, %v)", id, typ, err)
	}
	// 仅 keyresult
	id, typ, err = pickOKRTarget("", "kr1")
	if err != nil || id != "kr1" {
		t.Fatalf("expected (kr1, keyresult, nil), got (%q, %d, %v)", id, typ, err)
	}
}

// TestBuildOKRProgressContentJSON 校验 --content 与 --content-json 互斥 + 包装
func TestBuildOKRProgressContentJSON(t *testing.T) {
	// 都为空 → 报错
	if _, err := buildOKRProgressContentJSON("", ""); err == nil {
		t.Fatal("expected error when both empty")
	}
	// 都填 → 报错
	if _, err := buildOKRProgressContentJSON("hello", `{"blocks":[]}`); err == nil {
		t.Fatal("expected error when both filled")
	}
	// 非法 JSON → 报错
	if _, err := buildOKRProgressContentJSON("", `not-json`); err == nil {
		t.Fatal("expected error when content-json malformed")
	}
	// 合法 JSON 直接透传
	rawJSON := `{"blocks":[{"type":"paragraph"}]}`
	got, err := buildOKRProgressContentJSON("", rawJSON)
	if err != nil || got != rawJSON {
		t.Fatalf("expected raw passthrough, got %q (err=%v)", got, err)
	}
	// 纯文本 → 包装为合法 ContentBlock JSON
	got, err = buildOKRProgressContentJSON("hello", "")
	if err != nil {
		t.Fatalf("expected wrap success, got err %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("wrapped output not valid JSON: %v\nraw: %s", err, got)
	}
	if !strings.Contains(got, `"text":"hello"`) {
		t.Fatalf("wrapped output should include text=hello, got %s", got)
	}
}

func TestParseOKRProgressRateEnum(t *testing.T) {
	rate, err := parseOKRProgressRate("80", "done")
	if err != nil || rate == nil || rate.Status == nil || *rate.Status != client.OKRProgressStatusDone {
		t.Fatalf("done 应写入 wire=2，got rate=%+v err=%v", rate, err)
	}
	rate, err = parseOKRProgressRate("10", "1")
	if err != nil || rate == nil || rate.Status == nil || *rate.Status != client.OKRProgressStatusOverdue {
		t.Fatalf("数字 1 应是 overdue 而非 risky，got rate=%+v err=%v", rate, err)
	}

	_, err = parseOKRProgressRate("50", "risky")
	if err == nil {
		t.Fatal("risky 不得再映射为写入值")
	}
	if !strings.Contains(err.Error(), "已移除写入语义") || !strings.Contains(err.Error(), "done(2)") {
		t.Fatalf("risky 应给出兼容提示而非错误映射，实际: %v", err)
	}

	_, err = parseOKRProgressRate("50", "unknown")
	if err == nil || !strings.Contains(err.Error(), "normal / overdue / done") {
		t.Fatalf("非法枚举应列出官方取值，实际: %v", err)
	}
}
