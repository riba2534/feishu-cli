package cmd

import (
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "任务操作命令",
	Long: `任务操作命令，用于创建、查看、更新和管理飞书任务。

子命令:
  create    创建新任务
  get       获取任务详情
  list      列出任务
  update    更新任务
  delete    删除任务
  complete  完成任务

示例:
  # 创建任务
  feishu-cli task create --summary "完成项目文档"

  # 创建带截止时间的任务
  feishu-cli task create --summary "代码审查" --due "2024-12-31 18:00:00"

  # 获取任务详情
  feishu-cli task get <task_id>

  # 列出所有任务
  feishu-cli task list

  # 列出已完成的任务
  feishu-cli task list --completed

  # 更新任务
  feishu-cli task update <task_id> --summary "新标题"

  # 完成任务
  feishu-cli task complete <task_id>

  # 删除任务
  feishu-cli task delete <task_id>`,
}

func init() {
	rootCmd.AddCommand(taskCmd)
}
