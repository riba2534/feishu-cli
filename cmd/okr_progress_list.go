package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var okrProgressListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取目标或关键结果下的进展记录列表",
	Long: `获取一个 Objective 或 Key Result 下的所有进展记录（v2 接口，自动分页）。

二选一参数（必须且只能填一个）:
  --objective-id      目标 ID
  --key-result-id     关键结果 ID

可选参数:
  --user-id-type      用户 ID 类型：open_id（默认） / union_id / user_id
  --output, -o        输出格式：json

权限要求（应用 Token / Tenant，实测 user token 被服务端拒：99991668）:
  okr:okr:readonly 或 okr:okr.progress:readonly

示例:
  # 列出某个 Objective 的所有进展
  feishu-cli okr progress list --objective-id 7xxx

  # 列出某个 Key Result 的进展（JSON 输出）
  feishu-cli okr progress list --key-result-id 7xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		objectiveID, _ := cmd.Flags().GetString("objective-id")
		keyResultID, _ := cmd.Flags().GetString("key-result-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		targetID, targetType, err := pickOKRTarget(objectiveID, keyResultID)
		if err != nil {
			return err
		}

		if err := validateUserIDType(userIDType); err != nil {
			return err
		}

		// OKR API 服务端拒 user access token（99991668 实测），强制 App Token
		token := ""
		progresses, err := client.ListOKRProgresses(client.ListOKRProgressesOptions{
			TargetID:   targetID,
			TargetType: targetType,
			UserIDType: userIDType,
		}, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"progresses": progresses,
				"total":      len(progresses),
			})
		}

		if len(progresses) == 0 {
			fmt.Println("未找到 OKR 进展记录")
			return nil
		}

		fmt.Printf("共找到 %d 条 OKR 进展记录\n", len(progresses))
		for idx, p := range progresses {
			fmt.Printf("[%d] %s\n", idx+1, p.ProgressID)
			if p.ModifyTime != "" {
				fmt.Printf("    修改时间: %s\n", p.ModifyTime)
			}
			if p.CreateTime != "" {
				fmt.Printf("    创建时间: %s\n", p.CreateTime)
			}
			if p.ProgressRate != nil && p.ProgressRate.Percent != nil {
				fmt.Printf("    进度: %.1f%%\n", *p.ProgressRate.Percent)
			}
			if p.ProgressRate != nil && p.ProgressRate.Status != "" {
				fmt.Printf("    状态: %s\n", p.ProgressRate.Status)
			}
		}
		return nil
	},
}

func init() {
	okrProgressCmd.AddCommand(okrProgressListCmd)

	okrProgressListCmd.Flags().String("objective-id", "", "目标 ID（与 --key-result-id 二选一）")
	okrProgressListCmd.Flags().String("key-result-id", "", "关键结果 ID（与 --objective-id 二选一）")
	okrProgressListCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrProgressListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	okrProgressListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌，留空则自动读取登录态）")
}
