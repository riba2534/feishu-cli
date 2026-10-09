package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/converter"
)

// parseTableColumnWidthFlag 解析 --table-column-width flag，得到 ConvertOptions 的 ColumnWidthMode/Values。
//
// 接受三种形式：
//   - ""、"auto"：返回 mode="auto"（保留启发式）
//   - "fixed"：返回 mode="fixed"（按 defaultDocWidth/cols 等分）
//   - "N1,N2,...,Nk"：返回 mode="explicit"，每个值需为非负整数（像素），0 表示该列走 auto
//
// 解析失败返回带提示的 error，便于直接报到用户层。
func parseTableColumnWidthFlag(raw string) (mode string, values []int, err error) {
	s := strings.TrimSpace(raw)
	switch s {
	case "", "auto":
		return "auto", nil, nil
	case "fixed":
		return "fixed", nil, nil
	}

	parts := strings.Split(s, ",")
	values = make([]int, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" {
			values = append(values, 0)
			continue
		}
		v, e := strconv.Atoi(p)
		if e != nil {
			return "", nil, fmt.Errorf("--table-column-width 第 %d 项 %q 不是合法整数（用法：auto | fixed | N1,N2,...）", i+1, p)
		}
		if v < 0 {
			return "", nil, fmt.Errorf("--table-column-width 第 %d 项 %d 不能为负数", i+1, v)
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return "auto", nil, nil
	}
	return "explicit", values, nil
}

// applyColumnWidthOptions 把 flag 解析结果写入 ConvertOptions。
func applyColumnWidthOptions(opts *converter.ConvertOptions, mode string, values []int) {
	if opts == nil {
		return
	}
	opts.ColumnWidthMode = mode
	opts.ColumnWidthValues = values
}
