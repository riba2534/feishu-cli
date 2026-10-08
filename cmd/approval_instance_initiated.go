package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceInitiatedCmd = &cobra.Command{
	Use:   "initiated",
	Short: "查询当前用户已发起的审批实例",
	Long: `查询当前登录用户已发起的审批实例列表，对齐官方 approval.instances.initiated。

底层接口:
  GET /open-apis/approval/v4/instances/initiated

权限:
  User Token，scope: approval:instance:read

分页（重要）:
  该接口是稀疏分页：空页或不足 page_size 不代表没有更多，以 has_more 为准；用 --page-all 自动翻页
  （上限 --page-limit 页），或按提示的 --page-token 继续。count 只在首页返回，不是总数。

示例:
  feishu-cli approval instance initiated
  feishu-cli approval instance initiated --page-all
  feishu-cli approval instance initiated --definition-code <code> --output json
  feishu-cli approval instance initiated --page-size 20 --output raw-json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "approval instance initiated")
		if err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		locale, _ := cmd.Flags().GetString("locale")
		definitionCode, _ := cmd.Flags().GetString("definition-code")
		startTimestamp, _ := cmd.Flags().GetString("start-timestamp")
		endTimestamp, _ := cmd.Flags().GetString("end-timestamp")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		if err := validateApprovalWriteUserIDType(userIDType); err != nil {
			return err
		}
		if err := validateApprovalPaging(pageAll, pageLimit, output); err != nil {
			return err
		}

		opts := client.ListInitiatedApprovalInstancesOptions{
			PageSize:       pageSize,
			PageToken:      pageToken,
			Locale:         locale,
			DefinitionCode: definitionCode,
			StartTimestamp: startTimestamp,
			EndTimestamp:   endTimestamp,
			UserIDType:     userIDType,
		}

		if output == "raw-json" {
			raw, err := client.ListInitiatedApprovalInstancesRaw(opts, token)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}

		var firstCount *int
		limit := 1
		if pageAll {
			limit = pageLimit
		}
		pages, err := approvalCollectPages(pageToken, limit, func(tok string) ([]*client.ApprovalInitiatedInstance, string, bool, error) {
			o := opts
			o.PageToken = tok
			r, err := client.ListInitiatedApprovalInstances(o, token)
			if err != nil {
				return nil, "", false, err
			}
			if firstCount == nil {
				firstCount = r.Count
			}
			return r.Instances, r.PageToken, r.HasMore, nil
		})
		if err != nil {
			return err
		}
		result := &client.ApprovalInitiatedResult{
			Instances: pages.Items,
			PageToken: pages.PageToken,
			HasMore:   pages.HasMore,
			Count:     firstCount,
		}
		if result.Instances == nil {
			result.Instances = []*client.ApprovalInitiatedInstance{}
		}
		printApprovalSparseHint(len(result.Instances), result.HasMore, result.PageToken, pageAll)
		if output == "json" {
			if pageAll {
				return printJSON(map[string]any{
					"instances":  result.Instances,
					"page_token": result.PageToken,
					"has_more":   result.HasMore,
					"count":      result.Count,
					"pages":      pages.Pages,
				})
			}
			return printJSON(result)
		}

		if len(result.Instances) == 0 {
			if result.HasMore {
				fmt.Printf("当前页为空，但还有更多已发起的审批实例（稀疏分页），用 --page-token %s 或 --page-all 继续\n", result.PageToken)
				return nil
			}
			fmt.Println("没有找到已发起的审批实例")
			return nil
		}
		if pageAll {
			fmt.Printf("已发起审批实例，共 %d 条（翻了 %d 页）\n\n", len(result.Instances), pages.Pages)
		} else {
			fmt.Printf("已发起审批实例，当前页 %d 条\n\n", len(result.Instances))
		}
		for idx, item := range result.Instances {
			fmt.Printf("[%d] %s\n", idx+1, item.DefinitionName)
			fmt.Printf("    实例 Code: %s\n", item.InstanceCode)
			if item.InstanceStatus != "" {
				fmt.Printf("    实例状态: %s\n", item.InstanceStatus)
			}
			if item.InitiatorName != "" {
				fmt.Printf("    发起人: %s\n", item.InitiatorName)
			}
			if item.Link != "" {
				fmt.Printf("    链接: %s\n", item.Link)
			}
			fmt.Println()
		}
		if result.HasMore {
			fmt.Printf("还有更多实例，使用 --page-token %s 获取下一页\n", result.PageToken)
		}
		return nil
	},
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceInitiatedCmd)
	approvalInstanceInitiatedCmd.Flags().Int("page-size", 50, "每页数量")
	approvalInstanceInitiatedCmd.Flags().String("page-token", "", "分页标记")
	approvalInstanceInitiatedCmd.Flags().Bool("page-all", false, "自动翻页直到没有更多（稀疏分页：空页不代表结束）")
	approvalInstanceInitiatedCmd.Flags().Int("page-limit", 20, "--page-all 最多翻的页数（1-100）")
	approvalInstanceInitiatedCmd.Flags().String("locale", "", "语言，如 zh-CN / en-US / ja-JP")
	approvalInstanceInitiatedCmd.Flags().String("definition-code", "", "审批定义 Code，用于筛选")
	approvalInstanceInitiatedCmd.Flags().String("start-timestamp", "", "发起时间范围开始（秒级时间戳）")
	approvalInstanceInitiatedCmd.Flags().String("end-timestamp", "", "发起时间范围结束（秒级时间戳）")
	approvalInstanceInitiatedCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalInstanceInitiatedCmd.Flags().StringP("output", "o", "", "输出格式（json/raw-json）")
	approvalInstanceInitiatedCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
