package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
  --start-time  起始时间（Unix 秒 / 毫秒 / RFC3339 / YYYY-MM-DD），客户端本地过滤
  --end-time    结束时间（同上，含边界；纯日期取当天 23:59:59），客户端本地过滤

时间过滤说明:
  飞书对 thread 容器会忽略 start_time/end_time，因此 CLI 不把时间范围发给服务端，
  而是对当前页按消息 create_time 本地过滤。过滤只作用于本页：has_more=true 时
  仍需带 --page-token 继续翻页（升序翻页时遇到晚于 --end-time 的消息即可停止）。

示例:
  # 获取线程消息
  feishu-cli msg thread-messages omt_xxx

  # 按时间倒序获取
  feishu-cli msg thread-messages omt_xxx --sort ByCreateTimeDesc

  # 只看某个时间窗口内的回复（秒级时间戳，与 msg history 一致）
  feishu-cli msg thread-messages omt_xxx --start-time 1704067200 --end-time 1704153600

  # 分页获取
  feishu-cli msg thread-messages omt_xxx --page-size 20 --page-token xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		threadID := args[0]
		sortType, _ := cmd.Flags().GetString("sort")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")

		window, err := parseLocalTimeWindow(startTime, endTime)
		if err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userToken := resolveOptionalUserTokenWithFallback(cmd)

		// 注意：不把 start/end 发给服务端——thread 容器下服务端忽略时间范围（实测），
		// 且服务端要求秒而历史文档写的是毫秒，发过去只会制造"过滤生效了"的假象。
		opts := client.ListMessagesOptions{
			SortType:  sortType,
			PageSize:  pageSize,
			PageToken: pageToken,
		}

		result, err := client.ListThreadMessages(threadID, opts, userToken)
		if err != nil {
			return err
		}

		if window.active() {
			before := len(result.Items)
			result.Items = window.filter(result.Items)
			if dropped := before - len(result.Items); dropped > 0 || result.HasMore {
				printThreadWindowNote(cmd.ErrOrStderr(), dropped, result.HasMore)
			}
		}

		return printJSON(result)
	},
}

// localTimeWindow 客户端本地时间窗口（毫秒，含两端）；零值表示该端不限制。
type localTimeWindow struct {
	startMs int64
	endMs   int64
}

func (w localTimeWindow) active() bool { return w.startMs > 0 || w.endMs > 0 }

// contains 判断毫秒时间戳是否落在窗口内；createMs<=0（无法解析）时保守保留。
func (w localTimeWindow) contains(createMs int64) bool {
	if createMs <= 0 {
		return true
	}
	if w.startMs > 0 && createMs < w.startMs {
		return false
	}
	if w.endMs > 0 && createMs > w.endMs {
		return false
	}
	return true
}

func (w localTimeWindow) filter(items []*larkim.Message) []*larkim.Message {
	out := make([]*larkim.Message, 0, len(items))
	for _, m := range items {
		if m == nil {
			continue
		}
		ms, _ := strconv.ParseInt(strings.TrimSpace(client.StringVal(m.CreateTime)), 10, 64)
		if w.contains(ms) {
			out = append(out, m)
		}
	}
	return out
}

// parseLocalTimeWindow 解析 --start-time/--end-time（秒 / 毫秒 / RFC3339 / YYYY-MM-DD）。
// 秒级精度的结束时间按整秒含边界（+999ms），纯日期结束时间取当天 23:59:59。
func parseLocalTimeWindow(start, end string) (localTimeWindow, error) {
	var w localTimeWindow
	if strings.TrimSpace(start) != "" {
		t, err := client.ParseTimeInput(start, false)
		if err != nil {
			return w, clierr.Usagef("--start-time 无效: %v", err)
		}
		w.startMs = t.UnixMilli()
	}
	if strings.TrimSpace(end) != "" {
		t, err := client.ParseTimeInput(end, true)
		if err != nil {
			return w, clierr.Usagef("--end-time 无效: %v", err)
		}
		w.endMs = t.UnixMilli()
		if t.Nanosecond() == 0 {
			w.endMs += int64(time.Second/time.Millisecond) - 1
		}
	}
	if w.startMs > 0 && w.endMs > 0 && w.startMs > w.endMs {
		return w, clierr.Usagef("--start-time 不能晚于 --end-time")
	}
	return w, nil
}

func printThreadWindowNote(errOut io.Writer, dropped int, hasMore bool) {
	if errOut == nil {
		errOut = os.Stderr
	}
	msg := fmt.Sprintf("[提示] --start-time/--end-time 已在客户端按 create_time 本地过滤（服务端对话题容器忽略时间范围），本页过滤掉 %d 条", dropped)
	if hasMore {
		msg += "；has_more=true，窗口内的消息可能在后续页，请带 --page-token 继续翻页"
	}
	fmt.Fprintln(errOut, msg)
}

func init() {
	msgCmd.AddCommand(msgThreadMessagesCmd)
	msgThreadMessagesCmd.Flags().String("sort", "ByCreateTimeAsc", "排序方式（ByCreateTimeAsc 或 ByCreateTimeDesc）")
	msgThreadMessagesCmd.Flags().Int("page-size", 50, "每页数量")
	msgThreadMessagesCmd.Flags().String("page-token", "", "分页标记")
	msgThreadMessagesCmd.Flags().String("start-time", "", "起始时间（Unix 秒/毫秒、RFC3339、YYYY-MM-DD），客户端本地过滤")
	msgThreadMessagesCmd.Flags().String("end-time", "", "结束时间（Unix 秒/毫秒、RFC3339、YYYY-MM-DD，含边界），客户端本地过滤")
	msgThreadMessagesCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
