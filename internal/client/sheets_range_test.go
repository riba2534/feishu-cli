package client

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSplitSheetRangePrefix(t *testing.T) {
	cases := []struct {
		in, sheet, rest string
		ok              bool
	}{
		{"Sheet1!A1:C10", "Sheet1", "A1:C10", true},
		{"0b12!A:C", "0b12", "A:C", true},
		{"数据！A1", "数据", "A1", true},                // 全角分隔符
		{`Sheet1\!A1:B2`, "Sheet1", "A1:B2", true}, // zsh 转义写法
		{"'My Sheet'!A1", "My Sheet", "A1", true},
		{"'a!b'!A1", "a!b", "A1", true},         // 引号内可含 !
		{"'It''s'!B2", "It's", "B2", true},      // '' 还原为 '
		{" Sheet1 ! A1 ", "Sheet1", "A1", true}, // 两侧空白
		{"A1:C10", "", "", false},
		{"!A1", "", "", false},
		{"Sheet1!", "", "", false},
		{"'unclosed!A1", "", "", false},
	}
	for _, c := range cases {
		sheet, rest, ok := SplitSheetRangePrefix(c.in)
		if sheet != c.sheet || rest != c.rest || ok != c.ok {
			t.Errorf("SplitSheetRangePrefix(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, sheet, rest, ok, c.sheet, c.rest, c.ok)
		}
	}
}

func TestLooksLikeA1Range(t *testing.T) {
	for _, s := range []string{"A1", "a1:c10", "A:C", "1:3", "$A$1:$B$2", "AB12", "XFD1"} {
		if !LooksLikeA1Range(s) {
			t.Errorf("LooksLikeA1Range(%q) = false, want true", s)
		}
	}
	// 子表 ID / 子表名不应被当成 A1 写法
	for _, s := range []string{"Sheet1", "19e71f", "数据", "A:3", "A1:B2:C3", "", "0b12!A1"} {
		if LooksLikeA1Range(s) {
			t.Errorf("LooksLikeA1Range(%q) = true, want false", s)
		}
	}
}

func TestParseDimRange(t *testing.T) {
	cases := []struct {
		in   string
		want DimRange
	}{
		{"3:5", DimRange{Major: "ROWS", Start: 3, End: 5}},
		{"3", DimRange{Major: "ROWS", Start: 3, End: 3}},
		{"B:D", DimRange{Major: "COLUMNS", Start: 2, End: 4}},
		{"c", DimRange{Major: "COLUMNS", Start: 3, End: 3}},
		{"AA:AB", DimRange{Major: "COLUMNS", Start: 27, End: 28}},
		{"$2:$4", DimRange{Major: "ROWS", Start: 2, End: 4}},
	}
	for _, c := range cases {
		got, err := ParseDimRange(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseDimRange(%q) = (%+v, %v), want %+v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "0", "0:3", "5:3", "B:3", "A1:B2", "x-y", "1:2:3"} {
		if _, err := ParseDimRange(bad); err == nil {
			t.Errorf("ParseDimRange(%q) 应报错", bad)
		}
	}
	if got := (DimRange{Major: "COLUMNS", Start: 2, End: 4}).A1(); got != "B:D" {
		t.Errorf("A1() = %q, want B:D", got)
	}
	if got := (DimRange{Major: "ROWS", Start: 3, End: 5}).A1(); got != "3:5" {
		t.Errorf("A1() = %q, want 3:5", got)
	}
}

// TestDecodeSheetValues_PreservesNumbers 数字保留原始字面量：1000000 不变成 1e+06，大整数不丢精度。
func TestDecodeSheetValues_PreservesNumbers(t *testing.T) {
	values, err := DecodeSheetValues([]byte(`[[1000000, 1234567890123456789, 0.1, "007", true, null]]`))
	if err != nil {
		t.Fatal(err)
	}
	row := values[0]
	for i, want := range []string{"1000000", "1234567890123456789", "0.1"} {
		n, ok := row[i].(json.Number)
		if !ok || n.String() != want {
			t.Errorf("values[0][%d] = %#v, want json.Number(%s)", i, row[i], want)
		}
	}
	if row[3] != "007" || row[4] != true || row[5] != nil {
		t.Errorf("非数字单元格被改动: %#v", row[3:])
	}
	// 写入请求体中数字原样输出
	body, _ := json.Marshal(convertBoolCells(values))
	if !strings.Contains(string(body), "1234567890123456789") || !strings.Contains(string(body), "1000000") {
		t.Errorf("序列化后数字字面量变化: %s", body)
	}
	if _, err := DecodeSheetValues([]byte(`[[1]] [[2]]`)); err == nil {
		t.Error("尾随多余 JSON 应报错")
	}
}

// TestConvertToV3Element_NumberFormatting V3 value 元素不得出现科学计数法或精度丢失。
func TestConvertToV3Element_NumberFormatting(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{json.Number("1234567890123456789"), "1234567890123456789"},
		{json.Number("1000000"), "1000000"},
		{float64(1000000), "1000000"},
		{float64(1e21), "1000000000000000000000"},
		{0.1, "0.1"},
		{float64(1e-7), "0.0000001"},
		{int64(9007199254740993), "9007199254740993"},
		{42, "42"},
	}
	for _, c := range cases {
		el := ConvertToV3Element(c.in)
		if el.Type != "value" || el.Value == nil || el.Value.Value != c.want {
			t.Errorf("ConvertToV3Element(%#v) = %+v, want value %q", c.in, el.Value, c.want)
		}
	}
}

func TestPlanSheetWriteChunks(t *testing.T) {
	makeRows := func(n, w int) [][]any {
		rows := make([][]any, n)
		for i := range rows {
			rows[i] = make([]any, w)
			for j := range rows[i] {
				rows[i][j] = i
			}
		}
		return rows
	}

	// 未超限：原样单块（range 不改写，保持旧行为）
	chunks, err := PlanSheetWriteChunks("s1!A1:B2", makeRows(2, 2), 0, 0)
	if err != nil || len(chunks) != 1 || chunks[0].Range != "s1!A1:B2" {
		t.Fatalf("未超限应原样返回: %+v %v", chunks, err)
	}

	// 12000 行 × 2 列，以单元格锚点自动分 3 批
	chunks, err = PlanSheetWriteChunks("s1!A1", makeRows(12000, 2), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range chunks {
		got = append(got, c.Range)
	}
	want := []string{"s1!A1:B5000", "s1!A5001:B10000", "s1!A10001:B12000"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("行分批范围 = %v, want %v", got, want)
	}
	if len(chunks[2].Values) != 2000 || chunks[1].Values[0][0] != 5000 {
		t.Errorf("分批数据错位: len=%d first=%v", len(chunks[2].Values), chunks[1].Values[0][0])
	}

	// 列超限（150 列）从 C3 起：拆成 100 + 50 列
	chunks, err = PlanSheetWriteChunks("s1!C3:EV4", makeRows(2, 150), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Range != "s1!C3:CX4" || chunks[1].Range != "s1!CY3:EV4" || len(chunks[1].Values[0]) != 50 {
		t.Errorf("列分批错误: %+v", []string{chunks[0].Range, chunks[1].Range})
	}

	// 整列写法以第 1 行为锚点
	chunks, err = PlanSheetWriteChunks("s1!B:C", makeRows(5001, 2), 0, 0)
	if err != nil || chunks[0].Range != "s1!B1:C5000" || chunks[1].Range != "s1!B5001:C5001" {
		t.Errorf("整列锚点错误: %+v %v", chunks, err)
	}

	// 只有子表 ID：以 A1 为锚点
	chunks, err = PlanSheetWriteChunks("s1", makeRows(5001, 1), 0, 0)
	if err != nil || chunks[0].Range != "s1!A1:A5000" {
		t.Errorf("子表锚点错误: %+v %v", chunks, err)
	}

	// 显式范围小于数据：报错而不是越界写
	if _, err := PlanSheetWriteChunks("s1!A1:B100", makeRows(6000, 2), 0, 0); err == nil {
		t.Error("显式范围行数不足应报错")
	}
	if _, err := PlanSheetWriteChunks("s1!A1:B6000", makeRows(6000, 3), 0, 0); err == nil {
		t.Error("显式范围列数不足应报错")
	}
}

func TestAppendChunkRangeAndSpan(t *testing.T) {
	if got := AppendChunkRange("s1!A:B", "s1!A12001:B17000", 2, 1000); got != "s1!A17001:B18000" {
		t.Errorf("AppendChunkRange = %q", got)
	}
	if got := AppendChunkRange("s1!C1:D2", "s1!C5:D9", 2, 3); got != "s1!C10:D12" {
		t.Errorf("AppendChunkRange 列对齐错误: %q", got)
	}
	if got := AppendChunkRange("s1!A:B", "garbage", 2, 3); got != "s1!A:B" {
		t.Errorf("无法解析时应回退原范围: %q", got)
	}
	if got := SheetRangesSpan([]string{"s1!A1:B5000", "s1!A5001:B10000", "s1!C1:D2"}); got != "s1!A1:D10000" {
		t.Errorf("SheetRangesSpan = %q", got)
	}
	if got := SheetRangesSpan([]string{"s1!A1:B2"}); got != "s1!A1:B2" {
		t.Errorf("单范围 span = %q", got)
	}
}
