// Package textutil 提供与编码相关的文本小工具。
package textutil

import "unicode/utf8"

// TruncateUTF8 返回 s 不超过 maxBytes 字节、且结束于完整字符边界的最长前缀。
//
// 用于按字节预算截断（错误预览、文件名长度上限等）：直接 s[:n] 会把多字节字符
// （中文、emoji）切成半个，产生非法 UTF-8，终端显示乱码，写进文件名还可能被文件系统拒绝。
func TruncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	// 回退到字符起始字节；最多回退 utf8.UTFMax-1 次。
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
