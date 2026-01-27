package client

import (
	"encoding/json"
	"testing"
)

// TestConvertSimpleToV3Values 测试简单二维数组转换为 V3 格式
func TestConvertSimpleToV3Values(t *testing.T) {
	tests := []struct {
		name     string
		input    [][]interface{}
		expected int // 期望的行数
	}{
		{
			name:     "空数组",
			input:    [][]interface{}{},
			expected: 0,
		},
		{
			name:     "单行单列",
			input:    [][]interface{}{{"hello"}},
			expected: 1,
		},
		{
			name:     "多行多列",
			input:    [][]interface{}{{"a", "b"}, {"c", "d"}},
			expected: 2,
		},
		{
			name:     "混合类型",
			input:    [][]interface{}{{"text", 123, 45.6, true}},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertSimpleToV3Values(tt.input)
			if len(result) != tt.expected {
				t.Errorf("行数不匹配: got %d, want %d", len(result), tt.expected)
			}

			// 验证列数
			for i, row := range result {
				if len(row) != len(tt.input[i]) {
					t.Errorf("第 %d 行列数不匹配: got %d, want %d", i, len(row), len(tt.input[i]))
				}
			}
		})
	}
}

// TestConvertToV3Element 测试单个值转换为 V3 元素
func TestConvertToV3Element(t *testing.T) {
	tests := []struct {
		name         string
		input        interface{}
		expectedType string
	}{
		{
			name:         "字符串转文本",
			input:        "hello",
			expectedType: "text",
		},
		{
			name:         "整数转数值",
			input:        123,
			expectedType: "value",
		},
		{
			name:         "浮点数转数值",
			input:        45.6,
			expectedType: "value",
		},
		{
			name:         "布尔值true转文本",
			input:        true,
			expectedType: "text",
		},
		{
			name:         "布尔值false转文本",
			input:        false,
			expectedType: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToV3Element(tt.input)
			if result.Type != tt.expectedType {
				t.Errorf("类型不匹配: got %s, want %s", result.Type, tt.expectedType)
			}

			// 验证内容
			switch tt.expectedType {
			case "text":
				if result.Text == nil {
					t.Error("text 字段为空")
				}
			case "value":
				if result.Value == nil {
					t.Error("value 字段为空")
				}
			}
		})
	}
}

// TestConvertToV3ElementBoolValues 测试布尔值转换的具体值
func TestConvertToV3ElementBoolValues(t *testing.T) {
	trueElem := ConvertToV3Element(true)
	if trueElem.Text.Text != "TRUE" {
		t.Errorf("true 转换结果不正确: got %s, want TRUE", trueElem.Text.Text)
	}

	falseElem := ConvertToV3Element(false)
	if falseElem.Text.Text != "FALSE" {
		t.Errorf("false 转换结果不正确: got %s, want FALSE", falseElem.Text.Text)
	}
}

// TestCellElementJSON 测试 CellElement 的 JSON 序列化
func TestCellElementJSON(t *testing.T) {
	elem := &CellElement{
		Type: "text",
		Text: &TextElement{
			Text: "hello",
		},
	}

	data, err := json.Marshal(elem)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var parsed CellElement
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if parsed.Type != "text" {
		t.Errorf("类型不匹配: got %s, want text", parsed.Type)
	}
	if parsed.Text == nil || parsed.Text.Text != "hello" {
		t.Error("文本内容不匹配")
	}
}

// TestValueRangeV3JSON 测试 ValueRangeV3 的 JSON 序列化
func TestValueRangeV3JSON(t *testing.T) {
	vr := &ValueRangeV3{
		Range: "Sheet1!A1:B2",
		Values: [][][]*CellElement{
			{
				{
					{Type: "text", Text: &TextElement{Text: "A1"}},
				},
				{
					{Type: "value", Value: &ValueElement{Value: "123"}},
				},
			},
		},
	}

	data, err := json.Marshal(vr)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var parsed ValueRangeV3
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if parsed.Range != "Sheet1!A1:B2" {
		t.Errorf("范围不匹配: got %s, want Sheet1!A1:B2", parsed.Range)
	}
	if len(parsed.Values) != 1 {
		t.Errorf("行数不匹配: got %d, want 1", len(parsed.Values))
	}
	if len(parsed.Values[0]) != 2 {
		t.Errorf("列数不匹配: got %d, want 2", len(parsed.Values[0]))
	}
}

// TestCellElementTypes 测试各种元素类型的 JSON 序列化
func TestCellElementTypes(t *testing.T) {
	tests := []struct {
		name string
		elem *CellElement
	}{
		{
			name: "文本元素",
			elem: &CellElement{
				Type: "text",
				Text: &TextElement{Text: "hello"},
			},
		},
		{
			name: "数值元素",
			elem: &CellElement{
				Type:  "value",
				Value: &ValueElement{Value: "123.45"},
			},
		},
		{
			name: "日期时间元素",
			elem: &CellElement{
				Type:     "date_time",
				DateTime: &DateTimeElement{DateTime: "2024/01/01 10:00"},
			},
		},
		{
			name: "图片元素",
			elem: &CellElement{
				Type:  "image",
				Image: &ImageElement{ImageToken: "img_xxx"},
			},
		},
		{
			name: "链接元素",
			elem: &CellElement{
				Type: "link",
				Link: &LinkElement{Text: "点击", Link: "https://example.com"},
			},
		},
		{
			name: "公式元素",
			elem: &CellElement{
				Type:    "formula",
				Formula: &FormulaElement{Formula: "=SUM(A1:A10)"},
			},
		},
		{
			name: "提醒元素",
			elem: &CellElement{
				Type: "reminder",
				Reminder: &ReminderElement{
					NotifyDateTime: "2024/01/01 10:00",
					NotifyStrategy: 0,
				},
			},
		},
		{
			name: "提及用户元素",
			elem: &CellElement{
				Type: "mention_user",
				MentionUser: &MentionUserElem{
					UserID: "ou_xxx",
					Name:   "张三",
				},
			},
		},
		{
			name: "提及文档元素",
			elem: &CellElement{
				Type: "mention_document",
				MentionDocument: &MentionDocElem{
					ObjectType: "docx",
					Token:      "doc_xxx",
					Title:      "测试文档",
				},
			},
		},
		{
			name: "附件元素",
			elem: &CellElement{
				Type: "file",
				File: &FileElement{
					FileToken: "file_xxx",
					Name:      "test.pdf",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.elem)
			if err != nil {
				t.Fatalf("JSON 序列化失败: %v", err)
			}

			var parsed CellElement
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("JSON 反序列化失败: %v", err)
			}

			if parsed.Type != tt.elem.Type {
				t.Errorf("类型不匹配: got %s, want %s", parsed.Type, tt.elem.Type)
			}
		})
	}
}

// TestSegmentStyle 测试局部样式
func TestSegmentStyle(t *testing.T) {
	elem := &CellElement{
		Type: "text",
		Text: &TextElement{
			Text: "styled text",
			SegmentStyle: &SegmentStyle{
				Style: &TextStyleV3{
					Bold:      true,
					Italic:    true,
					ForeColor: "#FF0000",
					FontSize:  14,
				},
				AffectedText: "styled",
			},
		},
	}

	data, err := json.Marshal(elem)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var parsed CellElement
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if parsed.Text.SegmentStyle == nil {
		t.Fatal("SegmentStyle 为空")
	}
	if parsed.Text.SegmentStyle.Style == nil {
		t.Fatal("Style 为空")
	}
	if !parsed.Text.SegmentStyle.Style.Bold {
		t.Error("Bold 应该为 true")
	}
	if !parsed.Text.SegmentStyle.Style.Italic {
		t.Error("Italic 应该为 true")
	}
	if parsed.Text.SegmentStyle.Style.ForeColor != "#FF0000" {
		t.Errorf("ForeColor 不匹配: got %s, want #FF0000", parsed.Text.SegmentStyle.Style.ForeColor)
	}
}

// TestParseSheetRange 测试范围解析
func TestParseSheetRange(t *testing.T) {
	tests := []struct {
		input     string
		wantSheet string
		wantRange string
	}{
		{"Sheet1!A1:B2", "Sheet1", "A1:B2"},
		{"A1:B2", "", "A1:B2"},
		{"abc123!C3:D4", "abc123", "C3:D4"},
		{"Sheet1!A:C", "Sheet1", "A:C"},
		{"Sheet1!1:3", "Sheet1", "1:3"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sheet, cellRange := ParseSheetRange(tt.input)
			if sheet != tt.wantSheet {
				t.Errorf("sheetID 不匹配: got %s, want %s", sheet, tt.wantSheet)
			}
			if cellRange != tt.wantRange {
				t.Errorf("cellRange 不匹配: got %s, want %s", cellRange, tt.wantRange)
			}
		})
	}
}

// TestBuildSheetRange 测试范围构建
func TestBuildSheetRange(t *testing.T) {
	tests := []struct {
		sheetID   string
		cellRange string
		want      string
	}{
		{"Sheet1", "A1:B2", "Sheet1!A1:B2"},
		{"", "A1:B2", "A1:B2"},
		{"abc123", "C3:D4", "abc123!C3:D4"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			result := BuildSheetRange(tt.sheetID, tt.cellRange)
			if result != tt.want {
				t.Errorf("结果不匹配: got %s, want %s", result, tt.want)
			}
		})
	}
}

// TestColumnToIndex 测试列字母转索引
func TestColumnToIndex(t *testing.T) {
	tests := []struct {
		col  string
		want int
	}{
		{"A", 0},
		{"B", 1},
		{"Z", 25},
		{"AA", 26},
		{"AB", 27},
		{"AZ", 51},
		{"BA", 52},
	}

	for _, tt := range tests {
		t.Run(tt.col, func(t *testing.T) {
			result := ColumnToIndex(tt.col)
			if result != tt.want {
				t.Errorf("结果不匹配: got %d, want %d", result, tt.want)
			}
		})
	}
}

// TestIndexToColumn 测试索引转列字母
func TestIndexToColumn(t *testing.T) {
	tests := []struct {
		index int
		want  string
	}{
		{0, "A"},
		{1, "B"},
		{25, "Z"},
		{26, "AA"},
		{27, "AB"},
		{51, "AZ"},
		{52, "BA"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			result := IndexToColumn(tt.index)
			if result != tt.want {
				t.Errorf("结果不匹配: got %s, want %s", result, tt.want)
			}
		})
	}
}

// TestColumnIndexRoundTrip 测试列索引转换的往返一致性
func TestColumnIndexRoundTrip(t *testing.T) {
	for i := 0; i < 100; i++ {
		col := IndexToColumn(i)
		idx := ColumnToIndex(col)
		if idx != i {
			t.Errorf("往返不一致: %d -> %s -> %d", i, col, idx)
		}
	}
}
