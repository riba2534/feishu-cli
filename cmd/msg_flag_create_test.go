package cmd

import (
	"testing"
)

// TestMsgFlagCreateCmdRegistered 验证 msg flag create 子命令正确挂载到 msgFlagCmd
func TestMsgFlagCreateCmdRegistered(t *testing.T) {
	if msgFlagCreateCmd.Use != "create <message_id>" {
		t.Fatalf("msgFlagCreateCmd.Use = %q, want %q", msgFlagCreateCmd.Use, "create <message_id>")
	}
	if msgFlagCreateCmd.Short == "" {
		t.Fatal("msgFlagCreateCmd.Short should not be empty")
	}
	// 应在 msgFlagCmd 子命令列表中
	found := false
	for _, sub := range msgFlagCmd.Commands() {
		if sub == msgFlagCreateCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("msgFlagCreateCmd should be registered as child of msgFlagCmd")
	}
}

// TestMsgFlagCreateFlags 校验 flag 注册及默认值
func TestMsgFlagCreateFlags(t *testing.T) {
	itemType := msgFlagCreateCmd.Flags().Lookup("item-type")
	if itemType == nil {
		t.Fatal("--item-type flag missing")
	}
	if itemType.DefValue != "default" {
		t.Fatalf("--item-type default = %q, want %q", itemType.DefValue, "default")
	}
	flagType := msgFlagCreateCmd.Flags().Lookup("flag-type")
	if flagType == nil {
		t.Fatal("--flag-type flag missing")
	}
	if flagType.DefValue != "message" {
		t.Fatalf("--flag-type default = %q, want %q", flagType.DefValue, "message")
	}
	if msgFlagCreateCmd.Flags().Lookup("user-access-token") == nil {
		t.Fatal("--user-access-token flag missing")
	}
}

// TestMsgFlagCreateArgsValidation 校验 Args 必须 ExactArgs(1)
func TestMsgFlagCreateArgsValidation(t *testing.T) {
	// 0 个 args → 报错
	if err := msgFlagCreateCmd.Args(msgFlagCreateCmd, []string{}); err == nil {
		t.Fatal("expected error when no args provided")
	}
	// 1 个 args → OK
	if err := msgFlagCreateCmd.Args(msgFlagCreateCmd, []string{"om_xxx"}); err != nil {
		t.Fatalf("expected no error for 1 arg, got: %v", err)
	}
	// 2 个 args → 报错
	if err := msgFlagCreateCmd.Args(msgFlagCreateCmd, []string{"om_a", "om_b"}); err == nil {
		t.Fatal("expected error when 2 args provided")
	}
}
