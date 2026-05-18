package cmd

import (
	"strings"
	"testing"
)

// TestBuildEMLRejectsCRLFInHeaders 验证邮件 header value 含 CR/LF 被拒
// 修复 codex review finding #2：防 SMTP header injection
func TestBuildEMLRejectsCRLFInHeaders(t *testing.T) {
	base := mailMessageInput{
		From:    "a@example.com",
		To:      []string{"b@example.com"},
		Subject: "hi",
	}
	cases := []struct {
		name   string
		mutate func(*mailMessageInput)
	}{
		{"subject CRLF", func(m *mailMessageInput) { m.Subject = "hi\r\nBcc: evil@x" }},
		{"from CRLF", func(m *mailMessageInput) { m.From = "a@x\r\nBcc: e@x" }},
		{"from-name CRLF", func(m *mailMessageInput) { m.FromName = "Alice\nMalicious" }},
		{"in-reply-to CRLF", func(m *mailMessageInput) { m.InReplyTo = "<id>\r\nX-Evil: yes" }},
		{"references CRLF", func(m *mailMessageInput) { m.References = "<x>\nX-Evil: yes" }},
		{"to addr CRLF", func(m *mailMessageInput) { m.To = []string{"a@x\r\nBcc: e@x"} }},
		{"cc addr CRLF", func(m *mailMessageInput) { m.CC = []string{"c@x\nX-Evil:1"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			tc.mutate(&m)
			_, err := buildEMLBase64URL(m)
			if err == nil {
				t.Errorf("expected error rejecting CRLF in %s, got nil", tc.name)
				return
			}
			if !strings.Contains(err.Error(), "CR/LF") {
				t.Errorf("error should mention CR/LF, got: %v", err)
			}
		})
	}
}
