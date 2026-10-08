package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// isChildOfMail 判断 cmd 是否已注册到 mail 命令组
func isChildOfMail(c *cobra.Command) bool {
	for _, sub := range mailCmd.Commands() {
		if sub == c {
			return true
		}
	}
	return false
}

// TestMailManageCmdsRegistered 验证三个子命令均注册到 mail 组，Use 名正确
func TestMailManageCmdsRegistered(t *testing.T) {
	cases := []struct {
		cmd *cobra.Command
		use string
	}{
		{mailMessageModifyCmd, "message-modify"},
		{mailDraftSendCmd, "draft-send"},
		{mailMessageTrashCmd, "message-trash"},
	}
	for _, c := range cases {
		if c.cmd.Use != c.use {
			t.Errorf("Use = %q, want %q", c.cmd.Use, c.use)
		}
		if !isChildOfMail(c.cmd) {
			t.Errorf("%s should be child of mailCmd", c.use)
		}
	}
}

// TestMailManageFlags 验证各命令的关键 flag 均已注册
func TestMailManageFlags(t *testing.T) {
	modifyFlags := []string{"mailbox", "message-ids", "add-label-ids", "remove-label-ids", "folder-id", "user-id-type", "output", "user-access-token"}
	for _, n := range modifyFlags {
		if mailMessageModifyCmd.Flags().Lookup(n) == nil {
			t.Errorf("message-modify 缺少 --%s", n)
		}
	}
	sendFlags := []string{"mailbox", "draft-id", "confirm-send", "output", "user-access-token"}
	for _, n := range sendFlags {
		if mailDraftSendCmd.Flags().Lookup(n) == nil {
			t.Errorf("draft-send 缺少 --%s", n)
		}
	}
	trashFlags := []string{"mailbox", "message-ids", "yes", "output", "user-access-token"}
	for _, n := range trashFlags {
		if mailMessageTrashCmd.Flags().Lookup(n) == nil {
			t.Errorf("message-trash 缺少 --%s", n)
		}
	}

	// mailbox 默认 me
	for _, c := range []*cobra.Command{mailMessageModifyCmd, mailMessageTrashCmd} {
		if mb := c.Flags().Lookup("mailbox"); mb == nil || mb.DefValue != "me" {
			t.Errorf("%s --mailbox default = %v, want me", c.Use, mb)
		}
	}
	// output 短横线为 o
	if out := mailMessageModifyCmd.Flags().Lookup("output"); out != nil && out.Shorthand != "o" {
		t.Errorf("--output shorthand = %q, want o", out.Shorthand)
	}
}

// TestParseMailManageMessageIDs 验证 message-ids 解析：去空白、去空项、去重保序，超过 20 不再报错（改为自动分批）
func TestParseMailManageMessageIDs(t *testing.T) {
	ids, err := parseMailManageMessageIDs(" m1, m2 ,,m3 ,m1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Join(ids, ",") != "m1,m2,m3" {
		t.Fatalf("got %v, want [m1 m2 m3]", ids)
	}
	if _, err := parseMailManageMessageIDs("  "); err == nil {
		t.Error("空 message-ids 应报错")
	}
	many := make([]string, 45)
	for i := range many {
		many[i] = fmt.Sprintf("m%02d", i)
	}
	ids, err = parseMailManageMessageIDs(strings.Join(many, ","))
	if err != nil || len(ids) != 45 {
		t.Errorf("超过 20 个应允许（自动分批），got %d %v", len(ids), err)
	}
}

// TestMailManageMaxMessageIDs 固化单批上限常量，避免误改
func TestMailManageMaxMessageIDs(t *testing.T) {
	if mailManageMaxMessageIDs != 20 {
		t.Errorf("mailManageMaxMessageIDs = %d, want 20", mailManageMaxMessageIDs)
	}
}

func TestNormalizeMailManageLabelsAndFolder(t *testing.T) {
	got, err := normalizeMailManageLabels("flagged, Important,unread,FLAGGED,Label_Custom_1", "--add-label-ids")
	if err != nil || strings.Join(got, ",") != "FLAGGED,IMPORTANT,UNREAD,Label_Custom_1" {
		t.Errorf("系统标签应大写并去重、自定义原样: %v %v", got, err)
	}
	for in, want := range map[string]string{"inbox": "INBOX", "archive": "ARCHIVED", "Archived": "ARCHIVED", "7001": "7001", "": ""} {
		if got, err := normalizeMailManageFolder(in); err != nil || got != want {
			t.Errorf("normalizeMailManageFolder(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	if _, err := normalizeMailManageFolder("trash"); err == nil {
		t.Error("TRASH 应被拒绝")
	}
}

// TestMailMessageModify_AutoBatchAndPartialFailure 45 封按 20/20/5 分批；某批失败时输出明细并返回错误。
func TestMailMessageModify_AutoBatchAndPartialFailure(t *testing.T) {
	var batches [][]string
	var labelsSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MessageIDs  []string `json:"message_ids"`
			AddLabelIDs []string `json:"add_label_ids"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		batches = append(batches, body.MessageIDs)
		labelsSeen = body.AddLabelIDs
		w.Header().Set("Content-Type", "application/json")
		if len(batches) == 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":1234,"msg":"bad batch"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	many := make([]string, 45)
	for i := range many {
		many[i] = fmt.Sprintf("m%02d", i)
	}
	cmd := mailMessageModifyCmd
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("message-ids", strings.Join(many, ","))
	_ = cmd.Flags().Set("add-label-ids", "flagged")
	_ = cmd.Flags().Set("output", "json")
	var err error
	out := captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if len(batches) != 3 || len(batches[0]) != 20 || len(batches[1]) != 20 || len(batches[2]) != 5 {
		t.Fatalf("分批不符: %d 批", len(batches))
	}
	if strings.Join(labelsSeen, ",") != "FLAGGED" {
		t.Errorf("系统标签应规范化为大写: %v", labelsSeen)
	}
	if err == nil || !strings.Contains(err.Error(), "部分失败") {
		t.Errorf("部分批次失败应返回错误，得到 %v", err)
	}
	var got struct {
		Success []string         `json:"success_message_ids"`
		Failed  []map[string]any `json:"failed_message_ids"`
	}
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", jerr, out)
	}
	if len(got.Success) != 25 || len(got.Failed) != 20 || got.Failed[0]["message_id"] != "m20" {
		t.Errorf("成功/失败明细不符: success=%d failed=%d", len(got.Success), len(got.Failed))
	}
}
