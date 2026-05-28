package cmd

import "testing"

// TestSheetFilterViewExtSubcommandsRegistered 验证 get/update/condition 注册到 filter-view 组
func TestSheetFilterViewExtSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{"get": false, "update": false, "condition": false}
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

// TestSheetFilterViewGetFlags get flag 注册
func TestSheetFilterViewGetFlags(t *testing.T) {
	for _, n := range []string{"token", "spreadsheet-token", "sheet-id", "filter-view-id", "output"} {
		if sheetFilterViewGetCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on filter-view get", n)
		}
	}
}

// TestSheetFilterViewUpdateFlags update flag 注册
func TestSheetFilterViewUpdateFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "filter-view-id", "name", "range"} {
		if sheetFilterViewUpdateCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on filter-view update", n)
		}
	}
}
