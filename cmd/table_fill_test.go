package cmd

import (
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/converter"
)

func TestFlattenExtraRows_Empty(t *testing.T) {
	td := &converter.TableData{Cols: 3}
	contents, elements := flattenExtraRows(td)
	if contents != nil || elements != nil {
		t.Fatalf("无扩展行应返回 nil,nil，得到 contents=%v elements=%v", contents, elements)
	}
}

func TestFlattenExtraRows_PlainText(t *testing.T) {
	td := &converter.TableData{
		Cols: 3,
		ExtraRowContents: [][]string{
			{"a1", "a2", "a3"},
			{"b1", "b2", "b3"},
		},
	}
	contents, elements := flattenExtraRows(td)
	want := []string{"a1", "a2", "a3", "b1", "b2", "b3"}
	if len(contents) != len(want) {
		t.Fatalf("contents 长度 = %d, 期望 %d", len(contents), len(want))
	}
	for i, v := range want {
		if contents[i] != v {
			t.Errorf("contents[%d] = %q, 期望 %q", i, contents[i], v)
		}
	}
	if elements != nil {
		t.Errorf("无富文本元素时应返回 nil，得到 %v", elements)
	}
}

func TestFlattenExtraRows_WithRichElements(t *testing.T) {
	mkEl := func(s string) *larkdocx.TextElement {
		return &larkdocx.TextElement{TextRun: &larkdocx.TextRun{Content: &s}}
	}
	td := &converter.TableData{
		Cols: 2,
		ExtraRowContents: [][]string{
			{"a1", "a2"},
			{"b1", "b2"},
		},
		ExtraRowElements: [][][]*larkdocx.TextElement{
			{{mkEl("A1")}, {mkEl("A2")}},
			{{mkEl("B1")}, {mkEl("B2")}},
		},
	}
	contents, elements := flattenExtraRows(td)
	if len(contents) != 4 {
		t.Fatalf("contents 长度 = %d, 期望 4", len(contents))
	}
	if len(elements) != 4 {
		t.Fatalf("elements 长度 = %d, 期望 4", len(elements))
	}
	// 验证 cell-by-cell 顺序：(row-major)
	for i, want := range []string{"A1", "A2", "B1", "B2"} {
		if len(elements[i]) != 1 {
			t.Fatalf("elements[%d] 应含 1 个 TextElement，实际 %d", i, len(elements[i]))
		}
		got := *elements[i][0].TextRun.Content
		if got != want {
			t.Errorf("elements[%d].Content = %q, 期望 %q", i, got, want)
		}
	}
}

func TestFlattenExtraRows_MissingElementsForSomeRows(t *testing.T) {
	// 边界：ExtraRowContents 有 3 行，但 ExtraRowElements 只有 2 行
	// 应不崩；后续行只有纯文本，无 element 数据
	td := &converter.TableData{
		Cols: 2,
		ExtraRowContents: [][]string{
			{"a1", "a2"},
			{"b1", "b2"},
			{"c1", "c2"},
		},
		ExtraRowElements: [][][]*larkdocx.TextElement{
			{{}, {}},
			{{}, {}},
			// 第 3 行故意不提供
		},
	}
	contents, elements := flattenExtraRows(td)
	if len(contents) != 6 {
		t.Fatalf("contents 长度 = %d, 期望 6", len(contents))
	}
	// elements 只覆盖前 2 行 * 2 列 = 4 个
	if len(elements) != 4 {
		t.Fatalf("elements 长度 = %d, 期望 4（只覆盖有 ExtraRowElements 的行）", len(elements))
	}
}

func TestTableAppendProgress_BelowThreshold(t *testing.T) {
	// 扩展行 < threshold 时返回 nil
	fn := tableAppendProgress(3, 5, 5, func(int, int) {})
	if fn != nil {
		t.Error("扩展行 < threshold 时应返回 nil progress")
	}
}

func TestTableAppendProgress_NilLogger(t *testing.T) {
	fn := tableAppendProgress(10, 5, 5, nil)
	if fn != nil {
		t.Error("nil logger 应返回 nil progress")
	}
}

func TestTableAppendProgress_StepsAndFinal(t *testing.T) {
	var calls []struct{ a, b int }
	logger := func(a, b int) { calls = append(calls, struct{ a, b int }{a, b}) }
	fn := tableAppendProgress(12, 5, 5, logger)
	if fn == nil {
		t.Fatal("应返回非 nil progress")
	}
	// 模拟 12 次追加，step=5
	for i := 1; i <= 12; i++ {
		fn(i, 12)
	}
	// 期望触发于 5, 10, 12（最后一行）
	want := []struct{ a, b int }{{5, 12}, {10, 12}, {12, 12}}
	if len(calls) != len(want) {
		t.Fatalf("期望触发 %d 次，实际 %d：%+v", len(want), len(calls), calls)
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("calls[%d] = %+v, 期望 %+v", i, calls[i], w)
		}
	}
}

func TestTableAppendProgress_InvalidStep(t *testing.T) {
	fn := tableAppendProgress(10, 5, 0, func(int, int) {})
	if fn != nil {
		t.Error("step < 1 时应返回 nil progress")
	}
}

// 确保类型兼容 client.InsertRowProgressFunc
var _ client.InsertRowProgressFunc = tableAppendProgress(10, 5, 5, func(int, int) {})

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func TestCellTextBlockMapFromBlocks(t *testing.T) {
	cellType := int(converter.BlockTypeTableCell)
	textType := int(converter.BlockTypeText)
	blocks := []*larkdocx.Block{
		nil, // nil 块跳过
		{BlockId: strPtr("cell1"), BlockType: intPtr(cellType), Children: []string{"text1", "extra"}},
		{BlockId: strPtr("cell2"), BlockType: intPtr(cellType), Children: []string{"text2"}},
		{BlockId: strPtr("cell3"), BlockType: intPtr(cellType)},                         // 无子块跳过
		{BlockId: strPtr(""), BlockType: intPtr(cellType), Children: []string{"textX"}}, // 空 ID 跳过
		{BlockId: strPtr("txt1"), BlockType: intPtr(textType), Children: []string{"c"}}, // 非 cell 跳过
		{BlockId: strPtr("noType"), Children: []string{"c"}},                            // 无类型跳过
	}
	m := cellTextBlockMapFromBlocks(blocks)
	if len(m) != 2 {
		t.Fatalf("期望 2 项，得到 %d: %v", len(m), m)
	}
	if m["cell1"] != "text1" || m["cell2"] != "text2" {
		t.Fatalf("映射不符: %v", m)
	}
}

func TestMergeCellMaps(t *testing.T) {
	shared := map[string]string{"a": "1", "b": "2"}
	local := map[string]string{"b": "20", "c": "3"}
	merged := mergeCellMaps(shared, local)

	// 局部优先
	if merged["a"] != "1" || merged["b"] != "20" || merged["c"] != "3" || len(merged) != 3 {
		t.Fatalf("合并结果不符: %v", merged)
	}
	// 输入不被改写（import 多 worker 共享 shared，改写即 data race）
	if shared["b"] != "2" || len(shared) != 2 {
		t.Fatalf("shared 被改写: %v", shared)
	}
	if local["b"] != "20" || len(local) != 2 {
		t.Fatalf("local 被改写: %v", local)
	}

	// shared 为 nil（content-update 路径）
	m2 := mergeCellMaps(nil, local)
	if len(m2) != 2 || m2["c"] != "3" {
		t.Fatalf("nil shared 合并不符: %v", m2)
	}
}
