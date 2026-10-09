package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// defaultListPageLimit 是云盘/知识库/评论列表 --page-all 的默认翻页上限（页数）。
// 服务端异常（has_more 一直为 true）时防止无限翻页；用户可用 --page-limit 调整，0 表示不限。
const defaultListPageLimit = 50

// listPageOptions 描述一次列表请求的分页参数（来自 --page-token / --page-all / --page-limit）。
type listPageOptions struct {
	PageToken string
	PageAll   bool
	PageLimit int // --page-all 时最多翻多少页，0 = 不限
}

// listPageResult 是 collectListPages 的结果：HasMore=true 时 NextPageToken 为续翻游标。
type listPageResult[T any] struct {
	Items         []T
	HasMore       bool
	NextPageToken string
	Pages         int
	// Truncated 表示 --page-all 因达到 --page-limit 而提前停止（服务端仍有更多数据）。
	Truncated bool
}

// addListPageFlags 为列表命令注册统一的分页 flag。
func addListPageFlags(cmd *cobra.Command) {
	cmd.Flags().String("page-token", "", "分页标记（续翻上一次输出提示中的 page_token）")
	cmd.Flags().Bool("page-all", false, "自动翻页拉取全部结果（带重复游标防护与 --page-limit 上限）")
	cmd.Flags().Int("page-limit", defaultListPageLimit, "--page-all 时最多翻多少页（0 = 不限）")
}

// readListPageOptions 读取并校验分页 flag。
func readListPageOptions(cmd *cobra.Command) (listPageOptions, error) {
	pageToken, _ := cmd.Flags().GetString("page-token")
	pageAll, _ := cmd.Flags().GetBool("page-all")
	pageLimit, _ := cmd.Flags().GetInt("page-limit")
	if pageLimit < 0 {
		return listPageOptions{}, clierr.Usagef("--page-limit 不能为负数，得到 %d", pageLimit)
	}
	return listPageOptions{PageToken: strings.TrimSpace(pageToken), PageAll: pageAll, PageLimit: pageLimit}, nil
}

// collectListPages 按分页参数拉取列表：
//   - 未开 --page-all：只取一页（起点为 --page-token），把 has_more/page_token 原样带回；
//   - --page-all：从 --page-token（为空则首页）开始连续翻页，遇到"has_more 但无游标"或"游标不前进"
//     立即报错停止（client.PaginationCursor），达到 --page-limit 时停止并标记 Truncated。
func collectListPages[T any](opts listPageOptions, fetch func(pageToken string) ([]T, string, bool, error)) (*listPageResult[T], error) {
	res := &listPageResult[T]{}
	token := opts.PageToken
	for {
		items, next, hasMore, err := fetch(token)
		if err != nil {
			return nil, err
		}
		res.Pages++
		res.Items = append(res.Items, items...)
		more, cursor, err := client.PaginationCursor(hasMore, "", next, token)
		if err != nil {
			return nil, err
		}
		res.HasMore = more
		if more {
			res.NextPageToken = cursor
		} else {
			res.NextPageToken = ""
		}
		if !opts.PageAll || !more {
			return res, nil
		}
		if opts.PageLimit > 0 && res.Pages >= opts.PageLimit {
			res.Truncated = true
			return res, nil
		}
		token = cursor
	}
}

// printListPageHint 在 has_more=true 时向 stderr 输出续翻提示（stdout 保持纯数据，-o json 管道安全）。
func printListPageHint(w io.Writer, res interface {
	pageHint() (bool, string, bool)
}) {
	hasMore, next, truncated := res.pageHint()
	if !hasMore {
		return
	}
	// page_token 可能含 "||" 等 shell 元字符（实测 wiki spaces 返回 "1791006493||0"），续翻命令必须加引号
	quoted := quotePOSIXShell(next)
	if truncated {
		fmt.Fprintf(w, "⚠️  已达到 --page-limit 上限，结果不完整（has_more=true, page_token=%s）。可加大 --page-limit，或用 --page-token %s 继续翻页\n", next, quoted)
		return
	}
	fmt.Fprintf(w, "提示: 还有更多结果（has_more=true, page_token=%s）。续翻: --page-token %s；或加 --page-all 拉取全部\n", next, quoted)
}

func (r *listPageResult[T]) pageHint() (bool, string, bool) {
	if r == nil {
		return false, "", false
	}
	return r.HasMore, r.NextPageToken, r.Truncated
}

// resolveIdentityWithLegacyDefault 为"新增 --as 但默认保持旧身份行为"的命令解析身份：
//   - 显式传了 --as（bot|user|auto）→ 走 resolveIdentityToken（auto 已配置 User 但不可用时 fail-closed）；
//   - 未传 --as → 调用 legacy，保持命令原有的默认身份语义（避免破坏既有脚本）。
//
// 返回空字符串表示使用 App/Tenant Token。
func resolveIdentityWithLegacyDefault(cmd *cobra.Command, legacy func(*cobra.Command) (string, error)) (string, error) {
	if f := cmd.Flags().Lookup("as"); f != nil && f.Changed {
		return resolveIdentityToken(cmd)
	}
	return legacy(cmd)
}

// legacyBotUnlessExplicitUserToken：旧默认 = Bot，仅在显式 --user-access-token / FEISHU_USER_ACCESS_TOKEN 时切 User。
func legacyBotUnlessExplicitUserToken(cmd *cobra.Command) (string, error) {
	return resolveOptionalUserToken(cmd), nil
}

// legacyUserWithBotFallback：旧默认 = User 优先，User 不可用时 stderr 告警后回退 Bot（读类命令）。
func legacyUserWithBotFallback(cmd *cobra.Command) (string, error) {
	return resolveOptionalUserTokenWithFallback(cmd), nil
}

// addLegacyAsFlag 注册 --as；help 中说明"不传时保持旧默认身份"。
func addLegacyAsFlag(cmd *cobra.Command, legacyDesc string) {
	cmd.Flags().String("as", "", "身份: bot(App Token) | user(User Token) | auto(User 优先；未配置回退 Bot；已配置但不可用 fail-closed)。不传时"+legacyDesc)
}
