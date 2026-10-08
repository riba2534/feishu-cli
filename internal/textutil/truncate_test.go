package textutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"短于上限原样返回", "abc", 10, "abc"},
		{"ASCII 按字节截断", "abcdef", 3, "abc"},
		{"中文不切半个字符", "飞书文档", 4, "飞"}, // 每个汉字 3 字节，4 字节只能容下 1 个
		{"恰好在字符边界", "飞书文档", 6, "飞书"},
		{"emoji 不切半", "a😀b", 3, "a"},
		{"上限为 0", "飞书", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateUTF8(tc.in, tc.max)
			if got != tc.want {
				t.Fatalf("TruncateUTF8(%q, %d) = %q，期望 %q", tc.in, tc.max, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("结果不是合法 UTF-8: %q", got)
			}
			if len(got) > tc.max && tc.max >= 0 {
				t.Fatalf("结果超过字节上限: %d > %d", len(got), tc.max)
			}
		})
	}
}
