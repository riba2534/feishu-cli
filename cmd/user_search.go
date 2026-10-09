package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var userSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "查询用户 ID（支持邮箱/手机号/关键词）",
	Long: `查询用户 ID。支持三种入口：
  --email / --mobile  走 contact/v3/users/batch_get_id（App Token），精确返回 open_id；
                      另按 user_id / union_id 各查一次填入对应字段（应用缺对应权限时留空）。
                      已登录时再用 contact/v3/users/basic_batch 按 open_id 精确补齐姓名。
  --query             走 contact/v3/users/search（User Token 必需），按姓名/邮箱/手机号等关键词搜索，
                      覆盖跨租户联系人，返回 open_id、姓名、邮箱、部门、单聊 chat_id 等。

输出字段（--email / --mobile）:
  open_id   本应用下的 Open ID（ou_xxx）
  user_id   租户内 User ID（需应用有 contact:user.employee_id:readonly，否则为空）
  union_id  跨应用的 Union ID（on_xxx）
  注意：旧版本的 user_id 字段实际填的是 open_id，已修正为真实 user_id。

参数:
  --email     邮箱列表，逗号分隔
  --mobile    手机号列表，逗号分隔
  --query     任意关键词（姓名/邮箱/手机号，≤50 字）

关键词搜索的过滤条件（可不带 --query 单独使用）:
  --user-ids               限定 open_id 范围（逗号分隔，≤100）
  --has-chatted            只看聊过天的人
  --exclude-external-users 排除外部（跨租户）联系人
  --left-organization      只看已离职的人
  --has-enterprise-email   只看有企业邮箱的人
  --lang                   姓名语言偏好（如 en_us）
  --page-size / --page-token  分页（每页 1-30）

关键词搜索 JSON 输出: {users:[{open_id,name,email,enterprise_email,department,p2p_chat_id,
  has_chatted,is_cross_tenant,is_activated,chat_recency_hint,match_segments}], has_more, page_token, notice}
  注意：旧版 search/v1/user 的 user_id / department_ids 字段新接口不再返回；需要 user_id 请用 --email。

示例:
  feishu-cli user search --email user@example.com
  feishu-cli user search --mobile +8613800138000
  feishu-cli user search --query "张三" -o json
  feishu-cli user search --query "张三" --has-chatted --exclude-external-users
  feishu-cli user search --email a@example.com,b@example.com -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		emailStr, _ := cmd.Flags().GetString("email")
		mobileStr, _ := cmd.Flags().GetString("mobile")
		query, _ := cmd.Flags().GetString("query")
		output, _ := cmd.Flags().GetString("output")

		if emailStr == "" && mobileStr == "" && query == "" && !hasUserSearchFilters(cmd) {
			return clierr.Usagef("至少需要指定 --email、--mobile、--query 或过滤条件（--has-chatted 等）之一")
		}
		if n := len([]rune(query)); n > client.MaxContactSearchQuery {
			return clierr.Usagef("--query 最多 %d 个字符，得到 %d 个", client.MaxContactSearchQuery, n)
		}

		userToken := resolveOptionalUserTokenWithFallback(cmd)

		// --query / 过滤条件路径：走 contact/v3/users/search（User 身份，对齐官方 +search-user）
		if query != "" || hasUserSearchFilters(cmd) {
			if emailStr != "" || mobileStr != "" {
				return clierr.Usagef("--email/--mobile 与 --query/过滤条件互斥：前者按邮箱手机号精确查 ID，后者是关键词搜索")
			}
			return runContactUserSearch(cmd, query, output)
		}

		// --email / --mobile 路径
		var emails, mobiles []string
		if emailStr != "" {
			emails = splitAndTrim(emailStr)
		}
		if mobileStr != "" {
			mobiles = splitAndTrim(mobileStr)
		}

		result, err := client.BatchGetUserID(emails, mobiles)
		if err != nil {
			return err
		}

		// 有 User Token 时按 open_id 精确补齐姓名（basic_batch）。
		// 旧版用 search/v1/user 模糊搜邮箱并取第一条，可能把同名/相近账号的姓名张冠李戴。
		if userToken != "" {
			var openIDs []string
			for _, info := range result {
				if info.OpenID != "" {
					openIDs = append(openIDs, info.OpenID)
				}
			}
			if len(openIDs) > 0 {
				names, nameErr := client.BatchGetUsersBasic(openIDs, userToken)
				if nameErr != nil && len(names) == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 补齐姓名失败（不影响 ID 结果）: %v\n", nameErr)
				}
				for _, info := range result {
					if n := names[info.OpenID]; n != "" {
						info.Name = n
					}
				}
			}
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result) == 0 {
			fmt.Println("未找到匹配的用户")
			return nil
		}

		fmt.Printf("查询结果（共 %d 条）:\n\n", len(result))
		for i, item := range result {
			fmt.Printf("[%d]", i+1)
			if item.Name != "" {
				fmt.Printf(" %s", item.Name)
			}
			fmt.Println()
			if item.UserID != "" {
				fmt.Printf("    user_id: %s\n", item.UserID)
			}
			if item.OpenID != "" {
				fmt.Printf("    open_id: %s\n", item.OpenID)
			}
			if item.UnionID != "" {
				fmt.Printf("    union_id: %s\n", item.UnionID)
			}
			if item.Email != "" {
				fmt.Printf("    邮箱: %s\n", item.Email)
			}
			if item.Mobile != "" {
				fmt.Printf("    手机号: %s\n", item.Mobile)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	userCmd.AddCommand(userSearchCmd)
	userSearchCmd.Flags().String("email", "", "邮箱列表，逗号分隔")
	userSearchCmd.Flags().String("mobile", "", "手机号列表，逗号分隔")
	userSearchCmd.Flags().String("query", "", "关键词搜索（姓名/邮箱/手机号，≤50 字），需 User Token")
	userSearchCmd.Flags().String("user-ids", "", "关键词搜索：限定 open_id 范围（逗号分隔，≤100）")
	userSearchCmd.Flags().Bool("has-chatted", false, "关键词搜索：只看聊过天的人")
	userSearchCmd.Flags().Bool("exclude-external-users", false, "关键词搜索：排除外部（跨租户）联系人")
	userSearchCmd.Flags().Bool("left-organization", false, "关键词搜索：只看已离职的人")
	userSearchCmd.Flags().Bool("has-enterprise-email", false, "关键词搜索：只看有企业邮箱的人")
	userSearchCmd.Flags().String("lang", "", "关键词搜索：姓名语言偏好（如 zh_cn、en_us）")
	userSearchCmd.Flags().Int("page-size", 20, "关键词搜索：每页数量（1-30）")
	userSearchCmd.Flags().String("page-token", "", "关键词搜索：分页标记")
	userSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	userSearchCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}

func hasUserSearchFilters(cmd *cobra.Command) bool {
	for _, name := range []string{"has-chatted", "exclude-external-users", "left-organization", "has-enterprise-email"} {
		if v, _ := cmd.Flags().GetBool(name); v {
			return true
		}
	}
	return strings.TrimSpace(flagString(cmd, "user-ids")) != ""
}

// runContactUserSearch 关键词 / 过滤条件搜索用户（contact/v3/users/search）。
func runContactUserSearch(cmd *cobra.Command, query, output string) error {
	userIDs := splitAndTrim(flagString(cmd, "user-ids"))
	if len(userIDs) > 100 {
		return clierr.Usagef("--user-ids 最多 100 个，得到 %d 个", len(userIDs))
	}
	pageSize := flagInt(cmd, "page-size")
	if pageSize < 1 || pageSize > client.MaxContactSearchPageSize {
		return clierr.Usagef("--page-size 范围 1-%d，得到 %d", client.MaxContactSearchPageSize, pageSize)
	}
	opts := client.SearchUsersV3Options{
		Query:     query,
		UserIDs:   userIDs,
		PageSize:  pageSize,
		PageToken: flagString(cmd, "page-token"),
		Lang:      flagString(cmd, "lang"),
	}
	opts.HasChatted, _ = cmd.Flags().GetBool("has-chatted")
	opts.ExcludeExternalUsers, _ = cmd.Flags().GetBool("exclude-external-users")
	opts.LeftOrganization, _ = cmd.Flags().GetBool("left-organization")
	opts.HasEnterpriseEmail, _ = cmd.Flags().GetBool("has-enterprise-email")

	userToken, err := requireUserToken(cmd, "user search --query")
	if err != nil {
		return err
	}
	res, err := client.SearchUsersV3(opts, userToken)
	if err != nil {
		return err
	}
	printSearchNotice(cmd.ErrOrStderr(), res.Notice)
	if output == "json" {
		return printJSON(res)
	}
	if len(res.Users) == 0 {
		fmt.Println("未找到匹配的用户")
		return nil
	}
	fmt.Printf("查询结果（共 %d 条）:\n\n", len(res.Users))
	for i, u := range res.Users {
		fmt.Printf("[%d] %s\n", i+1, u.Name)
		fmt.Printf("    open_id: %s\n", u.OpenID)
		if u.Department != "" {
			fmt.Printf("    部门: %s\n", u.Department)
		}
		if email := firstNonEmpty(u.EnterpriseEmail, u.Email); email != "" {
			fmt.Printf("    邮箱: %s\n", email)
		}
		if u.IsCrossTenant {
			fmt.Printf("    外部联系人: 是\n")
		}
		if u.P2PChatID != "" {
			fmt.Printf("    单聊 chat_id: %s\n", u.P2PChatID)
		}
		fmt.Println()
	}
	if res.HasMore {
		fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 还有更多结果：可加过滤条件（--has-chatted 等）缩小范围，或带 --page-token %s 翻页\n", res.PageToken)
	}
	return nil
}
