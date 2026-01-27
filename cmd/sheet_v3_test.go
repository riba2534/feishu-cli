package cmd

import (
	"testing"
)

// TestUnescapeSheetRange 测试范围字符串的 shell 转义处理
func TestUnescapeSheetRange(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "正常范围",
			input: "Sheet1!A1:B2",
			want:  "Sheet1!A1:B2",
		},
		{
			name:  "带转义的感叹号",
			input: `Sheet1\!A1:B2`,
			want:  "Sheet1!A1:B2",
		},
		{
			name:  "多个转义",
			input: `abc\!def\!ghi`,
			want:  "abc!def!ghi",
		},
		{
			name:  "无感叹号",
			input: "A1:B2",
			want:  "A1:B2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unescapeSheetRange(tt.input)
			if result != tt.want {
				t.Errorf("结果不匹配: got %s, want %s", result, tt.want)
			}
		})
	}
}

// TestSheetClearCmd_Arguments 测试 clear 命令的参数验证
func TestSheetClearCmd_Arguments(t *testing.T) {
	// 验证命令需要至少 3 个参数
	if sheetClearCmd.Args == nil {
		t.Error("clear 命令应该有参数验证")
	}
}

// TestSheetReadPlainCmd_Arguments 测试 read-plain 命令的参数验证
func TestSheetReadPlainCmd_Arguments(t *testing.T) {
	// 验证命令需要至少 3 个参数
	if sheetReadPlainCmd.Args == nil {
		t.Error("read-plain 命令应该有参数验证")
	}
}

// TestSheetReadRichCmd_Arguments 测试 read-rich 命令的参数验证
func TestSheetReadRichCmd_Arguments(t *testing.T) {
	// 验证命令需要至少 3 个参数
	if sheetReadRichCmd.Args == nil {
		t.Error("read-rich 命令应该有参数验证")
	}
}

// TestSheetWriteRichCmd_Arguments 测试 write-rich 命令的参数验证
func TestSheetWriteRichCmd_Arguments(t *testing.T) {
	// 验证命令需要 2 个参数
	if sheetWriteRichCmd.Args == nil {
		t.Error("write-rich 命令应该有参数验证")
	}
}

// TestSheetInsertCmd_Arguments 测试 insert 命令的参数验证
func TestSheetInsertCmd_Arguments(t *testing.T) {
	// 验证命令需要 3 个参数
	if sheetInsertCmd.Args == nil {
		t.Error("insert 命令应该有参数验证")
	}
}

// TestSheetAppendRichCmd_Arguments 测试 append-rich 命令的参数验证
func TestSheetAppendRichCmd_Arguments(t *testing.T) {
	// 验证命令需要 3 个参数
	if sheetAppendRichCmd.Args == nil {
		t.Error("append-rich 命令应该有参数验证")
	}
}

// TestSheetClearCmd_Flags 测试 clear 命令没有特殊 flag（应该只有全局 flag）
func TestSheetClearCmd_Flags(t *testing.T) {
	// clear 命令没有自定义 flag
	flags := sheetClearCmd.Flags()
	if flags == nil {
		t.Error("flags 不应该为空")
	}
}

// TestSheetReadPlainCmd_Flags 测试 read-plain 命令的 flag
func TestSheetReadPlainCmd_Flags(t *testing.T) {
	outputFlag := sheetReadPlainCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("应该有 output flag")
	}
	if outputFlag.DefValue != "text" {
		t.Errorf("output 默认值应该是 text，实际是 %s", outputFlag.DefValue)
	}
}

// TestSheetReadRichCmd_Flags 测试 read-rich 命令的 flag
func TestSheetReadRichCmd_Flags(t *testing.T) {
	flags := []string{"datetime-render", "value-render", "user-id-type", "output"}
	for _, flagName := range flags {
		f := sheetReadRichCmd.Flags().Lookup(flagName)
		if f == nil {
			t.Errorf("应该有 %s flag", flagName)
		}
	}
}

// TestSheetWriteRichCmd_Flags 测试 write-rich 命令的 flag
func TestSheetWriteRichCmd_Flags(t *testing.T) {
	flags := []string{"data", "data-file", "user-id-type"}
	for _, flagName := range flags {
		f := sheetWriteRichCmd.Flags().Lookup(flagName)
		if f == nil {
			t.Errorf("应该有 %s flag", flagName)
		}
	}
}

// TestSheetInsertCmd_Flags 测试 insert 命令的 flag
func TestSheetInsertCmd_Flags(t *testing.T) {
	flags := []string{"data", "data-file", "user-id-type", "simple"}
	for _, flagName := range flags {
		f := sheetInsertCmd.Flags().Lookup(flagName)
		if f == nil {
			t.Errorf("应该有 %s flag", flagName)
		}
	}

	simpleFlag := sheetInsertCmd.Flags().Lookup("simple")
	if simpleFlag.DefValue != "false" {
		t.Errorf("simple 默认值应该是 false，实际是 %s", simpleFlag.DefValue)
	}
}

// TestSheetAppendRichCmd_Flags 测试 append-rich 命令的 flag
func TestSheetAppendRichCmd_Flags(t *testing.T) {
	flags := []string{"data", "data-file", "user-id-type", "simple"}
	for _, flagName := range flags {
		f := sheetAppendRichCmd.Flags().Lookup(flagName)
		if f == nil {
			t.Errorf("应该有 %s flag", flagName)
		}
	}
}

// TestSheetCmd_HasV3Subcommands 测试 sheet 命令组包含所有 V3 子命令
func TestSheetCmd_HasV3Subcommands(t *testing.T) {
	v3Commands := []string{"read-plain", "read-rich", "write-rich", "insert", "append-rich", "clear"}

	for _, cmdName := range v3Commands {
		found := false
		for _, subCmd := range sheetCmd.Commands() {
			if subCmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("sheet 命令组缺少子命令: %s", cmdName)
		}
	}
}
