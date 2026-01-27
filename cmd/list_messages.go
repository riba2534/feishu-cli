package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listMessagesCmd = &cobra.Command{
	Use:   "list",
	Short: "获取消息列表",
	Long: `获取指定会话中的消息列表。

参数:
  --container-id       会话 ID（必填）
  --container-id-type  会话 ID 类型（默认: chat）
  --start-time         起始时间戳（秒）
  --end-time           结束时间戳（秒）
  --sort-type          排序方式（ByCreateTimeAsc/ByCreateTimeDesc）
  --page-size          每页数量（默认: 20，最大: 50）
  --page-token         分页标记
  --output, -o         输出格式（json）

示例:
  # 获取群聊消息列表
  feishu-cli msg list --container-id oc_xxx

  # 获取指定时间范围内的消息
  feishu-cli msg list --container-id oc_xxx --start-time 1609459200 --end-time 1609545600

  # 分页获取
  feishu-cli msg list --container-id oc_xxx --page-size 10

  # JSON 格式输出
  feishu-cli msg list --container-id oc_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		containerID, _ := cmd.Flags().GetString("container-id")
		containerIDType, _ := cmd.Flags().GetString("container-id-type")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		sortType, _ := cmd.Flags().GetString("sort-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		opts := client.ListMessagesOptions{
			ContainerIDType: containerIDType,
			StartTime:       startTime,
			EndTime:         endTime,
			SortType:        sortType,
			PageSize:        pageSize,
			PageToken:       pageToken,
		}

		result, err := client.ListMessages(containerID, opts)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"items":      result.Items,
				"page_token": result.PageToken,
				"has_more":   result.HasMore,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("消息列表（共 %d 条）:\n", len(result.Items))
			for i, msg := range result.Items {
				msgID := ""
				msgType := ""
				createTime := ""
				senderID := ""
				if msg.MessageId != nil {
					msgID = *msg.MessageId
				}
				if msg.MsgType != nil {
					msgType = *msg.MsgType
				}
				if msg.CreateTime != nil {
					createTime = *msg.CreateTime
				}
				if msg.Sender != nil && msg.Sender.Id != nil {
					senderID = *msg.Sender.Id
				}
				fmt.Printf("\n[%d] 消息 ID: %s\n", i+1, msgID)
				fmt.Printf("    类型: %s\n", msgType)
				fmt.Printf("    发送者: %s\n", senderID)
				fmt.Printf("    创建时间: %s\n", createTime)
			}
			if result.HasMore {
				fmt.Printf("\n还有更多消息，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(listMessagesCmd)
	listMessagesCmd.Flags().String("container-id", "", "会话 ID")
	listMessagesCmd.Flags().String("container-id-type", "chat", "会话 ID 类型（chat）")
	listMessagesCmd.Flags().String("start-time", "", "起始时间戳（秒）")
	listMessagesCmd.Flags().String("end-time", "", "结束时间戳（秒）")
	listMessagesCmd.Flags().String("sort-type", "", "排序方式（ByCreateTimeAsc/ByCreateTimeDesc）")
	listMessagesCmd.Flags().Int("page-size", 20, "每页数量（最大 50）")
	listMessagesCmd.Flags().String("page-token", "", "分页标记")
	listMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(listMessagesCmd, "container-id")
}
