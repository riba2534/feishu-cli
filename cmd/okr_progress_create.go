package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var okrProgressCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "为目标或关键结果创建一条进展记录",
	Long: `为目标（Objective）或关键结果（Key Result）创建一条进展记录。

二选一参数（必须且只能填一个）:
  --objective-id      目标 ID
  --key-result-id     关键结果 ID

内容参数（二选一）:
  --content           纯文本内容（自动包装为 ContentBlock 富文本）
  --content-json      原始 ContentBlock JSON（适合需要 mention、链接、图片等富文本场景）

可选参数:
  --progress-percent  进度百分比（数字，配合 --progress-status 一起使用）
  --progress-status   进度状态：normal / overdue / done
  --source-title      来源标题（默认 "created by feishu-cli"）
  --source-url        来源 URL（飞书 API 必填字段；默认 "https://www.feishu.cn/okr/progress"，
                      可改成进展实际跳转地址）
  --user-id-type      用户 ID 类型：open_id（默认） / union_id / user_id
  --output, -o        输出格式：json

权限要求（User Token）:
  okr:okr 或 okr:okr.progress:writeonly

示例:
  # 给目标加一条纯文本进展
  feishu-cli okr progress create --objective-id 7xxx --content "本周完成模块联调"

  # 给关键结果加进展 + 进度百分比
  feishu-cli okr progress create \
    --key-result-id 7xxx \
    --content "完成 8/10 任务" \
    --progress-percent 80 \
    --progress-status normal

  # 用 ContentBlock JSON 自定义富文本
  feishu-cli okr progress create \
    --objective-id 7xxx \
    --content-json '{"blocks":[{"type":"paragraph","paragraph":{"elements":[{"type":"textRun","textRun":{"text":"加粗内容","style":{"bold":true}}}]}}]}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		objectiveID, _ := cmd.Flags().GetString("objective-id")
		keyResultID, _ := cmd.Flags().GetString("key-result-id")
		contentText, _ := cmd.Flags().GetString("content")
		contentJSON, _ := cmd.Flags().GetString("content-json")
		progressPercent, _ := cmd.Flags().GetString("progress-percent")
		progressStatus, _ := cmd.Flags().GetString("progress-status")
		sourceTitle, _ := cmd.Flags().GetString("source-title")
		sourceURL, _ := cmd.Flags().GetString("source-url")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		targetID, targetType, err := pickOKRTarget(objectiveID, keyResultID)
		if err != nil {
			return err
		}

		if err := validateUserIDType(userIDType); err != nil {
			return err
		}

		finalContentJSON, err := buildOKRProgressContentJSON(contentText, contentJSON)
		if err != nil {
			return err
		}

		opts := client.CreateOKRProgressOptions{
			ContentJSON: finalContentJSON,
			TargetID:    targetID,
			TargetType:  targetType,
			SourceTitle: sourceTitle,
			SourceURL:   sourceURL,
			UserIDType:  userIDType,
		}

		if progressPercent != "" {
			percent, err := strconv.ParseFloat(progressPercent, 64)
			if err != nil {
				return fmt.Errorf("--progress-percent 必须是数字: %w", err)
			}
			rate := &client.OKRProgressRateInput{Percent: percent}
			if progressStatus != "" {
				status, ok := client.ParseOKRProgressStatus(progressStatus)
				if !ok {
					return fmt.Errorf("--progress-status 必须为 normal / overdue / done")
				}
				rate.Status = &status
			}
			opts.ProgressRate = rate
		} else if progressStatus != "" {
			return fmt.Errorf("--progress-status 必须配合 --progress-percent 一起使用")
		}

		token := resolveOptionalUserTokenWithFallback(cmd)
		progress, err := client.CreateOKRProgress(opts, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(progress)
		}

		fmt.Printf("已创建 OKR 进展记录\n")
		fmt.Printf("  进展 ID: %s\n", progress.ProgressID)
		if progress.ModifyTime != "" {
			fmt.Printf("  修改时间: %s\n", progress.ModifyTime)
		}
		if progress.ProgressRate != nil && progress.ProgressRate.Percent != nil {
			fmt.Printf("  进度: %.1f%%\n", *progress.ProgressRate.Percent)
		}
		if progress.ProgressRate != nil && progress.ProgressRate.Status != "" {
			fmt.Printf("  状态: %s\n", progress.ProgressRate.Status)
		}
		return nil
	},
}

// pickOKRTarget 校验 --objective-id / --key-result-id 二选一，返回 (targetID, targetType)
func pickOKRTarget(objectiveID, keyResultID string) (string, client.OKRProgressTargetType, error) {
	hasObj := objectiveID != ""
	hasKR := keyResultID != ""
	switch {
	case hasObj && hasKR:
		return "", 0, fmt.Errorf("--objective-id 和 --key-result-id 只能填一个")
	case hasObj:
		return objectiveID, client.OKRTargetObjective, nil
	case hasKR:
		return keyResultID, client.OKRTargetKeyResult, nil
	default:
		return "", 0, fmt.Errorf("必须指定 --objective-id 或 --key-result-id 之一")
	}
}

// buildOKRProgressContentJSON 把 --content（纯文本）或 --content-json（原始 JSON）归一化为
// 飞书 OKR v1 ContentBlock JSON 字符串。
func buildOKRProgressContentJSON(text, raw string) (string, error) {
	rawTrim := strings.TrimSpace(raw)
	textTrim := strings.TrimSpace(text)
	if rawTrim != "" && textTrim != "" {
		return "", fmt.Errorf("--content 和 --content-json 只能填一个")
	}
	if rawTrim == "" && textTrim == "" {
		return "", fmt.Errorf("必须指定 --content 或 --content-json 之一")
	}
	if rawTrim != "" {
		// 校验是合法 JSON，避免下游报莫名错
		var probe any
		if err := json.Unmarshal([]byte(rawTrim), &probe); err != nil {
			return "", fmt.Errorf("--content-json 不是合法 JSON: %w", err)
		}
		return rawTrim, nil
	}
	// 纯文本：包装成最小可用的 ContentBlock paragraph + textRun
	wrapped := map[string]any{
		"blocks": []any{
			map[string]any{
				"type": "paragraph",
				"paragraph": map[string]any{
					"elements": []any{
						map[string]any{
							"type": "textRun",
							"textRun": map[string]any{
								"text": text,
							},
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(wrapped)
	if err != nil {
		return "", fmt.Errorf("构造 ContentBlock 失败: %w", err)
	}
	return string(data), nil
}

func init() {
	okrProgressCmd.AddCommand(okrProgressCreateCmd)

	okrProgressCreateCmd.Flags().String("objective-id", "", "目标 ID（与 --key-result-id 二选一）")
	okrProgressCreateCmd.Flags().String("key-result-id", "", "关键结果 ID（与 --objective-id 二选一）")
	okrProgressCreateCmd.Flags().String("content", "", "进展内容（纯文本，自动包装为 ContentBlock）")
	okrProgressCreateCmd.Flags().String("content-json", "", "进展内容（原始 ContentBlock JSON 字符串）")
	okrProgressCreateCmd.Flags().String("progress-percent", "", "进度百分比（数字）")
	okrProgressCreateCmd.Flags().String("progress-status", "", "进度状态：normal / overdue / done")
	okrProgressCreateCmd.Flags().String("source-title", "", "来源标题（默认 'created by feishu-cli'）")
	okrProgressCreateCmd.Flags().String("source-url", "", "来源 URL（用于卡片点击跳转）")
	okrProgressCreateCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrProgressCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	okrProgressCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌，留空则自动读取登录态）")
}
