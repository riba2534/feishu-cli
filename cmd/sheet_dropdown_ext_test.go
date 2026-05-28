package cmd

import (
	"reflect"
	"testing"
)

// TestSheetDropdownExtSubcommandsRegistered get/update/delete 注册到 dropdown 组
func TestSheetDropdownExtSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{"get": false, "update": false, "delete": false}
	for _, sub := range sheetDropdownCmd.Commands() {
		if _, ok := want[sub.Use]; ok {
			want[sub.Use] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("sheet dropdown %s not registered", n)
		}
	}
}

// TestSheetDropdownGetFlags get flag 注册
func TestSheetDropdownGetFlags(t *testing.T) {
	for _, n := range []string{"token", "range", "output"} {
		if sheetDropdownGetCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on dropdown get", n)
		}
	}
}

// TestSheetDropdownUpdateFlags update flag 注册
func TestSheetDropdownUpdateFlags(t *testing.T) {
	for _, n := range []string{"token", "sheet-id", "ranges", "options", "options-json", "multiple", "highlight", "colors"} {
		if sheetDropdownUpdateCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on dropdown update", n)
		}
	}
}

// TestSheetDropdownDeleteFlags delete flag 注册
func TestSheetDropdownDeleteFlags(t *testing.T) {
	for _, n := range []string{"token", "ranges"} {
		if sheetDropdownDeleteCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on dropdown delete", n)
		}
	}
}

// TestSplitSheetCSV 验证逗号拆分去空白去空项
func TestSplitSheetCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"单值", "0b1212!A1:A100", []string{"0b1212!A1:A100"}},
		{"多值带空白", " a , b ,c ", []string{"a", "b", "c"}},
		{"含空项", "a,,b,", []string{"a", "b"}},
		{"全空", " , ,", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSheetCSV(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitSheetCSV(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
