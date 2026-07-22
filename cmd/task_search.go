package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "按条件搜索任务",
	Long: `按创建人、负责人、关注人、完成状态、截止时间等条件搜索任务。

底层调用飞书任务搜索接口（POST /open-apis/task/v2/tasks/search）：创建人 /
负责人 / 关注人 / 完成状态 / 截止时间等条件在服务端过滤，--keyword 作为服务端
关键词检索（匹配任务标题等）。搜索接口只返回任务 GUID 与链接，命中的每条任务会
自动拉取详情以展示标题、截止时间等信息。

此命令需要 User Access Token（用户授权），解析优先级：
  1. --user-access-token 参数
  2. FEISHU_USER_ACCESS_TOKEN 环境变量
  3. ~/.feishu-cli/token.json（支持自动刷新）
  4. config.yaml 中的 user_access_token

参数:
  --keyword          关键词（服务端检索任务标题等）
  --creator          创建人 open_id，多个用逗号分隔
  --assignee         负责人 open_id，多个用逗号分隔
  --follower         关注人 open_id，多个用逗号分隔
  --completed        只搜索已完成的任务
  --uncompleted      只搜索未完成的任务
  --due-after        截止时间下界（RFC3339 / "2006-01-02 15:04:05" / "2006-01-02"）
  --due-before       截止时间上界（同上）
  --page-size        每页数量（默认 20，最大 30）
  --page-token       分页标记（从指定页开始）
  --page-all         自动翻页拉取全部结果
  --output, -o       输出格式（json）

说明:
  --keyword、--creator/--assignee/--follower、--completed/--uncompleted、
  --due-after/--due-before 至少要提供一个搜索条件。

示例:
  # 搜索我参与的未完成任务
  feishu-cli task search --uncompleted

  # 按关键词搜索
  feishu-cli task search --keyword "评审"

  # 搜索指定负责人、指定截止时间范围内的任务
  feishu-cli task search --assignee ou_xxx --due-after "2026-01-01" --due-before "2026-12-31"

  # 翻页拉取全部结果并输出 JSON
  feishu-cli task search --completed --page-all --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return fmt.Errorf("搜索任务需要 User Access Token: %w\n提示: 请先执行 feishu-cli auth login 进行授权", err)
		}

		keyword, _ := cmd.Flags().GetString("keyword")
		creator, _ := cmd.Flags().GetString("creator")
		assignee, _ := cmd.Flags().GetString("assignee")
		follower, _ := cmd.Flags().GetString("follower")
		completedFlag, _ := cmd.Flags().GetBool("completed")
		uncompletedFlag, _ := cmd.Flags().GetBool("uncompleted")
		dueAfter, _ := cmd.Flags().GetString("due-after")
		dueBefore, _ := cmd.Flags().GetString("due-before")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		enrich, _ := cmd.Flags().GetBool("enrich")
		output, _ := cmd.Flags().GetString("output")

		if completedFlag && uncompletedFlag {
			return fmt.Errorf("--completed 与 --uncompleted 不能同时使用")
		}

		var completed *bool
		if completedFlag {
			t := true
			completed = &t
		} else if uncompletedFlag {
			f := false
			completed = &f
		}

		dueStart, err := parseDueBoundToRFC3339(dueAfter, false)
		if err != nil {
			return fmt.Errorf("解析 --due-after 失败: %w", err)
		}
		dueEnd, err := parseDueBoundToRFC3339(dueBefore, true)
		if err != nil {
			return fmt.Errorf("解析 --due-before 失败: %w", err)
		}

		creatorIDs := splitAndTrim(creator)
		assigneeIDs := splitAndTrim(assignee)
		followerIDs := splitAndTrim(follower)

		if strings.TrimSpace(keyword) == "" && len(creatorIDs) == 0 && len(assigneeIDs) == 0 &&
			len(followerIDs) == 0 && completed == nil && dueStart == "" && dueEnd == "" {
			return fmt.Errorf("请至少提供一个搜索条件（--keyword / --creator / --assignee / --follower / --completed / --uncompleted / --due-after / --due-before）")
		}

		opts := client.TaskSearchOptions{
			Query:       keyword,
			CreatorIDs:  creatorIDs,
			AssigneeIDs: assigneeIDs,
			FollowerIDs: followerIDs,
			Completed:   completed,
			DueStart:    dueStart,
			DueEnd:      dueEnd,
			PageSize:    pageSize,
			PageToken:   pageToken,
			PageAll:     pageAll,
			Enrich:      enrich,
		}

		result, err := client.SearchTasks(opts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result.Tasks) == 0 {
			fmt.Println("没有找到符合条件的任务")
			return nil
		}

		fmt.Printf("搜索到 %d 个任务:\n\n", len(result.Tasks))
		for i, task := range result.Tasks {
			status := "[ ]"
			if task.CompletedAt != "" {
				status = "[x]"
			}
			fmt.Printf("[%d] %s %s\n", i+1, status, task.Summary)
			fmt.Printf("    ID: %s\n", task.Guid)
			if task.DueTime != "" {
				fmt.Printf("    截止: %s\n", task.DueTime)
			}
			if task.CompletedAt != "" {
				fmt.Printf("    完成: %s\n", task.CompletedAt)
			}
			if task.OriginHref != "" {
				fmt.Printf("    链接: %s\n", task.OriginHref)
			}
			fmt.Println()
		}

		if result.Truncated {
			fmt.Println("已达服务端翻页上限（offset 上限 150），仍有结果未返回，请用 --keyword 或更精确的过滤条件缩小范围")
		} else if result.HasMore && result.PageToken != "" {
			fmt.Printf("还有更多结果，使用 --page-token %s 获取下一页（或加 --page-all 拉取全部）\n", result.PageToken)
		}

		return nil
	},
}

// parseDueBoundToRFC3339 解析截止时间边界，支持 RFC3339 / "2006-01-02 15:04:05" / "2006-01-02"。
// endOfDay 为 true（上界 --due-before）时，纯日期对齐到当天 23:59:59（与 vc/minutes search 惯例一致），
// 否则 --due-before 2026-12-31 会漏掉当天稍晚到期的任务。
// 统一转换为 RFC3339 字符串（本地时区）。空输入返回空字符串。
func parseDueBoundToRFC3339(input string, endOfDay bool) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t.Format(time.RFC3339), nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", input, time.Local); err == nil {
		return t.Format(time.RFC3339), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", input, time.Local); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("无法识别的时间格式 %q（支持 RFC3339 / 2006-01-02 15:04:05 / 2006-01-02）", input)
}

func init() {
	taskCmd.AddCommand(taskSearchCmd)
	taskSearchCmd.Flags().String("keyword", "", "关键词（服务端检索任务标题等）")
	taskSearchCmd.Flags().String("creator", "", "创建人 open_id，多个用逗号分隔")
	taskSearchCmd.Flags().String("assignee", "", "负责人 open_id，多个用逗号分隔")
	taskSearchCmd.Flags().String("follower", "", "关注人 open_id，多个用逗号分隔")
	taskSearchCmd.Flags().Bool("completed", false, "只搜索已完成的任务")
	taskSearchCmd.Flags().Bool("uncompleted", false, "只搜索未完成的任务")
	taskSearchCmd.Flags().String("due-after", "", "截止时间下界（RFC3339 / 2006-01-02 15:04:05 / 2006-01-02）")
	taskSearchCmd.Flags().String("due-before", "", "截止时间上界（同上）")
	taskSearchCmd.Flags().Int("page-size", 20, "每页数量（最大 30）")
	taskSearchCmd.Flags().Bool("enrich", true, "逐条拉取任务详情（--enrich=false 只返回 GUID+链接，零额外往返）")
	taskSearchCmd.Flags().String("page-token", "", "分页标记")
	taskSearchCmd.Flags().Bool("page-all", false, "自动翻页拉取全部结果")
	taskSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskSearchCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
