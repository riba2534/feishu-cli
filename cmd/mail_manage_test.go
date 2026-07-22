package cmd

import (
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

// TestParseMailManageMessageIDs 验证 message-ids 解析与数量上限校验
func TestParseMailManageMessageIDs(t *testing.T) {
	// 正常：逗号分隔 + 去空白 + 去空项
	ids, err := parseMailManageMessageIDs(" m1, m2 ,,m3 ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ids) != 3 || ids[0] != "m1" || ids[1] != "m2" || ids[2] != "m3" {
		t.Fatalf("got %v, want [m1 m2 m3]", ids)
	}

	// 空输入报错
	if _, err := parseMailManageMessageIDs("  "); err == nil {
		t.Error("空 message-ids 应报错")
	}

	// 超过 20 报错
	many := make([]string, 21)
	for i := range many {
		many[i] = "m"
	}
	if _, err := parseMailManageMessageIDs(strings.Join(many, ",")); err == nil {
		t.Error("超过 20 个 message-ids 应报错")
	}

	// 恰好 20 通过
	twenty := make([]string, 20)
	for i := range twenty {
		twenty[i] = "id"
	}
	if _, err := parseMailManageMessageIDs(strings.Join(twenty, ",")); err != nil {
		t.Errorf("20 个 message-ids 应通过，got %v", err)
	}
}

// TestMailManageMaxMessageIDs 固化批量上限常量，避免误改
func TestMailManageMaxMessageIDs(t *testing.T) {
	if mailManageMaxMessageIDs != 20 {
		t.Errorf("mailManageMaxMessageIDs = %d, want 20", mailManageMaxMessageIDs)
	}
}
