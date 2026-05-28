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

// TestValidateFloatImageUpdate 验证尺寸/偏移边界校验与 help 声明一致：
// width/height 仅在显式设置时校验 ≥20，offset 校验 ≥0。
func TestValidateFloatImageUpdate(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name             string
		widthChanged     bool
		width            float64
		heightChanged    bool
		height           float64
		offsetX, offsetY *float64
		wantErr          bool
	}{
		{"全合法", true, 200, true, 150, f(0), f(5), false},
		{"width 未设置不校验(即使 0)", false, 0, false, 0, nil, nil, false},
		{"width 显式设置但 <20 报错", true, 10, false, 0, nil, nil, true},
		{"width 显式设置 =20 通过", true, 20, false, 0, nil, nil, false},
		{"height 显式设置但 <20 报错", false, 0, true, 5, nil, nil, true},
		{"height 显式设置 =20 通过", false, 0, true, 20, nil, nil, false},
		{"offset-x 负数报错", false, 0, false, 0, f(-1), nil, true},
		{"offset-y 负数报错", false, 0, false, 0, nil, f(-3), true},
		{"offset 为 0 合法", false, 0, false, 0, f(0), f(0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFloatImageUpdate(tc.widthChanged, tc.width, tc.heightChanged, tc.height, tc.offsetX, tc.offsetY)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
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
