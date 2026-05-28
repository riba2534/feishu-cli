package cmd

import (
	"reflect"
	"testing"
)

// TestSheetConditionGroupRegistered condition 父组挂在 filter-view 下
func TestSheetConditionGroupRegistered(t *testing.T) {
	if sheetFilterViewConditionCmd.Use != "condition" {
		t.Fatalf("Use = %q, want condition", sheetFilterViewConditionCmd.Use)
	}
	found := false
	for _, sub := range sheetFilterViewCmd.Commands() {
		if sub == sheetFilterViewConditionCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("condition 组应挂在 filter-view 下")
	}
}

// TestSheetConditionSubcommandsRegistered create/get/update/delete/list 全注册
func TestSheetConditionSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{"create": false, "get": false, "update": false, "delete": false, "list": false}
	for _, sub := range sheetFilterViewConditionCmd.Commands() {
		if _, ok := want[sub.Use]; ok {
			want[sub.Use] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("sheet filter-view condition %s not registered", n)
		}
	}
}

// TestSheetConditionCreateFlags create flag（含写入字段）注册
func TestSheetConditionCreateFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "filter-view-id", "condition-id", "filter-type", "compare-type", "expected", "output"} {
		if sheetFilterViewConditionCreateCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on condition create", n)
		}
	}
}

// TestSheetConditionListFlags list 不需要 condition-id
func TestSheetConditionListFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "filter-view-id", "output"} {
		if sheetFilterViewConditionListCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on condition list", n)
		}
	}
	if sheetFilterViewConditionListCmd.Flags().Lookup("condition-id") != nil {
		t.Error("condition list 不应注册 --condition-id")
	}
}

// TestSheetConditionDeleteNoOutput delete 不注册 output flag
func TestSheetConditionDeleteNoOutput(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "filter-view-id", "condition-id"} {
		if sheetFilterViewConditionDeleteCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on condition delete", n)
		}
	}
}

// TestParseSheetConditionExpected 验证 expected JSON 数组解析
func TestParseSheetConditionExpected(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"空串返回 nil", "", nil, false},
		{"单值数组", `["6"]`, []string{"6"}, false},
		{"多值数组", `["a","b","c"]`, []string{"a", "b", "c"}, false},
		{"非法 JSON", `[6`, nil, true},
		{"非字符串数组", `[6]`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSheetConditionExpected(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
