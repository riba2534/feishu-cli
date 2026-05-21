package cmd

import (
	"testing"
)

// TestMsgFlagListCmdRegistered 验证 msg flag list 子命令正确挂载
func TestMsgFlagListCmdRegistered(t *testing.T) {
	if msgFlagListCmd.Use != "list" {
		t.Fatalf("msgFlagListCmd.Use = %q, want %q", msgFlagListCmd.Use, "list")
	}
	if msgFlagListCmd.Short == "" {
		t.Fatal("msgFlagListCmd.Short should not be empty")
	}
	found := false
	for _, sub := range msgFlagCmd.Commands() {
		if sub == msgFlagListCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("msgFlagListCmd should be registered as child of msgFlagCmd")
	}
}

// TestMsgFlagListFlags 校验 list 命令的分页 flag
func TestMsgFlagListFlags(t *testing.T) {
	pageSize := msgFlagListCmd.Flags().Lookup("page-size")
	if pageSize == nil {
		t.Fatal("--page-size flag missing")
	}
	if pageSize.DefValue != "50" {
		t.Fatalf("--page-size default = %q, want %q", pageSize.DefValue, "50")
	}
	pageToken := msgFlagListCmd.Flags().Lookup("page-token")
	if pageToken == nil {
		t.Fatal("--page-token flag missing")
	}
	if pageToken.DefValue != "" {
		t.Fatalf("--page-token default should be empty, got %q", pageToken.DefValue)
	}
	if msgFlagListCmd.Flags().Lookup("user-access-token") == nil {
		t.Fatal("--user-access-token flag missing")
	}
}
