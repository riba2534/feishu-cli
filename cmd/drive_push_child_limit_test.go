package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestIsDriveFolderChildLimitErr 覆盖 drive push 的 1062507 终态判定分支：
// 命中该错误码（父目录直接子节点超 1500）时应中止整个 push，其它错误照常 continue。
func TestIsDriveFolderChildLimitErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"child-limit from upload", fmt.Errorf("上传文件失败: code=1062507, msg=folder children over limit"), true},
		{"child-limit from create folder", errors.New("创建文件夹失败: code=1062507, msg=xxx"), true},
		{"child-limit wrapped", fmt.Errorf("上传失败: %w", errors.New("code=1062507")), true},
		{"other rate limit", fmt.Errorf("上传文件失败: code=99991400, msg=too fast"), false},
		{"other not found", errors.New("code=1061002 not found"), false},
		{"unrelated text", errors.New("network timeout"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDriveFolderChildLimitErr(c.err); got != c.want {
				t.Fatalf("isDriveFolderChildLimitErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// TestDriveFolderChildLimitAdviceMentionsCode 确保中止建议里带上错误码与关键动作，
// 供用户按提示清理父目录后重跑。
func TestDriveFolderChildLimitAdviceMentionsCode(t *testing.T) {
	if driveFolderChildLimitAdvice == "" {
		t.Fatal("driveFolderChildLimitAdvice 不应为空")
	}
	for _, kw := range []string{"1062507", "1500", "重跑"} {
		if !strings.Contains(driveFolderChildLimitAdvice, kw) {
			t.Fatalf("清理建议应包含 %q，实际为 %q", kw, driveFolderChildLimitAdvice)
		}
	}
}
