package cmd

import (
	"testing"
)

// TestSheetBatchSetStyleRegistered batch-set-style 挂在 sheet 下
func TestSheetBatchSetStyleRegistered(t *testing.T) {
	found := false
	for _, sub := range sheetCmd.Commands() {
		if sub == sheetBatchSetStyleCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("batch-set-style 应挂在 sheet 下")
	}
	if sheetBatchSetStyleCmd.Args == nil {
		t.Error("batch-set-style 应有参数校验")
	}
}

// TestSheetBatchSetStyleFlags --data 必填 flag 注册
func TestSheetBatchSetStyleFlags(t *testing.T) {
	if sheetBatchSetStyleCmd.Flags().Lookup("data") == nil {
		t.Error("--data missing on batch-set-style")
	}
}

// TestParseSheetBatchStyleData 验证 --data JSON 解析为 ranges/style 结构
func TestParseSheetBatchStyleData(t *testing.T) {
	data := `[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true},"backColor":"#FF0000"}}]`
	styles, err := parseSheetBatchStyleData(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(styles) != 1 {
		t.Fatalf("len = %d, want 1", len(styles))
	}
	ranges, ok := styles[0]["ranges"].([]any)
	if !ok || len(ranges) != 1 || ranges[0] != "0b1212!A1:A2" {
		t.Errorf("ranges 解析错误: %v", styles[0]["ranges"])
	}
	style, ok := styles[0]["style"].(map[string]any)
	if !ok {
		t.Fatalf("style 不是 object: %v", styles[0]["style"])
	}
	if style["backColor"] != "#FF0000" {
		t.Errorf("backColor = %v, want #FF0000", style["backColor"])
	}
}

// TestParseSheetBatchStyleDataInvalid 非法 JSON 返回错误
func TestParseSheetBatchStyleDataInvalid(t *testing.T) {
	if _, err := parseSheetBatchStyleData(`{not array}`); err == nil {
		t.Error("非数组应返回错误")
	}
}
