package cmd

import (
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 评论 ID 含路径分隔符等字符时是用法错误，不得进入确认门禁或拼进请求路径。
func TestParseTaskCommentIDArg_RejectsPathChars(t *testing.T) {
	for _, bad := range []string{"bad/id", "a/../b", "x?y", "x#y", "a b"} {
		if _, err := parseTaskCommentIDArg(bad); exitCodeFor(err) != clierr.ExitUsage {
			t.Fatalf("%q 应为用法错误，got %v", bad, err)
		}
	}
	if id, err := parseTaskCommentIDArg(" 7234567890123456789 "); err != nil || id != "7234567890123456789" {
		t.Fatalf("合法 ID 应通过: %q %v", id, err)
	}
}
