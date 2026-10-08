package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMailTemplateUpdate_MergesWithCurrent 只改主题时，正文/收件人/附件保持原值整体写回。
func TestMailTemplateUpdate_MergesWithCurrent(t *testing.T) {
	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"template":{"template_id":"t1","name":"n","subject":"old","template_content":"<p>body</p>","tos":[{"mail_address":"a@example.com"}],"attachments":[{"id":"f1","filename":"a.pdf","body":"f1","is_inline":false}],"create_time":"1"}}}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		putBody = string(raw)
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"template":{"template_id":"t1","name":"n","subject":"new"}}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	cmd := mailTemplateUpdateCmd
	resetCmdFlags(cmd)
	defer resetCmdFlags(cmd)
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("template-id", "t1")
	_ = cmd.Flags().Set("subject", "new")
	_ = cmd.Flags().Set("output", "json")
	var err error
	captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Template map[string]any `json:"template"`
	}
	if err := json.Unmarshal([]byte(putBody), &got); err != nil {
		t.Fatalf("PUT 体不是 JSON: %v %s", err, putBody)
	}
	if got.Template["template_content"] != "<p>body</p>" {
		t.Errorf("正文应保持原值: %v", got.Template["template_content"])
	}
	for _, want := range []string{`"subject":"new"`, `"a@example.com"`, `"filename":"a.pdf"`} {
		if !strings.Contains(putBody, want) {
			t.Errorf("PUT 体缺少 %s: %s", want, putBody)
		}
	}
	if _, ok := got.Template["template_id"]; ok {
		t.Errorf("PUT 体不应携带 template_id: %s", putBody)
	}
}
