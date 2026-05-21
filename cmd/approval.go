package cmd

import "github.com/spf13/cobra"

var approvalCmd = &cobra.Command{
	Use:   "approval",
	Short: "审批相关命令",
	Long: `审批相关命令，用于查看审批定义、实例和任务，并支持发起/撤回/抄送/审批等写操作。

当前已提供：
  - 审批定义查询（approval get）
  - 审批实例详情（approval instance get）
  - 审批任务查询（approval task query）
  - 发起审批实例（approval instance create）
  - 取消审批实例（approval instance cancel）
  - 抄送审批实例（approval instance cc）
  - 通过审批任务（approval task approve）
  - 拒绝审批任务（approval task reject）
  - 转交审批任务（approval task transfer）

示例:
  # 查看审批定义详情
  feishu-cli approval get <approval_code>

  # 查看当前登录用户的待我审批任务
  feishu-cli approval task query --topic todo

  # 发起审批实例
  feishu-cli approval instance create --approval-code <code> --user-id ou_xxx --form-file form.json

  # 通过审批任务
  feishu-cli approval task approve --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx

  # 官方资源名别名也可用
  feishu-cli approval tasks transfer --instance-code <ic> --task-id <task> --transfer-user-id ou_xxx`,
}

func init() {
	rootCmd.AddCommand(approvalCmd)
}
