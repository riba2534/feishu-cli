package client

import (
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// TestResolvePageLimitOutOfRangeIsUsageError --page-limit 越界是用法错误（退出码 2），不是一般错误。
func TestResolvePageLimitOutOfRangeIsUsageError(t *testing.T) {
	for _, n := range []int{-1, SearchPageLimitMax + 1} {
		if _, err := ResolvePageLimit(n, SearchPageLimitMax, true); !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("ResolvePageLimit(%d) 应返回用法错误，得到 %v", n, err)
		}
	}
	if got, err := ResolvePageLimit(5, SearchPageLimitMax, false); err != nil || got != 5 {
		t.Errorf("合法值不应报错，得到 (%d, %v)", got, err)
	}
}
