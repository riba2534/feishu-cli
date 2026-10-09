package cmd

import "github.com/spf13/cobra"

var approvalTaskCmd = &cobra.Command{
	Use:     "task",
	Aliases: []string{"tasks"},
	Short:   "审批任务相关命令",
	Long: `审批任务相关命令，用于查询、通过、拒绝、转交、退回、加签或催办审批任务。

当前已提供：
  - 审批任务查询（approval task query）
  - 通过审批任务（approval task approve）
  - 拒绝审批任务（approval task reject）
  - 转交审批任务（approval task transfer）
  - 退回审批任务（approval task rollback）
  - 审批任务加签（approval task add-sign）
  - 催办审批任务（approval task remind）

task query 的 --topic 仅支持 todo / done / cc-unread / cc-read；
"我发起的审批"请用 approval instance initiated（tasks 接口的 started 已被官方下线）。

示例:
  # 查询待我审批的任务
  feishu-cli approval task query --topic todo

  # 查询我已处理的任务
  feishu-cli approval task query --topic done --output json

  # 查询我发起的审批（实例维度）
  feishu-cli approval instance initiated

  # 通过审批任务
  feishu-cli approval task approve --instance-code <ic> --task-id <task>

  # 拒绝审批任务
  feishu-cli approval task reject --instance-code <ic> --task-id <task> --comment "金额超预算"

  # 转交审批任务
  feishu-cli approval task transfer --instance-code <ic> --task-id <task> --transfer-user-id ou_xxx`,
}

func init() {
	approvalCmd.AddCommand(approvalTaskCmd)
}
