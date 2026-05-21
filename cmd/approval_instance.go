package cmd

import "github.com/spf13/cobra"

var approvalInstanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "审批实例相关命令",
	Long: `审批实例相关命令，用于创建、取消和抄送审批实例。

示例:
  # 创建审批实例
  feishu-cli approval instance create --approval-code <code> --user-id ou_xxx --form-file form.json

  # 取消审批实例
  feishu-cli approval instance cancel --instance-code <ic>

  # 抄送审批实例
  feishu-cli approval instance cc --instance-code <ic> --cc-user-ids ou_a,ou_b`,
}

func init() {
	approvalCmd.AddCommand(approvalInstanceCmd)
}
