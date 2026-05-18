package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var approvalTaskRejectCmd = &cobra.Command{
	Use:   "reject",
	Short: "拒绝审批任务",
	Long: `拒绝指定的审批任务。需要 User Token + scope approval:approval。

参数:
  --approval-code    审批定义 code（必填）
  --instance-code    审批实例 code（必填）
  --task-id          审批任务 ID（必填）
  --user-id          操作人用户 ID（必填）
  --comment          审批意见（可选，建议填写拒绝原因）
  --user-id-type     open_id（默认）/user_id/union_id

示例:
  feishu-cli approval task reject \
    --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx \
    --comment "金额超预算"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		opts, err := readApprovalTaskActionFlags(cmd)
		if err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)
		if err := client.RejectApprovalTask(opts, token); err != nil {
			return err
		}

		fmt.Printf("已拒绝审批任务: %s\n", opts.TaskID)
		return nil
	},
}

func init() {
	approvalTaskCmd.AddCommand(approvalTaskRejectCmd)
	registerApprovalTaskActionFlags(approvalTaskRejectCmd)
}
