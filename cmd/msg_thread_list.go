package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgThreadMessagesCmd = &cobra.Command{
	Use:   "thread-messages <thread_id>",
	Short: "获取话题/线程中的消息列表",
	Long: `获取指定话题或线程中的消息列表。

参数:
  thread_id    线程 ID（omt_xxx 格式）

选项:
  --sort        排序方式（ByCreateTimeAsc 或 ByCreateTimeDesc，默认 ByCreateTimeAsc）
  --page-size   每页数量（默认 50）
  --page-token  分页标记
  --start-time  起始时间（毫秒级时间戳）
  --end-time    结束时间（毫秒级时间戳）

示例:
  # 获取线程消息
  feishu-cli msg thread-messages omt_xxx

  # 按时间倒序获取
  feishu-cli msg thread-messages omt_xxx --sort ByCreateTimeDesc

  # 分页获取
  feishu-cli msg thread-messages omt_xxx --page-size 20 --page-token xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		threadID := args[0]
		sortType, _ := cmd.Flags().GetString("sort")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		userToken := resolveOptionalUserTokenWithFallback(cmd)

		opts := client.ListMessagesOptions{
			SortType:  sortType,
			PageSize:  pageSize,
			PageToken: pageToken,
			StartTime: startTime,
			EndTime:   endTime,
		}

		result, err := client.ListThreadMessages(threadID, opts, userToken)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	msgCmd.AddCommand(msgThreadMessagesCmd)
	msgThreadMessagesCmd.Flags().String("sort", "ByCreateTimeAsc", "排序方式（ByCreateTimeAsc 或 ByCreateTimeDesc）")
	msgThreadMessagesCmd.Flags().Int("page-size", 50, "每页数量")
	msgThreadMessagesCmd.Flags().String("page-token", "", "分页标记")
	msgThreadMessagesCmd.Flags().String("start-time", "", "起始时间（毫秒级时间戳）")
	msgThreadMessagesCmd.Flags().String("end-time", "", "结束时间（毫秒级时间戳）")
	msgThreadMessagesCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
