package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchDocsCmd = &cobra.Command{
	Use:   "docs <query>",
	Short: "搜索文档和 Wiki",
	Long: `搜索飞书文档和 Wiki。

注意：此功能需要 User Access Token（用户授权令牌）。

参数:
  query           搜索关键词（必需）

选项:
  --doc-types     文档类型列表（逗号分隔，必须大写，可选值：DOC/SHEET/BITABLE/MINDNOTE/FILE/WIKI/DOCX/FOLDER/CATALOG/SLIDES/SHORTCUT）
  --folder-tokens 文件夹 Token 列表（逗号分隔）
  --space-ids     Wiki 空间 ID 列表（逗号分隔）
  --creator-ids   创建者 ID 列表（逗号分隔）
  --only-title    仅搜索标题（默认 false，搜索全文）
  --sort-type     排序方式（EditedTime/CreatedTime/OpenedTime）
  --page-size     每页数量（默认 20）
  --page-token    分页 token

示例:
  # 搜索包含"产品需求"的文档
  feishu-cli search docs "产品需求" --user-access-token <token>

  # 搜索特定类型的文档（注意：类型必须大写）
  feishu-cli search docs "季度报告" --doc-types DOC,SHEET --user-access-token <token>

  # 搜索特定文件夹下的文档
  feishu-cli search docs "会议纪要" --folder-tokens fldcnxxxxxxxxxxxxxx --user-access-token <token>

  # 仅搜索标题
  feishu-cli search docs "技术方案" --only-title --user-access-token <token>

  # 搜索 Wiki 空间中的文档
  feishu-cli search docs "项目文档" --doc-types WIKI --space-ids space_xxxxxxxxxxxx --user-access-token <token>

  # 按最后编辑时间排序
  feishu-cli search docs "文档" --sort-type EditedTime --user-access-token <token>

  # 使用环境变量设置 token（推荐）
  export FEISHU_USER_ACCESS_TOKEN="u-xxx"
  feishu-cli search docs "产品需求"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query := args[0]

		// 获取 user access token
		userAccessToken, _ := cmd.Flags().GetString("user-access-token")
		if userAccessToken == "" {
			userAccessToken = config.Get().UserAccessToken
		}
		if userAccessToken == "" {
			userAccessToken = os.Getenv("FEISHU_USER_ACCESS_TOKEN")
		}
		if userAccessToken == "" {
			return fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n" +
				"  1. 命令行参数: --user-access-token <token>\n" +
				"  2. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
				"  3. 配置文件: user_access_token: <token>")
		}

		// 获取其他参数
		docTypesStr, _ := cmd.Flags().GetString("doc-types")
		folderTokensStr, _ := cmd.Flags().GetString("folder-tokens")
		spaceIDsStr, _ := cmd.Flags().GetString("space-ids")
		creatorIDsStr, _ := cmd.Flags().GetString("creator-ids")
		onlyTitleFlag, _ := cmd.Flags().GetBool("only-title")
		sortType, _ := cmd.Flags().GetString("sort-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		// 解析逗号分隔的列表
		var docTypes, folderTokens, spaceIDs, creatorIDs []string
		if docTypesStr != "" {
			docTypes = strings.Split(docTypesStr, ",")
			// 验证 doc-types 是否合法
			validDocTypes := map[string]bool{
				"DOC":      true,
				"SHEET":    true,
				"BITABLE":  true,
				"MINDNOTE": true,
				"FILE":     true,
				"WIKI":     true,
				"DOCX":     true,
				"FOLDER":   true,
				"CATALOG":  true,
				"SLIDES":   true,
				"SHORTCUT": true,
			}
			for _, dt := range docTypes {
				dt = strings.TrimSpace(dt)
				if !validDocTypes[dt] {
					return fmt.Errorf("不支持的文档类型: %s\n支持的类型（必须大写）: DOC, SHEET, BITABLE, MINDNOTE, FILE, WIKI, DOCX, FOLDER, CATALOG, SLIDES, SHORTCUT", dt)
				}
			}
		}
		if folderTokensStr != "" {
			folderTokens = strings.Split(folderTokensStr, ",")
		}
		if spaceIDsStr != "" {
			spaceIDs = strings.Split(spaceIDsStr, ",")
		}
		if creatorIDsStr != "" {
			creatorIDs = strings.Split(creatorIDsStr, ",")
		}

		// 处理 only-title 参数
		var onlyTitle *bool
		if cmd.Flags().Changed("only-title") {
			onlyTitle = &onlyTitleFlag
		}

		opts := client.SearchDocWikiOptions{
			Query:        query,
			DocTypes:     docTypes,
			FolderTokens: folderTokens,
			SpaceIDs:     spaceIDs,
			CreatorIDs:   creatorIDs,
			OnlyTitle:    onlyTitle,
			SortType:     sortType,
			PageSize:     pageSize,
			PageToken:    pageToken,
		}

		result, err := client.SearchDocWiki(opts, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			if len(result.ResUnits) == 0 {
				fmt.Println("未找到匹配的文档")
				return nil
			}

			fmt.Printf("搜索结果（共 %d 条）:\n\n", result.Total)
			for i, unit := range result.ResUnits {
				fmt.Printf("[%d] %s\n", i+1, unit.TitleHighlighted)
				if unit.EntityType != "" {
					fmt.Printf("    类型: %s\n", unit.EntityType)
				}
				if unit.DocTypes != "" {
					fmt.Printf("    文档类型: %s\n", unit.DocTypes)
				}
				if unit.URL != "" {
					fmt.Printf("    链接: %s\n", unit.URL)
				}
				if unit.OwnerName != "" {
					fmt.Printf("    所有者: %s\n", unit.OwnerName)
				}
				if unit.CreateTime > 0 {
					createTime := time.Unix(unit.CreateTime, 0)
					fmt.Printf("    创建时间: %s\n", createTime.Format("2006-01-02 15:04:05"))
				}
				if unit.UpdateTime > 0 {
					updateTime := time.Unix(unit.UpdateTime, 0)
					fmt.Printf("    更新时间: %s\n", updateTime.Format("2006-01-02 15:04:05"))
				}
				if unit.SummaryHighlighted != "" {
					fmt.Printf("    摘要: %s\n", unit.SummaryHighlighted)
				}
				fmt.Println()
			}

			if result.HasMore {
				fmt.Printf("还有更多结果，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	searchCmd.AddCommand(searchDocsCmd)

	searchDocsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	searchDocsCmd.Flags().String("doc-types", "", "文档类型列表（逗号分隔，必须大写，可选值：DOC/SHEET/BITABLE/MINDNOTE/FILE/WIKI/DOCX/FOLDER/CATALOG/SLIDES/SHORTCUT）")
	searchDocsCmd.Flags().String("folder-tokens", "", "文件夹 Token 列表（逗号分隔）")
	searchDocsCmd.Flags().String("space-ids", "", "Wiki 空间 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().String("creator-ids", "", "创建者 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().Bool("only-title", false, "仅搜索标题")
	searchDocsCmd.Flags().String("sort-type", "", "排序方式（EditedTime/CreatedTime/OpenedTime）")
	searchDocsCmd.Flags().Int("page-size", 20, "每页数量")
	searchDocsCmd.Flags().String("page-token", "", "分页 token")
	searchDocsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
