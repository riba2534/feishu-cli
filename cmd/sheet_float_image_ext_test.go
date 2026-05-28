package cmd

import "testing"

// TestSheetImageExtSubcommandsRegistered 验证 get/update/media-upload/write-image 注册到 image 组
func TestSheetImageExtSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{"get": false, "update": false, "media-upload": false, "write-image": false}
	for _, sub := range sheetImageCmd.Commands() {
		// Use 字段可能带参数占位（如 "get <...>"），取第一个单词
		name := firstWord(sub.Use)
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for n, ok := range want {
		if !ok {
			t.Errorf("sheet image %s not registered", n)
		}
	}
}

// TestSheetImageGetArgs get 需要 3 个位置参数
func TestSheetImageGetArgs(t *testing.T) {
	if sheetImageGetCmd.Args == nil {
		t.Error("image get 应有参数校验")
	}
	if err := sheetImageGetCmd.Args(sheetImageGetCmd, []string{"t", "s"}); err == nil {
		t.Error("image get 应拒绝 2 个参数")
	}
	if err := sheetImageGetCmd.Args(sheetImageGetCmd, []string{"t", "s", "f"}); err != nil {
		t.Errorf("image get 应接受 3 个参数: %v", err)
	}
}

// TestSheetImageUpdateFlags update flag 注册
func TestSheetImageUpdateFlags(t *testing.T) {
	for _, n := range []string{"range", "width", "height", "offset-x", "offset-y", "output"} {
		if sheetImageUpdateCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on image update", n)
		}
	}
}

// TestSheetImageWriteFlags write-image flag 注册
func TestSheetImageWriteFlags(t *testing.T) {
	for _, n := range []string{"range", "image", "name"} {
		if sheetImageWriteCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing on write-image", n)
		}
	}
}

// TestNormalizeSheetWriteImageRange 验证 write-image 范围被规整为带前缀的单格 cell:cell
func TestNormalizeSheetWriteImageRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		sheetID string
		want    string
	}{
		{"无前缀单格", "A1", "0b1212", "0b1212!A1:A1"},
		{"带前缀单格", "0b1212!B2", "0b1212", "0b1212!B2:B2"},
		{"带前缀且 cell:cell", "0b1212!C3:C3", "0b1212", "0b1212!C3:C3"},
		{"带前缀但写成区间取起始", "0b1212!D4:D9", "0b1212", "0b1212!D4:D4"},
		{"前缀与传入 sheetID 不同时尊重前缀", "abc!E5", "0b1212", "abc!E5:E5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSheetWriteImageRange(tt.input, tt.sheetID)
			if got != tt.want {
				t.Errorf("normalizeSheetWriteImageRange(%q,%q) = %q, want %q", tt.input, tt.sheetID, got, tt.want)
			}
		})
	}
}

// firstWord 取字符串首个空格前的单词。
func firstWord(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return s[:i]
		}
	}
	return s
}
