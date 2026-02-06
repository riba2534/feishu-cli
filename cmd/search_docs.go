package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchDocsCmd = &cobra.Command{
	Use:   "docs <query>",
	Short: "搜索文档",
	Long: `搜索飞书文档、表格、幻灯片等。

注意：此功能需要 User Access Token（用户授权令牌）和 search:docs:read 权限。

参数:
  query           搜索关键词（必需）

选项:
  --doc-types     文档类型过滤（doc/sheet/slides/wiki_database/wiki_doc/wiki_note/wiki_sheet，逗号分隔）
  --owner-ids     文档所有者用户 ID 列表（逗号分隔）
  --creator-ids   文档创建者用户 ID 列表（逗号分隔）
  --chat-ids      文档所在群组 ID 列表（逗号分隔）
  --doc-created   文档创建时间筛选（如: >=2024-01-01, <=2024-12-31）
  --doc-updated   文档更新时间筛选
  --page-size     每页数量（默认 20）
  --page-token    分页 token
  --user-id-type  用户 ID 类型（open_id/union_id/user_id，默认 open_id）

示例:
  # 搜索包含"项目"的文档
  feishu-cli search docs "项目"

  # 搜索特定创建者的文档
  feishu-cli search docs "项目" --creator-ids ou_xxx

  # 搜索特定类型的文档（仅文档和表格）
  feishu-cli search docs "项目" --doc-types doc,sheet

  # 搜索最近更新的文档
  feishu-cli search docs "项目" --doc-updated ">=2024-01-01"`,
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
			// 尝试从 token 文件获取（支持自动刷新）
			var err error
			userAccessToken, err = client.GetUserAccessToken()
			if err != nil {
				return fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n"+
					"  1. 命令行参数: --user-access-token <token>\n"+
					"  2. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n"+
					"  3. 配置文件: user_access_token: <token>\n"+
					"  4. 运行授权命令: feishu-cli auth login\n"+
					"\n错误: %w", err)
			}
		}

		// 获取其他参数
		docTypesStr, _ := cmd.Flags().GetString("doc-types")
		ownerIDsStr, _ := cmd.Flags().GetString("owner-ids")
		creatorIDsStr, _ := cmd.Flags().GetString("creator-ids")
		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")
		docCreated, _ := cmd.Flags().GetString("doc-created")
		docUpdated, _ := cmd.Flags().GetString("doc-updated")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		// 解析逗号分隔的列表
		var docTypes, ownerIDs, creatorIDs, chatIDs []string
		if docTypesStr != "" {
			docTypes = strings.Split(docTypesStr, ",")
		}
		if ownerIDsStr != "" {
			ownerIDs = strings.Split(ownerIDsStr, ",")
		}
		if creatorIDsStr != "" {
			creatorIDs = strings.Split(creatorIDsStr, ",")
		}
		if chatIDsStr != "" {
			chatIDs = strings.Split(chatIDsStr, ",")
		}

		opts := client.SearchDocsOptions{
			Query:        query,
			DocTypes:     docTypes,
			OwnerIDs:     ownerIDs,
			CreatorIDs:   creatorIDs,
			ChatIDs:      chatIDs,
			DocCreatedAt: docCreated,
			DocUpdatedAt: docUpdated,
			PageSize:     pageSize,
			PageToken:    pageToken,
			UserIDType:   userIDType,
		}

		result, err := client.SearchDocs(opts, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			if len(result.Docs) == 0 {
				fmt.Println("未找到匹配的文档")
				return nil
			}

			fmt.Printf("搜索结果（共 %d 条）:\n\n", len(result.Docs))
			for i, doc := range result.Docs {
				fmt.Printf("[%d] %s\n", i+1, doc.DocName)
				fmt.Printf("    类型: %s\n", doc.DocType)
				fmt.Printf("    Token: %s\n", doc.DocToken)
				if doc.CreatorID != "" {
					fmt.Printf("    创建者: %s\n", doc.CreatorID)
				}
				if doc.OwnerName != "" {
					fmt.Printf("    所有者: %s\n", doc.OwnerName)
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
	searchDocsCmd.Flags().String("doc-types", "", "文档类型（doc/sheet/slides/wiki_database/wiki_doc/wiki_note/wiki_sheet，逗号分隔）")
	searchDocsCmd.Flags().String("owner-ids", "", "文档所有者用户 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().String("creator-ids", "", "文档创建者用户 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().String("chat-ids", "", "文档所在群组 ID 列表（逗号分隔）")
	searchDocsCmd.Flags().String("doc-created", "", "文档创建时间筛选（如: \u003e=2024-01-01）")
	searchDocsCmd.Flags().String("doc-updated", "", "文档更新时间筛选（如: \u003e=2024-01-01）")
	searchDocsCmd.Flags().Int("page-size", 20, "每页数量")
	searchDocsCmd.Flags().String("page-token", "", "分页 token")
	searchDocsCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/union_id/user_id）")
	searchDocsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
