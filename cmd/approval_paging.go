package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// approvalPageAllMax --page-all 的翻页上限（审批列表是稀疏分页，空页不代表结束）
const approvalPageAllMax = 100

// approvalPageResult 翻页汇总
type approvalPageResult[T any] struct {
	Items     []T
	PageToken string // 仍有更多时用于续翻
	HasMore   bool
	Pages     int
}

// approvalCollectPages 按 page_token 连续翻页，直到 has_more=false、达到 limit 页或游标重复。
//
// 审批 tasks / instances/initiated 是**稀疏分页**：实测同一查询 page_size 3/10/50/100 首页分别返回
// 0/4/18/35 条且 has_more 都为 true，后续页仍有大量数据——空页或不足 page_size 都不代表结束，
// 只能以 has_more 为准。
func approvalCollectPages[T any](startToken string, limit int, fetch func(token string) ([]T, string, bool, error)) (*approvalPageResult[T], error) {
	if limit <= 0 || limit > approvalPageAllMax {
		limit = approvalPageAllMax
	}
	res := &approvalPageResult[T]{}
	token := startToken
	seen := map[string]bool{}
	for res.Pages < limit {
		items, next, more, err := fetch(token)
		if err != nil {
			return res, err
		}
		res.Pages++
		res.Items = append(res.Items, items...)
		res.PageToken, res.HasMore = next, more
		if !more || next == "" {
			res.HasMore = more && next != ""
			return res, nil
		}
		if seen[next] || next == token {
			// 游标不前进：防止死循环，按"还有更多但无法继续"返回
			fmt.Fprintf(cmdErrOut(), "警告：服务端返回了重复的 page_token，已停止翻页（已取 %d 页）\n", res.Pages)
			return res, nil
		}
		seen[next] = true
		token = next
	}
	return res, nil
}

// validateApprovalPaging 校验 --page-all 与 raw-json/page-limit 组合
func validateApprovalPaging(pageAll bool, pageLimit int, output string) error {
	if pageAll && output == "raw-json" {
		return clierr.Usagef("--page-all 不支持 --output raw-json（raw-json 只输出单页原始响应）")
	}
	if pageLimit < 0 || pageLimit > approvalPageAllMax {
		return clierr.Usagef("--page-limit 范围 1-%d", approvalPageAllMax)
	}
	return nil
}

// printApprovalSparseHint 在 stderr 提示稀疏分页：本页少/空不代表没有数据
func printApprovalSparseHint(n int, hasMore bool, pageToken string, pageAll bool) {
	if !hasMore {
		return
	}
	if pageAll {
		fmt.Fprintf(cmdErrOut(), "提示：已达翻页上限，仍有更多数据；用 --page-token %s 继续，或调大 --page-limit\n", pageToken)
		return
	}
	fmt.Fprintf(cmdErrOut(), "提示：本页 %d 条，服务端标记还有更多（审批列表为稀疏分页，空页或不足 page_size 不代表结束）；用 --page-token %s 继续，或加 --page-all 自动翻页\n", n, pageToken)
}
