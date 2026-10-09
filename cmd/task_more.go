package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// parseTaskGUIDArg 接受任务 GUID 或任务 applink（guid= 参数）
func parseTaskGUIDArg(raw string) (string, error) {
	guid, err := client.ParseTaskGUID(raw)
	if err != nil {
		return "", clierr.Usage(err)
	}
	return guid, nil
}

const taskPageAllMax = 40

var taskSectionCmd = &cobra.Command{
	Use:   "section",
	Short: "任务分组（我的任务 / 清单内的分组）",
	Long: `任务分组相关命令。

子命令:
  list   列出"我的任务"或某个清单下的分组
  tasks  列出某个分组内的任务

示例:
  feishu-cli task section list --resource-type my_tasks
  feishu-cli task section list --resource-type tasklist --resource-id <tasklist_guid>
  feishu-cli task section tasks <section_guid> --uncompleted`,
}

var taskSectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出任务分组",
	Long: `列出"我的任务"或某个清单下的分组（GET /task/v2/sections）。

参数:
  --resource-type   my_tasks（我的任务，需 User Token）| tasklist（默认 my_tasks）
  --resource-id     清单 GUID（--resource-type tasklist 时必填）
  --page-all        自动翻完所有页（上限 40 页）

示例:
  feishu-cli task section list
  feishu-cli task section list --resource-type tasklist --resource-id <tasklist_guid> -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		resType, _ := cmd.Flags().GetString("resource-type")
		resID, _ := cmd.Flags().GetString("resource-id")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		output, _ := cmd.Flags().GetString("output")
		if err := validateEnum(resType, "--resource-type", []string{"my_tasks", "tasklist"}); err != nil {
			return err
		}
		if resType == "tasklist" && strings.TrimSpace(resID) == "" {
			return clierr.Usagef("--resource-type tasklist 需要 --resource-id <tasklist_guid>")
		}
		if resType == "my_tasks" && resID != "" {
			return clierr.Usagef("--resource-type my_tasks 不需要 --resource-id")
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		var all []*client.TaskSection
		var next string
		var more bool
		for page := 0; ; page++ {
			items, n, m, err := client.ListTaskSections(resType, resID, pageToken, 50, token)
			if err != nil {
				return err
			}
			all = append(all, items...)
			next, more = n, m
			if !pageAll || !m || n == "" || n == pageToken || page+1 >= taskPageAllMax {
				break
			}
			pageToken = n
		}
		if more {
			fmt.Fprintf(cmdErrOut(), "提示：还有更多分组，用 --page-token %s 或 --page-all 继续\n", next)
		}
		if output == "json" {
			if all == nil {
				all = []*client.TaskSection{}
			}
			return printJSON(map[string]any{"sections": all, "page_token": next, "has_more": more})
		}
		if len(all) == 0 {
			fmt.Println("没有分组")
			return nil
		}
		for i, s := range all {
			def := ""
			if s.IsDefault {
				def = "（默认分组）"
			}
			fmt.Printf("[%d] %s%s\n    GUID: %s\n", i+1, s.Name, def, s.Guid)
		}
		return nil
	},
}

var taskSectionTasksCmd = &cobra.Command{
	Use:   "tasks <section_guid>",
	Short: "列出分组内的任务",
	Long: `列出某个分组内的任务（GET /task/v2/sections/{section_guid}/tasks）。

参数:
  section_guid     分组 GUID（task section list 获取）
  --completed      只看已完成；--uncompleted 只看未完成（互斥，默认全部）
  --page-all       自动翻完所有页（上限 40 页）

示例:
  feishu-cli task section tasks <section_guid> --uncompleted`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		completed, err := completedFilterFlags(cmd)
		if err != nil {
			return err
		}
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		output, _ := cmd.Flags().GetString("output")
		if err := config.Validate(); err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		res, err := collectTaskPages(pageToken, pageAll, func(tok string) (*client.TaskPage, error) {
			return client.ListSectionTasks(args[0], completed, tok, 50, token)
		})
		if err != nil {
			return err
		}
		return printTaskPage(res, output, "分组任务")
	},
}

var taskRelatedCmd = &cobra.Command{
	Use:   "related",
	Short: "列出与我相关的任务",
	Long: `列出与当前用户相关的任务（我负责、我关注、我创建等；GET /task/v2/task_v2/list_related_task）。
必需 User Token。

参数:
  --include-completed   是否包含已完成任务（默认 true）
  --page-token          起始游标（注意：是任务 updated_at 的**微秒**时间戳，不是任务 ID；
                        想从某个时间点开始查，可把该时间换算成微秒传入）
  --page-all            自动翻页（上限 --page-limit 页，最大 40）
  --page-limit          最多翻的页数（默认 20）

示例:
  feishu-cli task related --include-completed=false
  feishu-cli task related --page-all -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		include, _ := cmd.Flags().GetBool("include-completed")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		output, _ := cmd.Flags().GetString("output")
		if pageLimit <= 0 || pageLimit > taskPageAllMax {
			return clierr.Usagef("--page-limit 范围 1-%d", taskPageAllMax)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "task related")
		if err != nil {
			return err
		}
		res, err := collectTaskPagesLimit(pageToken, pageAll, pageLimit, func(tok string) (*client.TaskPage, error) {
			return client.ListRelatedTasks(include, tok, 50, token)
		})
		if err != nil {
			return err
		}
		return printTaskPage(res, output, "相关任务")
	},
}

var taskSetAncestorCmd = &cobra.Command{
	Use:   "set-ancestor <task_guid>",
	Short: "设置或解除父任务",
	Long: `把任务设为另一个任务的子任务，或解除父任务（POST /task/v2/tasks/{guid}/set_ancestor_task）。

参数:
  task_guid        要修改的任务 GUID（或任务 applink）
  --ancestor-id    父任务 GUID（或 applink）；不传则解除父任务，使其成为独立任务
  --as             身份：auto（默认）| user | bot

示例:
  feishu-cli task set-ancestor <child_guid> --ancestor-id <parent_guid>
  feishu-cli task set-ancestor <child_guid>   # 解除父任务`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		child, err := parseTaskGUIDArg(args[0])
		if err != nil {
			return err
		}
		ancestor, _ := cmd.Flags().GetString("ancestor-id")
		if strings.TrimSpace(ancestor) != "" {
			if ancestor, err = parseTaskGUIDArg(ancestor); err != nil {
				return err
			}
			if ancestor == child {
				return clierr.Usagef("父任务不能是任务自身")
			}
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if err := client.SetTaskAncestor(child, ancestor, token); err != nil {
			return err
		}
		if ancestor == "" {
			fmt.Printf("已解除任务 %s 的父任务\n", child)
		} else {
			fmt.Printf("已把任务 %s 设为 %s 的子任务\n", child, ancestor)
		}
		return nil
	},
}

var tasklistSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "搜索任务清单",
	Long: `按关键词或创建人搜索任务清单（POST /task/v2/tasklists/search），命中后补拉清单详情。

参数:
  --query        关键词（与 --creator 至少一个）
  --creator      创建人 open_id，逗号分隔
  --page-token   分页标记
  --page-size    每页数量

示例:
  feishu-cli tasklist search --query "Sprint"
  feishu-cli tasklist search --creator ou_xxx -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		creator, _ := cmd.Flags().GetString("creator")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")
		if strings.TrimSpace(query) == "" && strings.TrimSpace(creator) == "" {
			return clierr.Usagef("至少提供 --query 或 --creator 之一")
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		items, next, more, err := client.SearchTasklists(client.TasklistSearchOptions{
			Query: query, CreatorIDs: splitAndTrim(creator), PageToken: pageToken, PageSize: pageSize,
		}, token)
		if err != nil {
			return err
		}
		if more {
			fmt.Fprintf(cmdErrOut(), "提示：还有更多结果，用 --page-token %s 继续\n", next)
		}
		if output == "json" {
			return printJSON(map[string]any{"tasklists": items, "page_token": next, "has_more": more})
		}
		if len(items) == 0 {
			fmt.Println("没有匹配的任务清单")
			return nil
		}
		for i, tl := range items {
			fmt.Printf("[%d] %s\n    GUID: %s\n", i+1, tl.Name, tl.Guid)
		}
		return nil
	},
}

// completedFilterFlags 解析互斥的 --completed / --uncompleted
func completedFilterFlags(cmd *cobra.Command) (*bool, error) {
	c, _ := cmd.Flags().GetBool("completed")
	u, _ := cmd.Flags().GetBool("uncompleted")
	if c && u {
		return nil, clierr.Usagef("--completed 与 --uncompleted 不能同时使用")
	}
	if c {
		t := true
		return &t, nil
	}
	if u {
		f := false
		return &f, nil
	}
	return nil, nil
}

func collectTaskPages(start string, pageAll bool, fetch func(string) (*client.TaskPage, error)) (*client.TaskPage, error) {
	return collectTaskPagesLimit(start, pageAll, taskPageAllMax, fetch)
}

// collectTaskPagesLimit 按 page_token 翻页（游标重复/达到上限即停），汇总为一页结果
func collectTaskPagesLimit(start string, pageAll bool, limit int, fetch func(string) (*client.TaskPage, error)) (*client.TaskPage, error) {
	out := &client.TaskPage{Tasks: []*client.TaskInfo{}}
	tok := start
	for page := 0; ; page++ {
		p, err := fetch(tok)
		if err != nil {
			return nil, err
		}
		out.Tasks = append(out.Tasks, p.Tasks...)
		out.PageToken, out.HasMore = p.PageToken, p.HasMore
		if !pageAll || !p.HasMore || p.PageToken == "" || p.PageToken == tok || page+1 >= limit {
			break
		}
		tok = p.PageToken
	}
	if out.HasMore {
		fmt.Fprintf(cmdErrOut(), "提示：还有更多任务，用 --page-token %s 或 --page-all 继续\n", out.PageToken)
	}
	return out, nil
}

func printTaskPage(p *client.TaskPage, output, label string) error {
	if output == "json" {
		return printJSON(p)
	}
	if len(p.Tasks) == 0 {
		fmt.Printf("没有%s\n", label)
		return nil
	}
	fmt.Printf("%s（共 %d 个）:\n\n", label, len(p.Tasks))
	for i, t := range p.Tasks {
		status := "[ ]"
		if t.CompletedAt != "" {
			status = "[x]"
		}
		fmt.Printf("[%d] %s %s\n    ID: %s\n", i+1, status, t.Summary, t.Guid)
		if t.DueTime != "" {
			fmt.Printf("    截止: %s\n", t.DueTime)
		}
	}
	return nil
}

func init() {
	taskCmd.AddCommand(taskSectionCmd)
	taskSectionCmd.AddCommand(taskSectionListCmd)
	taskSectionListCmd.Flags().String("resource-type", "my_tasks", "分组所属资源：my_tasks | tasklist")
	taskSectionListCmd.Flags().String("resource-id", "", "清单 GUID（resource-type=tasklist 时必填）")
	taskSectionListCmd.Flags().String("page-token", "", "分页标记")
	taskSectionListCmd.Flags().Bool("page-all", false, "自动翻完所有页（上限 40 页）")
	taskSectionListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskSectionListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	taskSectionCmd.AddCommand(taskSectionTasksCmd)
	taskSectionTasksCmd.Flags().Bool("completed", false, "只看已完成")
	taskSectionTasksCmd.Flags().Bool("uncompleted", false, "只看未完成")
	taskSectionTasksCmd.Flags().String("page-token", "", "分页标记")
	taskSectionTasksCmd.Flags().Bool("page-all", false, "自动翻完所有页（上限 40 页）")
	taskSectionTasksCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskSectionTasksCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	taskCmd.AddCommand(taskRelatedCmd)
	taskRelatedCmd.Flags().Bool("include-completed", true, "是否包含已完成任务")
	taskRelatedCmd.Flags().String("page-token", "", "起始游标（updated_at 微秒时间戳）")
	taskRelatedCmd.Flags().Bool("page-all", false, "自动翻页（上限 --page-limit 页）")
	taskRelatedCmd.Flags().Int("page-limit", 20, "最多翻的页数（1-40）")
	taskRelatedCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskRelatedCmd.Flags().String("user-access-token", "", "User Access Token（必需）")

	taskCmd.AddCommand(taskSetAncestorCmd)
	taskSetAncestorCmd.Flags().String("ancestor-id", "", "父任务 GUID（不传则解除父任务）")
	taskSetAncestorCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(taskSetAncestorCmd)

	tasklistCmd.AddCommand(tasklistSearchCmd)
	tasklistSearchCmd.Flags().String("query", "", "关键词")
	tasklistSearchCmd.Flags().String("creator", "", "创建人 open_id，逗号分隔")
	tasklistSearchCmd.Flags().String("page-token", "", "分页标记")
	tasklistSearchCmd.Flags().Int("page-size", 0, "每页数量")
	tasklistSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistSearchCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
