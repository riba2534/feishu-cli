package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// searchDocsMaxPageSize v2 doc_wiki/search 单页上限
const searchDocsMaxPageSize = 20

// searchDocsTypes search docs --docs-types 接受的取值（大小写不敏感，发送时转为 v2 大写枚举）
var searchDocsTypes = []string{"doc", "docx", "sheet", "slides", "bitable", "mindnote", "file", "wiki", "shortcut", "folder", "catalog"}

var searchDocsCmd = &cobra.Command{
	Use:   "docs <query>",
	Short: "搜索云文档",
	Long: `搜索当前用户可见的飞书云文档与知识库（POST /open-apis/search/v2/doc_wiki/search，
与官方 docs +search、本 CLI 的 drive search 使用同一 v2 端点）。

注意：此功能需要 User Access Token（用户授权令牌），推荐通过 auth login 获取；scope: search:docs:read。

参数:
  query           搜索关键词（必需）

选项:
  --count         每页数量（1-20，默认 20；v2 单页上限 20，超出按 20 处理并在 stderr 提示）
  --page-token    分页标记（上一页输出的 page_token）
  --owner-ids     文件所有者 Open ID 列表（逗号分隔；映射 v2 filter.creator_ids，服务端按 owner 语义匹配）
  --chat-ids      文件所在群 ID 列表（逗号分隔；映射 filter.chat_ids）
  --docs-types    文档类型列表（逗号分隔，映射 filter.doc_types）：
                  doc/docx/sheet/slides/bitable/mindnote/file/wiki/shortcut/folder/catalog
  --offset        已废弃：v2 端点只能按 page_token 翻页；传 >0 的值报用法错误，请改用 --page-token

与旧端点（/open-apis/suite/docs-api/search/object）相比的变化:
  - 每页最多 20 条（旧端点 50），翻页改用 --page-token
  - 默认同时搜索云盘与知识库；更多过滤维度（文件夹、知识空间、排序等）见 drive search
  - JSON 输出保留 Total / HasMore / ResUnits（DocsToken/DocsType/Title/OwnerID/URL），新增 PageToken；
    DocsType 为小写类型名，Title 已去除 <h> 高亮标记

示例:
  # 先登录获取 Token（推荐）
  feishu-cli auth login

  # 搜索包含"产品需求"的文档
  feishu-cli search docs "产品需求"

  # 搜索特定类型的文档
  feishu-cli search docs "季度报告" --docs-types docx,sheet

  # 指定每页数量，再用上一页输出的 page_token 翻页
  feishu-cli search docs "技术方案" --count 10 -o json
  feishu-cli search docs "技术方案" --count 10 --page-token <page_token>

  # 也可以手动指定 Token
  feishu-cli search docs "产品需求" --user-access-token <token>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query := args[0]
		count, _ := cmd.Flags().GetInt("count")
		offset, _ := cmd.Flags().GetInt("offset")
		pageToken, _ := cmd.Flags().GetString("page-token")
		ownerIDsStr, _ := cmd.Flags().GetString("owner-ids")
		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")
		docsTypesStr, _ := cmd.Flags().GetString("docs-types")
		output, _ := cmd.Flags().GetString("output")

		// 本地参数校验（用法错误 exit 2）先于身份解析
		pageSize, err := searchDocsPageSize(count, offset)
		if err != nil {
			return err
		}
		docsTypes, err := normalizeSearchDocsTypes(docsTypesStr)
		if err != nil {
			return err
		}

		// 获取 user access token（搜索 API 必需）
		userAccessToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		opts := client.SearchDocWikiOptions{
			Query:     query,
			PageSize:  pageSize,
			PageToken: strings.TrimSpace(pageToken),
			OwnerIDs:  splitAndTrim(ownerIDsStr),
			ChatIDs:   splitAndTrim(chatIDsStr),
			DocTypes:  docsTypes,
		}

		result, err := client.SearchDocWiki(opts, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result.ResUnits) == 0 {
			fmt.Println("未找到匹配的文档")
			return nil
		}

		fmt.Printf("搜索结果（共 %d 条）:\n\n", result.Total)
		for i, unit := range result.ResUnits {
			fmt.Printf("[%d] %s\n", i+1, unit.Title)
			if unit.DocsType != "" {
				fmt.Printf("    类型: %s\n", unit.DocsType)
			}
			if unit.URL != "" {
				fmt.Printf("    链接: %s\n", unit.URL)
			}
			if unit.OwnerID != "" {
				fmt.Printf("    所有者: %s\n", unit.OwnerID)
			}
			fmt.Println()
		}

		if result.HasMore {
			if result.PageToken != "" {
				fmt.Printf("\n还有更多结果，使用 --page-token %s 获取下一页\n", result.PageToken)
			} else {
				fmt.Printf("\n还有更多结果（服务端未返回 page_token）\n")
			}
		}
		return nil
	},
}

// searchDocsPageSize 把旧 --count / --offset 映射到 v2 分页：count → page_size（夹到 ≤20，
// 超出在 stderr 提示）；offset 无法映射，>0 时报用法错误并提示改用 --page-token。
func searchDocsPageSize(count, offset int) (int, error) {
	if offset < 0 {
		return 0, clierr.Usagef("--offset 不能为负数（当前 %d）", offset)
	}
	if offset > 0 {
		return 0, clierr.Usagef("--offset 已不再支持：search docs 已迁移到 v2 搜索（/open-apis/search/v2/doc_wiki/search），只能按 page_token 翻页；请去掉 --offset，用上一页输出的 page_token 传 --page-token")
	}
	if count < 0 {
		return 0, clierr.Usagef("--count 不能为负数（当前 %d）", count)
	}
	if count > searchDocsMaxPageSize {
		fmt.Fprintf(os.Stderr, "提示: v2 搜索单页最多 %d 条，--count %d 已按 %d 处理；更多结果请用 --page-token 翻页\n", searchDocsMaxPageSize, count, searchDocsMaxPageSize)
		return searchDocsMaxPageSize, nil
	}
	return count, nil
}

// normalizeSearchDocsTypes 校验 --docs-types 并转为 v2 filter.doc_types 的大写枚举
func normalizeSearchDocsTypes(raw string) ([]string, error) {
	items := splitAndTrim(raw)
	if len(items) == 0 {
		return nil, nil
	}
	allowed := make(map[string]bool, len(searchDocsTypes))
	for _, t := range searchDocsTypes {
		allowed[t] = true
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		lower := strings.ToLower(item)
		if !allowed[lower] {
			return nil, clierr.Usagef("不支持的 --docs-types 取值 %q，可选值: %s", item, strings.Join(searchDocsTypes, ", "))
		}
		out = append(out, strings.ToUpper(lower))
	}
	return out, nil
}

func init() {
	searchCmd.AddCommand(searchDocsCmd)

	searchDocsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	searchDocsCmd.Flags().Int("count", searchDocsMaxPageSize, "每页数量（1-20，超出按 20 处理）")
	searchDocsCmd.Flags().Int("offset", 0, "已废弃：v2 只能按 --page-token 翻页，传 >0 报用法错误")
	searchDocsCmd.Flags().String("page-token", "", "分页标记（上一页输出的 page_token）")
	searchDocsCmd.Flags().String("owner-ids", "", "文件所有者 Open ID 列表（逗号分隔，映射 v2 creator_ids）")
	searchDocsCmd.Flags().String("chat-ids", "", "文件所在群 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().String("docs-types", "", "文档类型列表（逗号分隔：doc/docx/sheet/slides/bitable/mindnote/file/wiki/shortcut/folder/catalog）")
	searchDocsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
