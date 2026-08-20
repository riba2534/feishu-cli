package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestDisplayWidth(t *testing.T) {
	// 期望值来自终端实际渲染列数，独立于实现，不复用被测函数计算
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"alert", 5},
		{"--profile alert", 15},
		{"测试用户", 8},
		{"a测b", 4},
		{"（全角）", 8},
		{"张三🎉", 6},
		{"👨\u200d💻", 2},  // ZWJ 表情整簇占 2 列，不是 2+1+2
		{"é", 1},         // 组合音标：e + U+0301
		{"❤\ufe0f", 2},   // variation selector 让心形变成全宽表情
		{"a\u0301bc", 3}, // 组合音标不额外占列
	}
	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// tabwriter 按字节对齐，中文用户名会把后面的列顶歪；renderColumns 必须按显示宽度对齐。
func TestRenderColumnsAlignsCJK(t *testing.T) {
	var buf bytes.Buffer
	header := []string{"NAME", "USER", "SELECT"}
	rows := [][]string{
		{"alert", "测试用户", "--profile alert"},
		{"notify", "-", "--profile notify"},
		{"a", "Ada", "--profile a"},
		{"zwj", "👨\u200d💻", "--profile zwj"},
	}
	if err := renderColumns(&buf, header, rows); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("行数 = %d, want %d", len(lines), len(rows)+1)
	}

	// 列宽由最宽单元格决定：NAME=6("notify")、USER=8("测试用户")，列间 2 空格,
	// 因此末列必须从第 6+2+8+2=18 列开始。硬编码此值，不用 displayWidth 自证。
	const wantCol = 18
	for i, line := range lines {
		marker := "SELECT"
		if i > 0 {
			marker = "--profile"
		}
		idx := strings.Index(line, marker)
		if idx < 0 {
			t.Fatalf("第 %d 行缺少末列: %q", i, line)
		}
		if col := uniseg.StringWidth(line[:idx]); col != wantCol {
			t.Errorf("第 %d 行末列起始于第 %d 显示列，want %d\n%s", i, col, wantCol, buf.String())
		}
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("末列不应补尾随空格: %q", line)
		}
	}
}
