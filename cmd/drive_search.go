package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
  --creator-ids     CSV，所有者 open_id 列表（接口字段名为 creator_ids，实际按所有者匹配）；与 --mine 互斥
  --mine            只看我拥有的文档（等价 --creator-ids <当前用户 open_id>）
  --original-creator-ids  CSV，原始创建者 open_id 列表（创建者不随所有权转移变化）；与 --created-by-me 互斥
  --created-by-me   只看我创建的文档（等价 --original-creator-ids <当前用户 open_id>）
  --edited-since / --edited-until        我编辑过的时间窗（服务端按小时聚合，非整点向外取整并提示）
  --commented-since / --commented-until  我评论过的时间窗（按小时聚合）
  --opened-since / --opened-until        我打开过的时间窗；服务端单次最多 3 个月：超过 90 天只查最近 90 天切片，
                                         stderr 列出全部切片的绝对时间；超过 365 天 exit 2
  --created-since / --created-until      文档创建时间窗
                    时间值: 7d / 1m(30 天) / 1y(365 天) / YYYY-MM-DD[ HH:MM:SS] / RFC3339 / unix 秒
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
  feishu-cli drive search --query "RFC" --only-title -o json
  feishu-cli drive search --created-since 7d --created-by-me -o json
  feishu-cli drive search --edited-since 2026-10-01 --edited-until 2026-10-31 --doc-types DOCX
  feishu-cli drive search --opened-since 6m --mine`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		mine, _ := cmd.Flags().GetBool("mine")
		createdByMe, _ := cmd.Flags().GetBool("created-by-me")
		originalCreators, _ := cmd.Flags().GetString("original-creator-ids")

		if folders != "" && spaces != "" {
			return clierr.Usagef("--folder-tokens 与 --space-ids 互斥")
		}
		if mine && strings.TrimSpace(creators) != "" {
			return clierr.Usagef("--mine 与 --creator-ids 不能同时使用")
		}
		if createdByMe && strings.TrimSpace(originalCreators) != "" {
			return clierr.Usagef("--created-by-me 与 --original-creator-ids 不能同时使用")
		}
		timeInputs := map[string]driveSearchTimeInput{}
		for _, d := range driveSearchTimeDims {
			since, _ := cmd.Flags().GetString(d.flag + "-since")
			until, _ := cmd.Flags().GetString(d.flag + "-until")
			timeInputs[d.flag] = driveSearchTimeInput{Since: strings.TrimSpace(since), Until: strings.TrimSpace(until)}
		}
		timeRanges, notices, err := buildDriveSearchTimeRanges(timeInputs, time.Now())
		if err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}

		userToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		creatorIDs := splitAndTrim(creators)
		originalCreatorIDs := splitAndTrim(originalCreators)
		if mine || createdByMe {
			openID, err := resolveCurrentAuthedUserID(cmd, "open_id")
			if err != nil {
				return err
			}
			if mine {
				creatorIDs = []string{openID}
			}
			if createdByMe {
				originalCreatorIDs = []string{openID}
			}
		}
		for _, n := range notices {
			fmt.Fprintln(cmd.ErrOrStderr(), n)
		}

		opts := client.DriveSearchOptions{
			Query:        query,
			PageToken:    pageToken,
			PageSize:     pageSize,
			CreatorIDs:   creatorIDs,
			FolderTokens: splitAndTrim(folders),
			SpaceIDs:     splitAndTrim(spaces),
			ChatIDs:      splitAndTrim(chats),
			SharerIDs:    splitAndTrim(sharers),
			DocTypes:     normalizeDocTypes(docTypes),
			OnlyTitle:    onlyTitle,
			OnlyComment:  onlyComment,
			Sort:         sortBy,

			OriginalCreatorIDs: originalCreatorIDs,
			TimeRanges:         timeRanges,
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
	driveSearchCmd.Flags().String("creator-ids", "", "CSV，所有者 open_id（接口字段 creator_ids）")
	driveSearchCmd.Flags().Bool("mine", false, "只看我拥有的文档（等价 --creator-ids <自己>）")
	driveSearchCmd.Flags().String("original-creator-ids", "", "CSV，原始创建者 open_id")
	driveSearchCmd.Flags().Bool("created-by-me", false, "只看我创建的文档（等价 --original-creator-ids <自己>）")
	for _, d := range driveSearchTimeDims {
		driveSearchCmd.Flags().String(d.flag+"-since", "", "时间窗起点（7d/1m/1y/YYYY-MM-DD/RFC3339/unix 秒）")
		driveSearchCmd.Flags().String(d.flag+"-until", "", "时间窗终点（同上）")
	}
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
