package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteTaskCmd = &cobra.Command{
	Use:   "delete <task_id>",
	Short: "删除任务",
	Long: `删除指定的任务。

参数:
  task_id       任务 ID（必填）
  --output, -o  输出格式（json）

示例:
  # 删除任务
  feishu-cli task delete e297ddff-06ca-4166-b917-4ce57cd3a7a0

  # JSON 格式输出
  feishu-cli task delete e297ddff-06ca-4166-b917-4ce57cd3a7a0 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		taskGuid := args[0]

		err := client.DeleteTask(taskGuid)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"success": true,
				"task_id": taskGuid,
				"message": "任务删除成功",
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("任务删除成功！\n")
			fmt.Printf("  任务 ID: %s\n", taskGuid)
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(deleteTaskCmd)
	deleteTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
