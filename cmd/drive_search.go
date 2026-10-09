package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var driveSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "在云盘 / 知识库范围内搜索文档（v2 doc_wiki/search，扁平 filter）",
	Long: `走 /open-apis/search/v2/doc_wiki/search v2 端点，比 search docs（v1）支持更丰富的扁平 filter：
folder-tokens、space-ids、creator-ids、sharer-ids、only-title、sort 排序等。

可选:
  --query           关键字（可空，纯按 filter 浏览）
  --creator-ids     CSV，创建者 open_id 列表
  --folder-tokens   CSV，限定在某些云盘文件夹（与 --space-ids 互斥）
  --space-ids       CSV，限定在某些知识库 space（与 --folder-tokens 互斥）
  --chat-ids        CSV，限定群分享列表
  --sharer-ids      CSV，分享者 open_id 列表
  --doc-types       CSV，类型（DOC/DOCX/SHEET/BITABLE/MINDNOTE/FILE/WIKI/FOLDER/CATALOG/SLIDES/SHORTCUT）
  --only-title      仅匹配标题
  --only-comment    仅搜评论
  --sort            排序：default / edit_time / edit_time_asc / open_time / create_time
  --page-size       1-20，默认 15
  --page-token      分页 token
  --output / -o     输出格式（json）

权限:
  - User Access Token（必需）
  - search:docs:read

示例:
  feishu-cli drive search --query "季度报告" --doc-types DOC,SHEET --sort edit_time
  feishu-cli drive search --folder-tokens fldxxx --doc-types FILE
  feishu-cli drive search --space-ids spcxxx --query "API 设计"
  feishu-cli drive search --query "RFC" --only-title -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		query, _ := cmd.Flags().GetString("query")
		creators, _ := cmd.Flags().GetString("creator-ids")
		folders, _ := cmd.Flags().GetString("folder-tokens")
		spaces, _ := cmd.Flags().GetString("space-ids")
		chats, _ := cmd.Flags().GetString("chat-ids")
		sharers, _ := cmd.Flags().GetString("sharer-ids")
		docTypes, _ := cmd.Flags().GetString("doc-types")
		onlyTitle, _ := cmd.Flags().GetBool("only-title")
		onlyComment, _ := cmd.Flags().GetBool("only-comment")
		sortBy, _ := cmd.Flags().GetString("sort")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		if folders != "" && spaces != "" {
			return fmt.Errorf("--folder-tokens 与 --space-ids 互斥")
		}

		opts := client.DriveSearchOptions{
			Query:        query,
			PageToken:    pageToken,
			PageSize:     pageSize,
			CreatorIDs:   splitAndTrim(creators),
			FolderTokens: splitAndTrim(folders),
			SpaceIDs:     splitAndTrim(spaces),
			ChatIDs:      splitAndTrim(chats),
			SharerIDs:    splitAndTrim(sharers),
			DocTypes:     normalizeDocTypes(docTypes),
			OnlyTitle:    onlyTitle,
			OnlyComment:  onlyComment,
			Sort:         sortBy,
		}

		result, err := client.DriveSearchV2(opts, userToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("搜索结果（共 %d 条，has_more=%v）:\n\n", result.Total, result.HasMore)
		if len(result.Items) == 0 {
			fmt.Println("未找到匹配结果")
			return nil
		}
		for i, it := range result.Items {
			title := stripSearchHighlight(getString(it, "title_highlighted"))
			if title == "" {
				title = getString(it, "title")
			}
			meta, _ := it["result_meta"].(map[string]any)
			entityType := getString(it, "entity_type")
			docsType := getString(meta, "doc_types")
			if docsType == "" {
				docsType = entityType
			}
			url := getString(meta, "url")
			ownerName := getString(meta, "owner_name")
			ownerID := getString(meta, "owner_id")
			token := getString(meta, "token")

			fmt.Printf("[%d] %s\n", i+1, title)
			if docsType != "" {
				fmt.Printf("    类型: %s\n", docsType)
			}
			if url != "" {
				fmt.Printf("    链接: %s\n", url)
			} else if token != "" {
				fmt.Printf("    token: %s\n", token)
			}
			if ownerName != "" || ownerID != "" {
				fmt.Printf("    所有者: %s (%s)\n", ownerName, ownerID)
			}
			fmt.Println()
		}
		if result.HasMore && result.PageToken != "" {
			fmt.Printf("下一页 --page-token <token>（见 -o json 输出的 page_token 字段）\n")
		}
		return nil
	},
}

// stripSearchHighlight 去掉 v2 搜索响应里 title_highlighted / summary_highlighted 中的 <h> 标记。
func stripSearchHighlight(s string) string {
	s = strings.ReplaceAll(s, "<h>", "")
	s = strings.ReplaceAll(s, "</h>", "")
	return s
}

// getString 安全地从 map 取 string 字段，nil/缺失/类型错都返回 ""。
func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// normalizeDocTypes 把 doc-types 列表转大写（v2 端点要求大写枚举值）。
func normalizeDocTypes(s string) []string {
	parts := splitAndTrim(s)
	for i, p := range parts {
		parts[i] = strings.ToUpper(p)
	}
	return parts
}

func init() {
	driveCmd.AddCommand(driveSearchCmd)
	driveSearchCmd.Flags().String("query", "", "关键字（可空）")
	driveSearchCmd.Flags().String("creator-ids", "", "CSV，创建者 open_id")
	driveSearchCmd.Flags().String("folder-tokens", "", "CSV，限定云盘文件夹")
	driveSearchCmd.Flags().String("space-ids", "", "CSV，限定知识库 space")
	driveSearchCmd.Flags().String("chat-ids", "", "CSV，限定群分享")
	driveSearchCmd.Flags().String("sharer-ids", "", "CSV，分享者 open_id")
	driveSearchCmd.Flags().String("doc-types", "", "CSV，文档类型（大写）")
	driveSearchCmd.Flags().Bool("only-title", false, "仅匹配标题")
	driveSearchCmd.Flags().Bool("only-comment", false, "仅搜评论")
	driveSearchCmd.Flags().String("sort", "", "排序方式")
	driveSearchCmd.Flags().Int("page-size", 15, "1-20")
	driveSearchCmd.Flags().String("page-token", "", "分页 token")
	driveSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveSearchCmd.Flags().String("user-access-token", "", "User Access Token（搜索 API 必需）")
}
