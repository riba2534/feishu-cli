package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// okr cycle detail —— 拉取某周期下全部目标 + 关键结果
var okrCycleDetailCmd = &cobra.Command{
	Use:   "detail <cycle_id>",
	Short: "获取 OKR 周期详情（目标 + 关键结果）",
	Long: `获取指定 OKR 周期下的全部目标（Objective）及其关键结果（Key Result）。

参数:
  <cycle_id>      周期 ID（okr cycle list 可查）
  --output, -o    输出格式：json

权限要求: okr:okr:readonly（bot 身份需应用后台开通；user 身份需登录时带该 scope）

示例:
  feishu-cli okr cycle detail 7123456789012345678
  feishu-cli okr cycle detail 7123456789012345678 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		objectives, err := client.GetOKRCycleDetail(args[0], token)
		if err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(objectives)
		}
		fmt.Printf("周期 %s 下共 %d 个目标:\n", args[0], len(objectives))
		for i, obj := range objectives {
			fmt.Printf("\n[%d] %s\n", i+1, obj.Content)
			fmt.Printf("    目标 ID: %s\n", obj.ID)
			if obj.Owner.UserID != "" {
				fmt.Printf("    负责人: %s\n", obj.Owner.UserID)
			}
			for j, kr := range obj.KeyResults {
				fmt.Printf("    KR%d: %s（ID: %s）\n", j+1, kr.Content, kr.ID)
			}
		}
		return nil
	},
}

// okr progress get —— 查询单条进展记录
var okrProgressGetCmd = &cobra.Command{
	Use:   "get <progress_id>",
	Short: "获取一条 OKR 进展记录详情",
	Long: `获取一条 OKR 进展记录详情。

参数:
  <progress_id>    进展记录 ID（okr progress list 可查）
  --user-id-type   用户 ID 类型：open_id（默认）/ union_id / user_id
  --output, -o     输出格式：json

权限要求: okr:okr:readonly 或 okr:okr.progress:readonly

示例:
  feishu-cli okr progress get 7123456789012345678 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		userIDType := flagString(cmd, "user-id-type")
		if err := validateUserIDType(userIDType); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		progress, err := client.GetOKRProgress(args[0], userIDType, token)
		if err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(progress)
		}
		printOKRProgressSummary(progress)
		return nil
	},
}

// okr progress update —— 更新进展记录
var okrProgressUpdateCmd = &cobra.Command{
	Use:   "update <progress_id>",
	Short: "更新一条 OKR 进展记录",
	Long: `更新一条 OKR 进展记录的内容和进度。

参数:
  <progress_id>       进展记录 ID
  --content           纯文本内容（自动包装为 ContentBlock 富文本）
  --content-json      原始 ContentBlock JSON（与 --content 二选一）
  --progress-percent  进度百分比（数字，配合 --progress-status 使用）
  --progress-status   进度状态：normal / risky / overdue
  --user-id-type      用户 ID 类型：open_id（默认）/ union_id / user_id
  --output, -o        输出格式：json

权限要求: okr:okr 或 okr:okr.progress:writeonly

示例:
  feishu-cli okr progress update 7xxx --content "更新后的进展说明"
  feishu-cli okr progress update 7xxx --content "已完成 9/10" --progress-percent 90 --progress-status normal`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		userIDType := flagString(cmd, "user-id-type")
		if err := validateUserIDType(userIDType); err != nil {
			return err
		}
		contentJSON, err := buildOKRProgressContentJSON(flagString(cmd, "content"), flagString(cmd, "content-json"))
		if err != nil {
			return err
		}
		opts := client.UpdateOKRProgressOptions{
			ProgressID:  args[0],
			ContentJSON: contentJSON,
			UserIDType:  userIDType,
		}
		rate, err := parseOKRProgressRate(flagString(cmd, "progress-percent"), flagString(cmd, "progress-status"))
		if err != nil {
			return err
		}
		opts.ProgressRate = rate
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		progress, err := client.UpdateOKRProgress(opts, token)
		if err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(progress)
		}
		fmt.Println("已更新 OKR 进展记录")
		printOKRProgressSummary(progress)
		return nil
	},
}

// okr progress delete —— 删除进展记录
var okrProgressDeleteCmd = &cobra.Command{
	Use:   "delete <progress_id>",
	Short: "删除一条 OKR 进展记录",
	Long: `删除一条 OKR 进展记录（不可恢复）。

参数:
  <progress_id>   进展记录 ID
  --yes           跳过确认直接删除

权限要求: okr:okr 或 okr:okr.progress:delete

示例:
  feishu-cli okr progress delete 7123456789012345678 --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		if yes, _ := cmd.Flags().GetBool("yes"); !yes {
			if !confirmAction(fmt.Sprintf("确认删除进展记录 %s？此操作不可恢复", args[0])) {
				fmt.Println("已取消")
				return nil
			}
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if err := client.DeleteOKRProgress(args[0], token); err != nil {
			return err
		}
		fmt.Printf("已删除进展记录 %s\n", args[0])
		return nil
	},
}

// okr upload-image —— 上传进展图片
var okrUploadImageCmd = &cobra.Command{
	Use:   "upload-image",
	Short: "上传 OKR 进展记录图片",
	Long: `上传图片素材，供 OKR 进展记录的 ContentBlock 富文本引用（imageList）。

参数（--objective-id / --key-result-id 二选一）:
  --file            本地图片路径（必填）
  --objective-id    目标 ID
  --key-result-id   关键结果 ID
  --output, -o      输出格式：json

权限要求: okr:okr 或 okr:okr.progress.file:upload

示例:
  feishu-cli okr upload-image --file chart.png --objective-id 7xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		filePath := flagString(cmd, "file")
		if filePath == "" {
			return fmt.Errorf("请通过 --file 指定图片路径")
		}
		targetID, targetType, err := pickOKRTarget(flagString(cmd, "objective-id"), flagString(cmd, "key-result-id"))
		if err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		result, err := client.UploadOKRImage(filePath, targetID, targetType, token)
		if err != nil {
			return err
		}
		if flagString(cmd, "output") == "json" {
			return printJSON(result)
		}
		fmt.Println("图片上传成功")
		fmt.Printf("  file_token: %s\n", result.FileToken)
		if result.URL != "" {
			fmt.Printf("  URL: %s\n", result.URL)
		}
		fmt.Printf("  文件: %s（%d 字节）\n", result.FileName, result.Size)
		return nil
	},
}

// printOKRProgressSummary 打印进展记录的文本摘要。
func printOKRProgressSummary(progress *client.OKRProgress) {
	if progress == nil {
		return
	}
	fmt.Printf("  进展 ID: %s\n", progress.ProgressID)
	if progress.Content != "" {
		fmt.Printf("  内容: %s\n", progress.Content)
	}
	if progress.ModifyTime != "" {
		fmt.Printf("  修改时间: %s\n", progress.ModifyTime)
	}
	if progress.ProgressRate != nil && progress.ProgressRate.Percent != nil {
		fmt.Printf("  进度: %.1f%%\n", *progress.ProgressRate.Percent)
	}
	if progress.ProgressRate != nil && progress.ProgressRate.Status != "" {
		fmt.Printf("  状态: %s\n", progress.ProgressRate.Status)
	}
}

// parseOKRProgressRate 解析 --progress-percent / --progress-status 组合。
// percent 为空时 status 必须也为空；返回 nil 表示不更新进度。
func parseOKRProgressRate(percentStr, statusStr string) (*client.OKRProgressRateInput, error) {
	if percentStr == "" {
		if statusStr != "" {
			return nil, fmt.Errorf("--progress-status 必须配合 --progress-percent 一起使用")
		}
		return nil, nil
	}
	percent, err := strconv.ParseFloat(strings.TrimSpace(percentStr), 64)
	if err != nil {
		return nil, fmt.Errorf("--progress-percent 必须是数字: %w", err)
	}
	rate := &client.OKRProgressRateInput{Percent: percent}
	if statusStr != "" {
		status, ok := client.ParseOKRProgressStatus(statusStr)
		if !ok {
			return nil, fmt.Errorf("--progress-status 必须为 normal / risky / overdue")
		}
		rate.Status = &status
	}
	return rate, nil
}

func init() {
	okrCycleCmd.AddCommand(okrCycleDetailCmd)
	okrCycleDetailCmd.Flags().StringP("output", "o", "", "输出格式：json")

	okrProgressCmd.AddCommand(okrProgressGetCmd, okrProgressUpdateCmd, okrProgressDeleteCmd)
	okrProgressGetCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrProgressGetCmd.Flags().StringP("output", "o", "", "输出格式：json")

	okrProgressUpdateCmd.Flags().String("content", "", "纯文本内容")
	okrProgressUpdateCmd.Flags().String("content-json", "", "原始 ContentBlock JSON")
	okrProgressUpdateCmd.Flags().String("progress-percent", "", "进度百分比")
	okrProgressUpdateCmd.Flags().String("progress-status", "", "进度状态：normal / risky / overdue")
	okrProgressUpdateCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrProgressUpdateCmd.Flags().StringP("output", "o", "", "输出格式：json")

	okrProgressDeleteCmd.Flags().Bool("yes", false, "跳过确认直接删除")

	okrCmd.AddCommand(okrUploadImageCmd)
	okrUploadImageCmd.Flags().String("file", "", "本地图片路径")
	okrUploadImageCmd.Flags().String("objective-id", "", "目标 ID")
	okrUploadImageCmd.Flags().String("key-result-id", "", "关键结果 ID")
	okrUploadImageCmd.Flags().StringP("output", "o", "", "输出格式：json")
}
