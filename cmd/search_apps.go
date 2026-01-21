package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchAppsCmd = &cobra.Command{
	Use:   "apps <query>",
	Short: "搜索应用",
	Long: `搜索飞书应用。

注意：此功能需要 User Access Token（用户授权令牌）。

参数:
  query           搜索关键词（必需）

选项:
  --page-size     每页数量（默认 20）
  --page-token    分页 token
  --user-id-type  用户 ID 类型（open_id/union_id/user_id，默认 open_id）

示例:
  # 搜索应用
  feishu-cli search apps "审批" --user-access-token <token>

  # 使用环境变量设置 token
  export FEISHU_USER_ACCESS_TOKEN="u-xxx"
  feishu-cli search apps "审批"

  # 分页获取更多结果
  feishu-cli search apps "审批" --page-size 50`,
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
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		opts := client.SearchAppsOptions{
			Query:      query,
			PageSize:   pageSize,
			PageToken:  pageToken,
			UserIDType: userIDType,
		}

		result, err := client.SearchApps(opts, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(result.AppIDs) == 0 {
				fmt.Println("未找到匹配的应用")
				return nil
			}

			fmt.Printf("搜索结果（共 %d 个应用）:\n\n", len(result.AppIDs))
			for i, appID := range result.AppIDs {
				fmt.Printf("[%d] 应用 ID: %s\n", i+1, appID)
			}

			if result.HasMore {
				fmt.Printf("\n还有更多结果，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	searchCmd.AddCommand(searchAppsCmd)

	searchAppsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	searchAppsCmd.Flags().Int("page-size", 20, "每页数量")
	searchAppsCmd.Flags().String("page-token", "", "分页 token")
	searchAppsCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/union_id/user_id）")
	searchAppsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
