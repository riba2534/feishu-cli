package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getMessageHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "获取会话历史消息",
	Long: `获取飞书会话中的历史消息。

参数:
  --container-id-type   容器类型 (chat)，必填
  --container-id        容器 ID，必填
  --start-time          起始时间（秒级时间戳）
  --end-time            结束时间（秒级时间戳）
  --sort-type           排序方式 (ByCreateTimeAsc/ByCreateTimeDesc)，默认 ByCreateTimeDesc
  --page-size           分页大小 (1-50)，默认 50
  --page-token          分页标记
  --output, -o          输出格式 (json)

排序方式:
  ByCreateTimeAsc    按创建时间升序
  ByCreateTimeDesc   按创建时间降序（默认）

示例:
  # 获取群聊历史消息
  feishu-cli msg history --container-id oc_xxx --container-id-type chat

  # 指定时间范围
  feishu-cli msg history --container-id oc_xxx --container-id-type chat \
    --start-time 1704067200 --end-time 1704153600

  # 按时间升序排列
  feishu-cli msg history --container-id oc_xxx --container-id-type chat \
    --sort-type ByCreateTimeAsc

  # 分页获取
  feishu-cli msg history --container-id oc_xxx --container-id-type chat \
    --page-size 20 --page-token xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		containerIDType, _ := cmd.Flags().GetString("container-id-type")
		containerID, _ := cmd.Flags().GetString("container-id")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		sortType, _ := cmd.Flags().GetString("sort-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		if containerID == "" || containerIDType == "" {
			return fmt.Errorf("必须指定 --container-id 和 --container-id-type")
		}

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

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("找到 %d 条消息:\n\n", len(result.Items))
			for i, msg := range result.Items {
				msgID := ""
				if msg.MessageId != nil {
					msgID = *msg.MessageId
				}
				msgType := ""
				if msg.MsgType != nil {
					msgType = *msg.MsgType
				}
				createTime := ""
				if msg.CreateTime != nil {
					createTime = *msg.CreateTime
				}
				sender := ""
				if msg.Sender != nil && msg.Sender.Id != nil {
					sender = *msg.Sender.Id
				}

				fmt.Printf("[%d] 消息 ID: %s\n", i+1, msgID)
				fmt.Printf("    类型: %s\n", msgType)
				fmt.Printf("    发送者: %s\n", sender)
				fmt.Printf("    时间: %s\n", createTime)
				fmt.Println()
			}
			if result.HasMore {
				fmt.Printf("还有更多消息，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(getMessageHistoryCmd)
	getMessageHistoryCmd.Flags().String("container-id-type", "", "容器类型 (chat)")
	getMessageHistoryCmd.Flags().String("container-id", "", "容器 ID")
	getMessageHistoryCmd.Flags().String("start-time", "", "起始时间（秒级时间戳）")
	getMessageHistoryCmd.Flags().String("end-time", "", "结束时间（秒级时间戳）")
	getMessageHistoryCmd.Flags().String("sort-type", "ByCreateTimeDesc", "排序方式 (ByCreateTimeAsc/ByCreateTimeDesc)")
	getMessageHistoryCmd.Flags().Int("page-size", 50, "分页大小 (1-50)")
	getMessageHistoryCmd.Flags().String("page-token", "", "分页标记")
	getMessageHistoryCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	_ = getMessageHistoryCmd.MarkFlagRequired("container-id-type")
	_ = getMessageHistoryCmd.MarkFlagRequired("container-id")
}
