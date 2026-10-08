package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestBodyPreviewKeepsValidUTF8 验证错误响应体预览按字符边界截断：
// 512 字节上限落在汉字中间时不能产生半个字符（终端乱码）。
func TestBodyPreviewKeepsValidUTF8(t *testing.T) {
	body := "x" + strings.Repeat("飞", 300) // 1 + 900 字节，512 落在第 171 个汉字中间
	got := bodyPreview([]byte(body))
	if !utf8.ValidString(got) {
		t.Fatalf("预览不是合法 UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...(已截断)") {
		t.Fatalf("超长预览应带截断标记: %q", got)
	}
}
