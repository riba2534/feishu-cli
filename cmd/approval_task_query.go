package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

const (
	approvalTopicTodo     = "1"
	approvalTopicDone     = "2"
	approvalTopicStarted  = "3"
	approvalTopicCCUnread = "17"
	approvalTopicCCRead   = "18"
)

var approvalTaskQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "查询审批任务列表",
	Long: `查询当前 auth 登录用户的审批任务列表，可用于查看待办审批、已办审批、已发起审批和抄送通知。

参数:
  --topic        任务主题，可选：todo、done、started、cc-unread、cc-read
  --output, -o   输出格式，可选：json、raw-json

示例:
  # 查询当前登录用户的待我审批（User Token 必需）
  feishu-cli approval task query --topic todo

  # 查询我已审批的任务
  feishu-cli approval task query --topic done

  # 显式使用 User Token
  feishu-cli approval task query --topic todo --user-access-token u-xxx

  # JSON 输出
  feishu-cli approval task query --topic started --output json

  # 原始 API 响应
  feishu-cli approval task query --topic started --output raw-json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		topic, _ := cmd.Flags().GetString("topic")
		topicValue, err := normalizeApprovalTaskTopic(topic)
		if err != nil {
			return err
		}

		userID, err := resolveCurrentAuthedUserID(cmd, "open_id")
		if err != nil {
			return fmt.Errorf("无法从当前登录态自动获取用户身份，请先执行 feishu-cli auth login: %w", err)
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		token, err := requireUserToken(cmd, "approval task query")
		if err != nil {
			return err
		}
		queryOpts := client.ApprovalTaskQueryOptions{
			PageSize:   pageSize,
			PageToken:  pageToken,
			UserID:     userID,
			Topic:      topicValue,
			UserIDType: "open_id",
		}

		if output == "raw-json" {
			raw, err := client.QueryApprovalTasksRaw(queryOpts, token)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}

		result, err := client.QueryApprovalTasks(queryOpts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result.Tasks) == 0 {
			fmt.Printf("没有找到审批任务（topic: %s）\n", approvalTaskTopicLabel(topicValue))
			return nil
		}

		if result.Count != nil {
			fmt.Printf("审批任务（%s），总数约 %d\n\n", approvalTaskTopicLabel(topicValue), result.Count.Total)
		} else {
			fmt.Printf("审批任务（%s），当前页 %d 条\n\n", approvalTaskTopicLabel(topicValue), len(result.Tasks))
		}

		for idx, task := range result.Tasks {
			fmt.Printf("[%d] %s\n", idx+1, task.Title)
			fmt.Printf("    任务 ID: %s\n", task.TaskID)
			if task.DefinitionName != "" {
				fmt.Printf("    审批流: %s\n", task.DefinitionName)
			}
			if len(task.InitiatorNames) > 0 {
				fmt.Printf("    发起人: %s\n", strings.Join(task.InitiatorNames, ", "))
			}
			if task.Status != "" {
				fmt.Printf("    任务状态: %s\n", task.Status)
			}
			if task.ProcessStatus != "" {
				fmt.Printf("    流程状态: %s\n", task.ProcessStatus)
			}
			if task.PCURL != "" {
				fmt.Printf("    PC 链接: %s\n", task.PCURL)
			} else if task.MobileURL != "" {
				fmt.Printf("    移动端链接: %s\n", task.MobileURL)
			}
			fmt.Println()
		}

		if result.HasMore {
			fmt.Printf("还有更多任务，使用 --page-token %s 获取下一页\n", result.PageToken)
		}

		return nil
	},
}

func normalizeApprovalTaskTopic(topic string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "1", "todo":
		return approvalTopicTodo, nil
	case "2", "done":
		return approvalTopicDone, nil
	case "3", "started", "initiated":
		return approvalTopicStarted, nil
	case "17", "cc-unread", "unread-cc":
		return approvalTopicCCUnread, nil
	case "18", "cc-read", "read-cc":
		return approvalTopicCCRead, nil
	default:
		return "", fmt.Errorf("不支持的 topic: %s（可选值: todo, done, started, cc-unread, cc-read）", topic)
	}
}

func approvalTaskTopicLabel(topic string) string {
	switch topic {
	case approvalTopicTodo:
		return "待我审批"
	case approvalTopicDone:
		return "我已审批"
	case approvalTopicStarted:
		return "我发起的审批"
	case approvalTopicCCUnread:
		return "未读抄送"
	case approvalTopicCCRead:
		return "已读抄送"
	default:
		return topic
	}
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskQueryCmd)

	approvalTaskQueryCmd.Flags().String("topic", "", "任务主题：todo、done、started、cc-unread、cc-read")
	approvalTaskQueryCmd.Flags().Int("page-size", 50, "每页数量")
	approvalTaskQueryCmd.Flags().String("page-token", "", "分页标记")
	approvalTaskQueryCmd.Flags().StringP("output", "o", "", "输出格式（json/raw-json）")
	approvalTaskQueryCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(approvalTaskQueryCmd, "topic")
}
