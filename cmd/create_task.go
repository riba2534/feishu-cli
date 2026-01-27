package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createTaskCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新任务",
	Long: `创建新的飞书任务。

参数:
  --summary, -s       任务标题（必填）
  --description, -d   任务描述
  --due               截止时间（格式: 2006-01-02 15:04:05 或 2006-01-02）
  --origin-href       任务来源链接
  --origin-platform   任务来源平台名称（默认: feishu-cli）
  --output, -o        输出格式（json）

示例:
  # 创建简单任务
  feishu-cli task create --summary "完成项目文档"

  # 创建带描述的任务
  feishu-cli task create --summary "代码审查" --description "审查 PR #123"

  # 创建带截止时间的任务
  feishu-cli task create --summary "提交报告" --due "2024-12-31 18:00:00"

  # 创建带来源链接的任务
  feishu-cli task create --summary "处理 Issue" --origin-href "https://github.com/example/repo/issues/1"

  # JSON 格式输出
  feishu-cli task create --summary "测试任务" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		summary, _ := cmd.Flags().GetString("summary")
		description, _ := cmd.Flags().GetString("description")
		dueStr, _ := cmd.Flags().GetString("due")
		originHref, _ := cmd.Flags().GetString("origin-href")
		originPlatform, _ := cmd.Flags().GetString("origin-platform")

		opts := client.CreateTaskOptions{
			Summary:        summary,
			Description:    description,
			OriginHref:     originHref,
			OriginPlatform: originPlatform,
		}

		// Parse due time
		if dueStr != "" {
			dueTime, err := parseTime(dueStr)
			if err != nil {
				return fmt.Errorf("解析截止时间失败: %w", err)
			}
			opts.DueTimestamp = dueTime.UnixMilli()
		}

		task, err := client.CreateTask(opts)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(task, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("任务创建成功！\n")
			fmt.Printf("  任务 ID: %s\n", task.Guid)
			fmt.Printf("  标题: %s\n", task.Summary)
			if task.Description != "" {
				fmt.Printf("  描述: %s\n", task.Description)
			}
			if task.DueTime != "" {
				fmt.Printf("  截止时间: %s\n", task.DueTime)
			}
			if task.OriginHref != "" {
				fmt.Printf("  来源链接: %s\n", task.OriginHref)
			}
		}

		return nil
	},
}

// parseTime parses a time string in various formats
func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("无法识别的时间格式: %s", s)
}

func init() {
	taskCmd.AddCommand(createTaskCmd)
	createTaskCmd.Flags().StringP("summary", "s", "", "任务标题（必填）")
	createTaskCmd.Flags().StringP("description", "d", "", "任务描述")
	createTaskCmd.Flags().String("due", "", "截止时间（格式: 2006-01-02 15:04:05）")
	createTaskCmd.Flags().String("origin-href", "", "任务来源链接")
	createTaskCmd.Flags().String("origin-platform", "", "任务来源平台名称")
	createTaskCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(createTaskCmd, "summary")
}
