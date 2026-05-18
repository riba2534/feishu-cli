package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalInstanceCcCmd = &cobra.Command{
	Use:   "cc",
	Short: "抄送审批实例",
	Long: `把某条审批实例抄送给一个或多个用户。需要 User Token + scope approval:approval。

参数:
  --approval-code    审批定义 code（必填）
  --instance-code    审批实例 code（必填）
  --user-id          发起抄送的用户 ID（必填）
  --cc-user-ids      被抄送用户 ID 列表，逗号分隔（必填，至少一个）
  --comment          抄送备注（可选）
  --user-id-type     open_id（默认）/user_id/union_id

示例:
  feishu-cli approval instance cc \
    --approval-code <code> --instance-code <ic> \
    --user-id ou_xxx --cc-user-ids ou_a,ou_b --comment "请知悉"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		approvalCode, _ := cmd.Flags().GetString("approval-code")
		if err := validateApprovalCode(approvalCode); err != nil {
			return err
		}

		instanceCode, _ := cmd.Flags().GetString("instance-code")
		if strings.TrimSpace(instanceCode) == "" {
			return fmt.Errorf("--instance-code 不能为空")
		}

		userID, _ := cmd.Flags().GetString("user-id")
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("--user-id 不能为空")
		}

		ccRaw, _ := cmd.Flags().GetString("cc-user-ids")
		ccUserIDs := parseCommaSeparatedIDs(ccRaw)
		if len(ccUserIDs) == 0 {
			return fmt.Errorf("--cc-user-ids 至少需要一个用户 ID")
		}

		comment, _ := cmd.Flags().GetString("comment")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		token := resolveOptionalUserTokenWithFallback(cmd)

		err := client.CCApprovalInstance(client.CCApprovalInstanceOptions{
			ApprovalCode: approvalCode,
			InstanceCode: instanceCode,
			UserID:       userID,
			CCUserIDs:    ccUserIDs,
			Comment:      comment,
			UserIDType:   userIDType,
		}, token)
		if err != nil {
			return err
		}

		fmt.Printf("已抄送审批实例 %s 给 %d 位用户\n", instanceCode, len(ccUserIDs))
		return nil
	},
}

// parseCommaSeparatedIDs 把逗号分隔的字符串切成去空格、去空值、去重的切片。
// 保留首次出现顺序，便于用户传入 ou_a,ou_b,ou_a 时只抄送一次。
func parseCommaSeparatedIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func init() {
	approvalInstanceCmd.AddCommand(approvalInstanceCcCmd)

	approvalInstanceCcCmd.Flags().String("approval-code", "", "审批定义 code（必填）")
	approvalInstanceCcCmd.Flags().String("instance-code", "", "审批实例 code（必填）")
	approvalInstanceCcCmd.Flags().String("user-id", "", "执行抄送的用户 ID（必填）")
	approvalInstanceCcCmd.Flags().String("cc-user-ids", "", "被抄送用户 ID 列表，逗号分隔（必填）")
	approvalInstanceCcCmd.Flags().String("comment", "", "抄送备注（可选）")
	approvalInstanceCcCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id/user_id/union_id")
	approvalInstanceCcCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(approvalInstanceCcCmd, "approval-code", "instance-code", "user-id", "cc-user-ids")
}
