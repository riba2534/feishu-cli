package converter

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

func TestParseColWidthList_Basic(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"80,200,120", []int{80, 200, 120}},
		{"80, 200, 120", []int{80, 200, 120}},
		{"*,200,*", []int{0, 200, 0}},
		{"80,,120", []int{80, 0, 120}},        // 空位 → 0
		{"30%,50%,*", []int{210, 350, 0}},     // defaultDocWidth=700
		{"80,xxx,120", []int{80, 0, 120}},     // 非法 token → 0（容错）
		{"-50,200,300", []int{-50, 200, 300}}, // 负数透传，由 alignAndClampColumnWidths clamp 到 minColumnWidth
	}
	for _, tc := range cases {
		got := parseColWidthList(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseColWidthList(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestColWidthCommentRegex(t *testing.T) {
	pos := []string{
		"<!-- feishu-colwidth: 80, 200, 120 -->",
		"<!--feishu-colwidth:80,200,120-->",
		"<!-- feishu-colwidth: 80,200,* -->",
		"<!--   feishu-colwidth   :   80, 30%, *   -->",
		// 含尾随换行的 ast.HTMLBlock 文本
		"<!-- feishu-colwidth: 80,200 -->\n",
	}
	for _, s := range pos {
		if m := colWidthCommentRe.FindStringSubmatch(s); m == nil {
			t.Errorf("应该匹配但没匹配: %q", s)
		}
	}
	neg := []string{
		"<!-- some other comment -->",
		"<!-- COLWIDTH: 80,200 -->",                   // 大小写敏感
		"some text <!-- feishu-colwidth: 80 --> more", // 不独占行（regex 要求整段匹配）
		"<!-- feishu-colwidth -->",                    // 缺冒号和值
	}
	for _, s := range neg {
		if m := colWidthCommentRe.FindStringSubmatch(s); m != nil {
			t.Errorf("不应匹配但匹配了: %q", s)
		}
	}
}

func TestAlignAndClampColumnWidths_LengthMismatch(t *testing.T) {
	// 用户指定 2 列，实际有 4 列：缺位走 auto；超出截断
	headers := []string{"a", "b", "c", "d"}
	rows := [][]string{{"1", "2", "3", "4"}}

	got := alignAndClampColumnWidths([]int{120, 250}, headers, rows, 4)
	if len(got) != 4 {
		t.Fatalf("结果长度应等于 cols=4，实际 %d", len(got))
	}
	if got[0] != 120 {
		t.Errorf("第 1 列应使用显式值 120，实际 %d", got[0])
	}
	if got[1] != 250 {
		t.Errorf("第 2 列应使用显式值 250，实际 %d", got[1])
	}
	if got[2] < minColumnWidth || got[2] > maxColumnWidth {
		t.Errorf("第 3 列应走 auto + clamp，实际 %d", got[2])
	}
}

func TestAlignAndClampColumnWidths_StarPlaceholder(t *testing.T) {
	// 0 占位 → 该列走 auto
	headers := []string{"x", "y", "z"}
	rows := [][]string{{"1", "2", "3"}}
	got := alignAndClampColumnWidths([]int{200, 0, 150}, headers, rows, 3)
	if got[0] != 200 || got[2] != 150 {
		t.Errorf("非零位应保留显式值，实际 %v", got)
	}
	if got[1] < minColumnWidth || got[1] > maxColumnWidth {
		t.Errorf("0 占位应走 auto + clamp，实际 %d", got[1])
	}
}

func TestAlignAndClampColumnWidths_BelowMinClamped(t *testing.T) {
	headers := []string{"a", "b"}
	rows := [][]string{{"1", "2"}}
	got := alignAndClampColumnWidths([]int{20, 30}, headers, rows, 2)
	for i, w := range got {
		if w < minColumnWidth {
			t.Errorf("col %d: %d < min(%d)，应被 clamp", i, w, minColumnWidth)
		}
	}
}

func TestAlignAndClampColumnWidths_AboveMaxClamped(t *testing.T) {
	headers := []string{"a", "b"}
	rows := [][]string{{"1", "2"}}
	got := alignAndClampColumnWidths([]int{9999, 8000}, headers, rows, 2)
	for i, w := range got {
		if w > maxColumnWidth {
			t.Errorf("col %d: %d > max(%d)，应被 clamp", i, w, maxColumnWidth)
		}
	}
}

func TestExtractIntColumns(t *testing.T) {
	src := []int{80, 200, 0, 150, 100}
	got := extractIntColumns(src, []int{0, 2, 4})
	want := []int{80, 0, 100}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractIntColumns = %v, want %v", got, want)
	}
	// 索引越界 → 0
	got2 := extractIntColumns(src, []int{0, 10})
	if got2[1] != 0 {
		t.Errorf("越界应返回 0，实际 %v", got2)
	}
}

func TestResolveColumnWidths_Priority(t *testing.T) {
	headers := []string{"a", "b"}
	rows := [][]string{{"1", "2"}}

	// 优先级 1：注释（pendingColWidth）盖过 explicit
	c := &MarkdownToBlock{
		options:         ConvertOptions{ColumnWidthMode: "explicit", ColumnWidthValues: []int{300, 300}},
		pendingColWidth: []int{120, 250},
	}
	got := c.resolveColumnWidths(headers, rows, 2)
	if got[0] != 120 || got[1] != 250 {
		t.Errorf("注释应胜过 explicit，实际 %v", got)
	}
	if c.pendingColWidth != nil {
		t.Error("注释消费后 pendingColWidth 必须清空")
	}

	// 优先级 2：explicit
	c2 := &MarkdownToBlock{
		options: ConvertOptions{ColumnWidthMode: "explicit", ColumnWidthValues: []int{120, 250}},
	}
	got2 := c2.resolveColumnWidths(headers, rows, 2)
	if got2[0] != 120 || got2[1] != 250 {
		t.Errorf("explicit 应生效，实际 %v", got2)
	}

	// 优先级 3：fixed → defaultDocWidth/cols 等分
	c3 := &MarkdownToBlock{
		options: ConvertOptions{ColumnWidthMode: "fixed"},
	}
	got3 := c3.resolveColumnWidths(headers, rows, 2)
	want3 := defaultDocWidth / 2
	if got3[0] != want3 || got3[1] != want3 {
		t.Errorf("fixed 应等分 %d，实际 %v", want3, got3)
	}

	// 优先级 4：auto（兼容旧行为）
	c4 := &MarkdownToBlock{}
	got4 := c4.resolveColumnWidths(headers, rows, 2)
	want4 := calculateColumnWidths(headers, rows, 2)
	if !reflect.DeepEqual(got4, want4) {
		t.Errorf("auto 应等价于 calculateColumnWidths，实际 %v vs %v", got4, want4)
	}
}

func TestColumnWidth_HTMLBlockComment_EndToEnd(t *testing.T) {
	md := []byte("<!-- feishu-colwidth: 100,250,150 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n")
	conv := NewMarkdownToBlock(md, ConvertOptions{}, "")
	res, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(res.TableDatas) != 1 {
		t.Fatalf("应有 1 张表，实际 %d", len(res.TableDatas))
	}
	// 找到 Table block 并校验 ColumnWidth
	var table *larkdocx.Block
	for _, n := range res.BlockNodes {
		if n.Block != nil && n.Block.Table != nil {
			table = n.Block
			break
		}
	}
	if table == nil {
		t.Fatal("未在 BlockNodes 中找到 table block")
	}
	cw := table.Table.Property.ColumnWidth
	if len(cw) != 3 {
		t.Fatalf("ColumnWidth 长度应为 3，实际 %d (%v)", len(cw), cw)
	}
	if cw[0] != 100 || cw[1] != 250 || cw[2] != 150 {
		t.Errorf("注释列宽未正确写入: %v", cw)
	}
}

// TestColumnWidth_ExplicitFlag_ColumnSplit 验证 BUG#9：>9 列表用 --table-column-width(explicit)
// 时，列宽必须按列组切片——第二组是 col0(标识列)+col9+col10，应取对应索引的列宽 [v0,v9,v10]，
// 而不是把完整数组的前 3 个 [v0,v1,v2] 套上去（修复前的错位行为）。与注释路径(splitPendingColWidth)对称。
func TestColumnWidth_ExplicitFlag_ColumnSplit(t *testing.T) {
	// 11 列（超 9 触发列拆分），1 数据行
	header := "| c0 | c1 | c2 | c3 | c4 | c5 | c6 | c7 | c8 | c9 | c10 |"
	sep := "|---|---|---|---|---|---|---|---|---|---|---|"
	row := "| 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |"
	md := []byte(header + "\n" + sep + "\n" + row + "\n")
	values := []int{110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210}
	conv := NewMarkdownToBlock(md, ConvertOptions{ColumnWidthMode: "explicit", ColumnWidthValues: values}, "")
	res, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	var tables []*larkdocx.Block
	for _, n := range res.BlockNodes {
		if n.Block != nil && n.Block.Table != nil {
			tables = append(tables, n.Block)
		}
	}
	if len(tables) != 2 {
		t.Fatalf("11 列应拆成 2 个列组表，实际 %d", len(tables))
	}
	// group1: col0..col8 → 列宽 110..190
	cw1 := tables[0].Table.Property.ColumnWidth
	want1 := []int{110, 120, 130, 140, 150, 160, 170, 180, 190}
	if !reflect.DeepEqual(cw1, want1) {
		t.Errorf("group1 列宽错误: got %v want %v", cw1, want1)
	}
	// group2: col0(标识列)+col9+col10 → 列宽 [110,200,210]，绝不能是前 3 个 [110,120,130]
	cw2 := tables[1].Table.Property.ColumnWidth
	want2 := []int{110, 200, 210}
	if !reflect.DeepEqual(cw2, want2) {
		t.Errorf("group2 列宽应按列组切片(col0/col9/col10)=%v，得到 %v（修复前的 bug 是错误的前 3 个值）", want2, cw2)
	}

	// 顺带验证：explicit flag 是全局的，不应被本次列拆分消费掉——同一 conv 再转一张表仍生效。
	if len(conv.options.ColumnWidthValues) != 11 {
		t.Errorf("列拆分不应破坏全局 options.ColumnWidthValues，剩余 %d 个", len(conv.options.ColumnWidthValues))
	}
}

// TestColumnWidth_PendingDoesNotLeakAcrossBlocks 验证 review-fix-1：
// 注释和表格之间夹了非空块（heading/paragraph）时，pendingColWidth 必须被清空，
// 不能让"悬浮"注释污染下游表格列宽。
func TestColumnWidth_PendingDoesNotLeakAcrossBlocks(t *testing.T) {
	cases := []struct {
		name string
		md   string
	}{
		{
			name: "heading 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n# 一级标题\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			name: "paragraph 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n这是一段普通段落。\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			name: "list 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n- item1\n- item2\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			name: "code block 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n```go\nfmt.Println(\"hi\")\n```\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			name: "thematic break 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n---\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			// review-fix-1b 增补：缩进 4 空格代码块（*ast.CodeBlock，与 FencedCodeBlock 是两种 AST 类型）
			name: "indented code block 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n    indented code\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			// review-fix-1b 增补：link reference definition（*ast.TextBlock 包裹）
			name: "link reference definition 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n[foo]: http://example.com\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			// review-fix-1b 增补：blockquote 间隔
			name: "blockquote 间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n> 引用\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
		{
			// review-fix-1b 增补：另一个不命中 colwidth 注释的 HTMLBlock 间隔
			name: "其他 HTML 注释间隔",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n<!-- 一段无关注释 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := NewMarkdownToBlock([]byte(tc.md), ConvertOptions{}, "")
			res, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			var table *larkdocx.Block
			for _, n := range res.BlockNodes {
				if n.Block != nil && n.Block.Table != nil {
					table = n.Block
					break
				}
			}
			if table == nil {
				t.Fatal("未找到 table block")
			}
			cw := table.Table.Property.ColumnWidth
			// 期望走 auto，宽度由内容决定 — 不应等于 80,200,300
			if len(cw) == 3 && cw[0] == 80 && cw[1] == 200 && cw[2] == 300 {
				t.Errorf("悬浮注释泄漏到下游表格：%v；期望走 auto", cw)
			}
		})
	}
}

// TestColumnWidth_PendingConsumedByImmediateTable 验证语义：紧邻 Table 时仍正确消费。
func TestColumnWidth_PendingConsumedByImmediateTable(t *testing.T) {
	// 紧邻（中间只有空行）
	md := "<!-- feishu-colwidth: 100,200,300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	res, _ := conv.ConvertWithTableData()
	var table *larkdocx.Block
	for _, n := range res.BlockNodes {
		if n.Block != nil && n.Block.Table != nil {
			table = n.Block
			break
		}
	}
	cw := table.Table.Property.ColumnWidth
	if cw[0] != 100 || cw[1] != 200 || cw[2] != 300 {
		t.Errorf("紧邻消费失败: %v", cw)
	}
}

// TestColumnWidth_PendingClearedAfterTable 验证：注释作用一次后必须清空，
// 第二张表（即使紧邻第一张）不应继承注释。
func TestColumnWidth_PendingClearedAfterTable(t *testing.T) {
	md := "<!-- feishu-colwidth: 100,200,300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n\n| x | y | z |\n|---|---|---|\n| 4 | 5 | 6 |\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	res, _ := conv.ConvertWithTableData()
	var tables []*larkdocx.Block
	for _, n := range res.BlockNodes {
		if n.Block != nil && n.Block.Table != nil {
			tables = append(tables, n.Block)
		}
	}
	if len(tables) != 2 {
		t.Fatalf("应有 2 张表，实际 %d", len(tables))
	}
	// 第一张应消费注释
	cw1 := tables[0].Table.Property.ColumnWidth
	if cw1[0] != 100 {
		t.Errorf("第一张表应消费注释，实际 %v", cw1)
	}
	// 第二张应走 auto，不能等于 100,200,300
	cw2 := tables[1].Table.Property.ColumnWidth
	if len(cw2) == 3 && cw2[0] == 100 && cw2[1] == 200 && cw2[2] == 300 {
		t.Errorf("第二张表泄漏了首张的注释: %v", cw2)
	}
}

// TestColumnWidth_MultilineHTMLComment 验证 review-fix-2：
// goldmark.HTMLBlock.Lines() 在多行注释下只返回首行，但 getHTMLBlockText
// 必须能扫到 `-->` 闭合标签，把整段注释完整还原。
func TestColumnWidth_MultilineHTMLComment(t *testing.T) {
	cases := []struct {
		name     string
		md       string
		want     []int
		extraCol int // 当 extraMin > 0 时，校验该列至少 extraMin（auto 路径），其余列对齐 want
		extraMin int
	}{
		{
			name: "单行紧凑注释",
			md:   "<!-- feishu-colwidth: 80,200,300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
			want: []int{80, 200, 300},
		},
		{
			name: "闭合 --> 在第二行",
			md:   "<!-- feishu-colwidth: 80,200,300\n-->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
			want: []int{80, 200, 300},
		},
		{
			name: "多列写在多行，闭合在第三行",
			md:   "<!-- feishu-colwidth: 80,\n200,\n300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
			want: []int{80, 200, 300},
		},
		{
			// review-fix-3 增补：用户笔误写负数，旧 [^-]+ 正则会让整段注释匹配失败导致静默丢
			// 新 [^>]*? 正则放行；parseColWidthList 透传负值 → alignAndClampColumnWidths
			// 把 ≤0 视作"未指定"走 auto。第 1 列宽度由 calculateColumnWidths 决定，>=80 即可
			name:     "negative value 走 auto（不再静默丢失整段注释）",
			md:       "<!-- feishu-colwidth: -50,200,300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
			want:     []int{200, 300}, // 仅校验 col 1 / 2 是显式值；col 0 单独断言
			extraCol: 0,
			extraMin: minColumnWidth,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := NewMarkdownToBlock([]byte(tc.md), ConvertOptions{}, "")
			res, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			var table *larkdocx.Block
			for _, n := range res.BlockNodes {
				if n.Block != nil && n.Block.Table != nil {
					table = n.Block
					break
				}
			}
			if table == nil {
				t.Fatal("未找到 table block")
			}
			cw := table.Table.Property.ColumnWidth
			// extraMin > 0 时校验某一列至少不小于该阈值（auto 计算路径）
			if tc.extraMin > 0 {
				if cw[tc.extraCol] < tc.extraMin {
					t.Errorf("列 %d 走 auto 应 >= %d，实际 %d", tc.extraCol, tc.extraMin, cw[tc.extraCol])
				}
				// 校验其余列等于 want 中的显式值
				explicitColIdx := 0
				for i := 0; i < len(cw); i++ {
					if i == tc.extraCol {
						continue
					}
					if explicitColIdx >= len(tc.want) {
						break
					}
					if cw[i] != tc.want[explicitColIdx] {
						t.Errorf("列 %d: 实际 %d, 期望 %d (full=%v)", i, cw[i], tc.want[explicitColIdx], cw)
					}
					explicitColIdx++
				}
				return
			}

			if len(cw) != len(tc.want) {
				t.Fatalf("列宽长度 %d != 期望 %d (%v)", len(cw), len(tc.want), cw)
			}
			for i, w := range tc.want {
				if cw[i] != w {
					t.Errorf("列 %d: 实际 %d, 期望 %d (full=%v)", i, cw[i], w, cw)
				}
			}
		})
	}
}

// TestColumnWidth_LengthMismatchWarn 验证 review-fix-3a：
// alignAndClampColumnWidths 对长度与列数不一致时不再静默处理，
// 默认向 stderr 打印一行警告（用户显式输入列宽应被告知是否生效）。
func TestColumnWidth_LengthMismatchWarn(t *testing.T) {
	cases := []struct {
		name       string
		md         string
		expectWarn bool
		expectIn   string // 警告中应包含的关键词
	}{
		{
			name:       "多写应警告",
			md:         "<!-- feishu-colwidth: 80,200,300 -->\n\n| a | b |\n|---|---|\n| 1 | 2 |\n",
			expectWarn: true,
			expectIn:   "尾部",
		},
		{
			name:       "少写应警告",
			md:         "<!-- feishu-colwidth: 80,200 -->\n\n| a | b | c | d |\n|---|---|---|---|\n| 1 | 2 | 3 | 4 |\n",
			expectWarn: true,
			expectIn:   "缺失",
		},
		{
			name:       "长度匹配不应警告",
			md:         "<!-- feishu-colwidth: 80,200,300 -->\n\n| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n",
			expectWarn: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 把 stderr 重定向到 pipe，捕获输出
			origStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w
			t.Cleanup(func() { os.Stderr = origStderr })

			conv := NewMarkdownToBlock([]byte(tc.md), ConvertOptions{}, "")
			_, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			_ = w.Close()
			out, _ := io.ReadAll(r)
			_ = r.Close()

			gotWarn := strings.Contains(string(out), "[警告]")
			if gotWarn != tc.expectWarn {
				t.Fatalf("expectWarn=%v, got=%v, stderr=%q", tc.expectWarn, gotWarn, string(out))
			}
			if tc.expectWarn && tc.expectIn != "" && !strings.Contains(string(out), tc.expectIn) {
				t.Errorf("警告应包含关键词 %q，实际 %q", tc.expectIn, string(out))
			}
		})
	}
}
