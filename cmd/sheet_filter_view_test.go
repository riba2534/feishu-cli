package cmd

import "testing"

// TestSheetFilterViewCmdRegistered 验证 filter-view 父命令注册
func TestSheetFilterViewCmdRegistered(t *testing.T) {
	if sheetFilterViewCmd.Use != "filter-view" {
		t.Fatalf("Use = %q, want filter-view", sheetFilterViewCmd.Use)
	}
	found := false
	for _, sub := range sheetCmd.Commands() {
		if sub == sheetFilterViewCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sheetFilterViewCmd should be child of sheetCmd")
	}
}

// TestSheetFilterViewSubcommandsRegistered 验证 create/list/delete 子命令都注册
func TestSheetFilterViewSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{"create": false, "list": false, "delete": false}
	for _, sub := range sheetFilterViewCmd.Commands() {
		if _, ok := want[sub.Use]; ok {
			want[sub.Use] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("sheet filter-view %s not registered", n)
		}
	}
}

// TestSheetFilterViewCreateFlags 验证 create flag 注册
func TestSheetFilterViewCreateFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "range", "name", "filter-view-id", "output"} {
		if sheetFilterViewCreateCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on create", n)
		}
	}
}

// TestSheetFilterViewListFlags 验证 list flag 注册
func TestSheetFilterViewListFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "output"} {
		if sheetFilterViewListCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on list", n)
		}
	}
}

// TestSheetFilterViewDeleteFlags 验证 delete flag 注册
func TestSheetFilterViewDeleteFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "filter-view-id"} {
		if sheetFilterViewDeleteCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on delete", n)
		}
	}
}
