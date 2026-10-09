package client

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// ResolvePageSize 把 CLI 的 page-size 归一到 API 合法范围。
// n==0 表示未指定，使用 def；越界必须报错，禁止静默截断。
func ResolvePageSize(n, def, min, max int) (int, error) {
	if n == 0 {
		return def, nil
	}
	if n < min || n > max {
		return 0, fmt.Errorf("每页数量必须在 %d–%d 之间，得到 %d", min, max, n)
	}
	return n, nil
}

// PaginationCursor 合并 page_token / next_page_token，并在 --page-all 时检测无进展游标。
// prev 为上一页实际使用的 cursor（首页为请求里带的 page_token，可为空）。
// SearchPageLimitMax 是 search messages / chat search --page-all 的官方页数上限。
const SearchPageLimitMax = 40

// ResolvePageLimit 归一 --page-limit。负数和超过 max 报用法错误（退出码 2）；
// --page-all 且 n==0 时采用官方上限 max（不是无限翻页）。
func ResolvePageLimit(n, max int, pageAll bool) (int, error) {
	if n < 0 {
		return 0, clierr.Usagef("--page-limit 不能为负数，得到 %d", n)
	}
	if max <= 0 {
		max = SearchPageLimitMax
	}
	if n > max {
		return 0, clierr.Usagef("--page-limit 最大为 %d，得到 %d", max, n)
	}
	if pageAll && n == 0 {
		return max, nil
	}
	return n, nil
}

func PaginationCursor(hasMore bool, pageToken, nextPageToken, prev string) (more bool, token string, err error) {
	token = pageToken
	if token == "" {
		token = nextPageToken
	}
	if !hasMore {
		return false, token, nil
	}
	if token == "" {
		return false, "", fmt.Errorf("分页失败: 服务端 has_more=true 但未返回 page_token/next_page_token，已停止以免重复拉取")
	}
	if prev != "" && token == prev {
		return false, "", fmt.Errorf("分页失败: page_token %q 未前进，已停止以免死循环", token)
	}
	return true, token, nil
}
