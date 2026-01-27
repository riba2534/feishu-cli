package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchChatsCmd = &cobra.Command{
	Use:   "search-chats",
	Short: "搜索群聊",
	Long: `搜索飞书群聊列表。

参数:
  --user-id-type   用户 ID 类型 (open_id/union_id/user_id)，默认 open_id
  --query          关键词搜索
  --page-token     分页标记
  --page-size      分页大小 (1-100)，默认 50
  --output, -o     输出格式 (json)

示例:
  # 列出所有群聊
  feishu-cli msg search-chats

  # 搜索包含关键词的群聊
  feishu-cli msg search-chats --query "测试群"

  # 分页获取
  feishu-cli msg search-chats --page-size 20 --page-token xxx

  # JSON 格式输出
  feishu-cli msg search-chats -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		userIDType, _ := cmd.Flags().GetString("user-id-type")
		query, _ := cmd.Flags().GetString("query")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		opts := client.SearchChatsOptions{
			UserIDType: userIDType,
			Query:      query,
			PageToken:  pageToken,
			PageSize:   pageSize,
		}

		result, err := client.SearchChats(opts)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("找到 %d 个群聊:\n\n", len(result.Items))
			for i, chat := range result.Items {
				fmt.Printf("[%d] %s\n", i+1, chat.Name)
				fmt.Printf("    群聊 ID: %s\n", chat.ChatID)
				if chat.Description != "" {
					fmt.Printf("    描述: %s\n", chat.Description)
				}
				if chat.OwnerID != "" {
					fmt.Printf("    群主: %s\n", chat.OwnerID)
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
	msgCmd.AddCommand(searchChatsCmd)
	searchChatsCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型 (open_id/union_id/user_id)")
	searchChatsCmd.Flags().String("query", "", "关键词搜索")
	searchChatsCmd.Flags().String("page-token", "", "分页标记")
	searchChatsCmd.Flags().Int("page-size", 50, "分页大小 (1-100)")
	searchChatsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
