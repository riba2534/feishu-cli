package event

import (
	"strings"
	"testing"
)

func TestApplyDotPath_Identity(t *testing.T) {
	body := []byte(`{"a":1,"b":{"c":"hi"}}`)
	got, ok := applyDotPath(body, ".")
	if !ok {
		t.Fatal(". 应总是命中")
	}
	if string(got) != string(body) {
		t.Errorf(". 应返回原始 body 完全相同")
	}
}

func TestApplyDotPath_Nested(t *testing.T) {
	body := []byte(`{"header":{"event_id":"abc"},"event":{"message":{"message_id":"om_x"}}}`)
	got, ok := applyDotPath(body, ".event.message")
	if !ok {
		t.Fatal(".event.message 应命中")
	}
	if !strings.Contains(string(got), "om_x") {
		t.Errorf("子树缺少 om_x: %s", got)
	}
}

func TestApplyDotPath_Miss(t *testing.T) {
	body := []byte(`{"a":1}`)
	_, ok := applyDotPath(body, ".no.such.path")
	if ok {
		t.Errorf("不存在的路径应返回 ok=false")
	}
}

func TestApplyDotPath_RejectsNonDot(t *testing.T) {
	body := []byte(`{"a":1}`)
	_, ok := applyDotPath(body, "select(.a==1)")
	if ok {
		t.Errorf("非 . 开头的复杂 jq 表达式应直接拒绝（不假装支持）")
	}
}

func TestApplyDotPath_LeafScalar(t *testing.T) {
	body := []byte(`{"header":{"event_id":"abc"}}`)
	got, ok := applyDotPath(body, ".header.event_id")
	if !ok {
		t.Fatal("叶子标量应可访问")
	}
	if string(got) != `"abc"` {
		t.Errorf("期望字符串 \"abc\"，实际 %s", got)
	}
}

func TestIsCompactJSON(t *testing.T) {
	if !isCompactJSON([]byte(`{"a":1}`)) {
		t.Errorf("无换行的 JSON 应为 compact")
	}
	if isCompactJSON([]byte("{\n  \"a\": 1\n}")) {
		t.Errorf("含换行的 JSON 不应为 compact")
	}
}

func TestIsContextCanceled(t *testing.T) {
	if !isContextCanceled(errString("context canceled")) {
		t.Errorf(`"context canceled" 应判定为 context 退出`)
	}
	if !isContextCanceled(errString("context deadline exceeded")) {
		t.Errorf(`"context deadline exceeded" 应判定为 context 退出`)
	}
	if isContextCanceled(errString("network unreachable")) {
		t.Errorf("无关错误不应判定为 context 退出")
	}
	if isContextCanceled(nil) {
		t.Errorf("nil error 不应判定为 context 退出")
	}
}

// errString 是测试用的简易 error 实现
type errString string

func (e errString) Error() string { return string(e) }
