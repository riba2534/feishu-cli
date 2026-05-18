package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailTemplateListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出当前邮箱下的全部邮件模板",
	Long: `列出指定邮箱（默认 me）下的全部个人邮件模板。

注意: 该 OpenAPI 不分页，会一次性返回所有模板的 id 与 name。

可选:
  --mailbox  邮箱 ID（默认 me）
  -o json    JSON 输出

权限:
  - User Access Token
  - mail:user_mailbox:readonly
  - mail:user.email.template（字节租户暂未开放）

示例:
  feishu-cli mail template list
  feishu-cli mail template list -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail template list")
		if err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		output, _ := cmd.Flags().GetString("output")

		list, err := client.ListMailTemplates(mailbox, token)
		if err != nil {
			return fmt.Errorf("列出邮件模板失败: %w", err)
		}

		if output == "json" {
			return printJSON(map[string]any{
				"templates": list,
				"total":     len(list),
			})
		}
		if len(list) == 0 {
			fmt.Println("当前邮箱下没有任何邮件模板。")
			return nil
		}
		fmt.Printf("共 %d 个邮件模板:\n", len(list))
		for i, t := range list {
			fmt.Printf("  %d. [%s] %s\n", i+1, t.TemplateID, t.Name)
		}
		return nil
	},
}

func init() {
	mailTemplateCmd.AddCommand(mailTemplateListCmd)
	mailTemplateListCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailTemplateListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailTemplateListCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
