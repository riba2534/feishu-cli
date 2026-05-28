package cmd

import (
	"encoding/json"
	"testing"
)

// TestMailSignatureCmdRegistered 验证 signature 子命令注册到 mail 组
func TestMailSignatureCmdRegistered(t *testing.T) {
	if mailSignatureCmd.Use != "signature" {
		t.Fatalf("Use = %q, want signature", mailSignatureCmd.Use)
	}
	found := false
	for _, sub := range mailCmd.Commands() {
		if sub == mailSignatureCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mailSignatureCmd should be child of mailCmd")
	}
	for _, n := range []string{"from", "detail", "dry-run", "output", "user-access-token"} {
		if mailSignatureCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on signature", n)
		}
	}
	if from := mailSignatureCmd.Flags().Lookup("from"); from == nil || from.DefValue != "me" {
		t.Errorf("--from default = %v, want me", from)
	}
	if out := mailSignatureCmd.Flags().Lookup("output"); out != nil && out.Shorthand != "o" {
		t.Errorf("--output shorthand=%q, want o", out.Shorthand)
	}
}

// TestMailSignatureParse 验证签名列表解析（兼容 id / signature_id 字段）
func TestMailSignatureParse(t *testing.T) {
	raw := `{"signatures":[
		{"id":"100","name":"工作签名","is_default":true,"recommended_usage":"subject_and_body"},
		{"signature_id":"200","name":"私人签名","content":"<p>Best</p>"}
	]}`
	var parsed struct {
		Signatures []mailSignatureItem `json:"signatures"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Signatures) != 2 {
		t.Fatalf("got %d signatures, want 2", len(parsed.Signatures))
	}

	// signatureID 兼容两种字段名
	if id := parsed.Signatures[0].signatureID(); id != "100" {
		t.Errorf("first signatureID = %q, want 100", id)
	}
	if id := parsed.Signatures[1].signatureID(); id != "200" {
		t.Errorf("second signatureID = %q, want 200", id)
	}

	// findMailSignature 同时匹配 id 与 signature_id
	if m := findMailSignature(parsed.Signatures, "100"); m == nil || m.Name != "工作签名" {
		t.Errorf("findMailSignature(100) miss, got %v", m)
	}
	if m := findMailSignature(parsed.Signatures, "200"); m == nil || m.Name != "私人签名" {
		t.Errorf("findMailSignature(200) miss, got %v", m)
	}
	if m := findMailSignature(parsed.Signatures, "999"); m != nil {
		t.Errorf("findMailSignature(999) should miss, got %v", m)
	}
}

// TestDisplayMailSignatureName 验证无名字时回退到 ID
func TestDisplayMailSignatureName(t *testing.T) {
	named := mailSignatureItem{ID: "1", Name: "Work"}
	if got := displayMailSignatureName(named); got != "Work" {
		t.Errorf("named = %q, want Work", got)
	}
	noName := mailSignatureItem{SignatureID: "42"}
	if got := displayMailSignatureName(noName); got != "(signature 42)" {
		t.Errorf("noName = %q, want (signature 42)", got)
	}
	empty := mailSignatureItem{}
	if got := displayMailSignatureName(empty); got != "(未命名)" {
		t.Errorf("empty = %q, want (未命名)", got)
	}
}
