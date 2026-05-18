package cmd

import "github.com/spf13/cobra"

var approvalTaskCmd = &cobra.Command{
	Use:   "task",
	Short: "审批任务相关命令",
	Long: `审批任务相关命令，用于查询、通过或拒绝审批任务。

当前已提供：
  - 审批任务查询（approval task query）
  - 通过审批任务（approval task approve）
  - 拒绝审批任务（approval task reject）

示例:
  # 查询待我审批的任务
  feishu-cli approval task query --topic todo

  # 查询我发起的审批
  feishu-cli approval task query --topic started --output json

  # 通过审批任务
  feishu-cli approval task approve --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx

  # 拒绝审批任务
  feishu-cli approval task reject --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx --comment "金额超预算"`,
}

func init() {
	approvalCmd.AddCommand(approvalTaskCmd)
}
