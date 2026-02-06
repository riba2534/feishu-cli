package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchMessagesCmd = &cobra.Command{
	Use:   "messages <query>",
	Short: "搜索消息",
	Long: `搜索飞书消息。

注意：此功能需要 User Access Token（用户授权令牌）。

参数:
  query           搜索关键词（必需）

选项:
  --chat-ids      指定搜索的会话 ID 列表（逗号分隔）
  --from-ids      指定消息发送者用户 ID 列表（逗号分隔）
  --message-type  消息类型过滤（file/image/media）
  --chat-type     会话类型（group_chat/p2p_chat）
  --from-type     发送者类型（bot/user）
  --start-time    消息发送起始时间（Unix 时间戳，秒）
  --end-time      消息发送结束时间（Unix 时间戳，秒）
  --page-size     每页数量（默认 20）
  --page-token    分页 token
  --user-id-type  用户 ID 类型（open_id/union_id/user_id，默认 open_id）

示例:
  # 搜索包含"会议"的消息
  feishu-cli search messages "会议" --user-access-token <token>

  # 搜索指定会话中的消息
  feishu-cli search messages "会议" --chat-ids oc_xxx,oc_yyy

  # 搜索图片类型的消息
  feishu-cli search messages "图片" --message-type image

  # 搜索指定时间范围内的消息
  feishu-cli search messages "项目" --start-time 1704067200 --end-time 1704153600

  # 使用环境变量设置 token
  export FEISHU_USER_ACCESS_TOKEN="u-xxx"
  feishu-cli search messages "会议"`,
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
				return fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n" +
					"  1. 命令行参数: --user-access-token <token>\n" +
					"  2. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
					"  3. 配置文件: user_access_token: <token>\n" +
					"  4. 运行授权命令: feishu-cli auth login\n" +
					"\n错误: %w", err)
			}
		}

		// 获取其他参数
		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")
		fromIDsStr, _ := cmd.Flags().GetString("from-ids")
		atChatterIDsStr, _ := cmd.Flags().GetString("at-chatter-ids")
		messageType, _ := cmd.Flags().GetString("message-type")
		chatType, _ := cmd.Flags().GetString("chat-type")
		fromType, _ := cmd.Flags().GetString("from-type")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		// 解析逗号分隔的列表
		var chatIDs, fromIDs, atChatterIDs []string
		if chatIDsStr != "" {
			chatIDs = strings.Split(chatIDsStr, ",")
		}
		if fromIDsStr != "" {
			fromIDs = strings.Split(fromIDsStr, ",")
		}
		if atChatterIDsStr != "" {
			atChatterIDs = strings.Split(atChatterIDsStr, ",")
		}

		opts := client.SearchMessagesOptions{
			Query:        query,
			ChatIDs:      chatIDs,
			FromIDs:      fromIDs,
			AtChatterIDs: atChatterIDs,
			MessageType:  messageType,
			ChatType:     chatType,
			FromType:     fromType,
			StartTime:    startTime,
			EndTime:      endTime,
			PageSize:     pageSize,
			PageToken:    pageToken,
			UserIDType:   userIDType,
		}

		result, err := client.SearchMessages(opts, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			if len(result.MessageIDs) == 0 {
				fmt.Println("未找到匹配的消息")
				return nil
			}

			fmt.Printf("搜索结果（共 %d 条）:\n\n", len(result.MessageIDs))
			for i, msgID := range result.MessageIDs {
				fmt.Printf("[%d] 消息 ID: %s\n", i+1, msgID)
			}

			if result.HasMore {
				fmt.Printf("\n还有更多结果，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	searchCmd.AddCommand(searchMessagesCmd)

	searchMessagesCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	searchMessagesCmd.Flags().String("chat-ids", "", "会话 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("from-ids", "", "消息发送者用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("at-chatter-ids", "", "@的用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("message-type", "", "消息类型（file/image/media）")
	searchMessagesCmd.Flags().String("chat-type", "", "会话类型（group_chat/p2p_chat）")
	searchMessagesCmd.Flags().String("from-type", "", "发送者类型（bot/user）")
	searchMessagesCmd.Flags().String("start-time", "", "消息发送起始时间（Unix 时间戳）")
	searchMessagesCmd.Flags().String("end-time", "", "消息发送结束时间（Unix 时间戳）")
	searchMessagesCmd.Flags().Int("page-size", 20, "每页数量")
	searchMessagesCmd.Flags().String("page-token", "", "分页 token")
	searchMessagesCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/union_id/user_id）")
	searchMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
