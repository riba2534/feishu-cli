package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/rivo/uniseg"
)

// displayWidth 返回字符串在等宽终端里占的列数。
//
// 用 uniseg 按 grapheme cluster 计算：ZWJ 表情（👨‍💻）、variation selector、组合音标
// 都必须整簇算一列宽度，按 rune 累加会把 👨‍💻 算成 2+1+2=5 而实际只占 2 列。
// uniseg 已在依赖图中（go-runewidth 的依赖），不引入新的体积。
func displayWidth(s string) int {
	return uniseg.StringWidth(s)
}

// padCell 把单元格补到 width 显示列；超宽时原样返回，由后续列自行错开。
func padCell(s string, width int) string {
	if pad := width - displayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// renderColumns 按显示宽度对齐输出表格：列间固定 2 空格，末列不补尾随空格。
func renderColumns(w io.Writer, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && displayWidth(cell) > widths[i] {
				widths[i] = displayWidth(cell)
			}
		}
	}
	for _, row := range append([][]string{header}, rows...) {
		var b strings.Builder
		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)
				break
			}
			b.WriteString(padCell(cell, widths[i]))
			b.WriteString("  ")
		}
		if _, err := fmt.Fprintln(w, b.String()); err != nil {
			return err
		}
	}
	return nil
}
