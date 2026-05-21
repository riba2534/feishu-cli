package cmd

import (
	"testing"
)

// TestMsgFlagCancelCmdRegistered 验证 msg flag cancel 子命令正确挂载
func TestMsgFlagCancelCmdRegistered(t *testing.T) {
	if msgFlagCancelCmd.Use != "cancel <message_id>" {
		t.Fatalf("msgFlagCancelCmd.Use = %q, want %q", msgFlagCancelCmd.Use, "cancel <message_id>")
	}
	if msgFlagCancelCmd.Short == "" {
		t.Fatal("msgFlagCancelCmd.Short should not be empty")
	}
	found := false
	for _, sub := range msgFlagCmd.Commands() {
		if sub == msgFlagCancelCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("msgFlagCancelCmd should be registered as child of msgFlagCmd")
	}
}

// TestMsgFlagCancelFlagsDefaults 校验 cancel 默认值与 create 一致
func TestMsgFlagCancelFlagsDefaults(t *testing.T) {
	itemType := msgFlagCancelCmd.Flags().Lookup("item-type")
	if itemType == nil || itemType.DefValue != "default" {
		t.Fatalf("--item-type default = %v, want default", itemType)
	}
	flagType := msgFlagCancelCmd.Flags().Lookup("flag-type")
	if flagType == nil || flagType.DefValue != "message" {
		t.Fatalf("--flag-type default = %v, want message", flagType)
	}
}

// TestMsgFlagCancelArgsValidation 校验 Args 必须 ExactArgs(1)
func TestMsgFlagCancelArgsValidation(t *testing.T) {
	if err := msgFlagCancelCmd.Args(msgFlagCancelCmd, []string{}); err == nil {
		t.Fatal("expected error when no args provided")
	}
	if err := msgFlagCancelCmd.Args(msgFlagCancelCmd, []string{"om_xxx"}); err != nil {
		t.Fatalf("expected no error for 1 arg, got: %v", err)
	}
	if err := msgFlagCancelCmd.Args(msgFlagCancelCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected error when 2 args provided")
	}
}
