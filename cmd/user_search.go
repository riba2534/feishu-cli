package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var userSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "通过姓名、邮箱或手机号查询用户",
	Long: `通过姓名、邮箱或手机号查询用户。至少需要指定一个查询条件。

参数:
  --name      姓名（模糊匹配，递归搜索所有部门）
  --email     邮箱列表，逗号分隔
  --mobile    手机号列表，逗号分隔

示例:
  feishu-cli user search --name "张三"
  feishu-cli user search --email user@example.com
  feishu-cli user search --mobile +8613800138000
  feishu-cli user search --email a@example.com,b@example.com -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nameStr, _ := cmd.Flags().GetString("name")
		emailStr, _ := cmd.Flags().GetString("email")
		mobileStr, _ := cmd.Flags().GetString("mobile")
		output, _ := cmd.Flags().GetString("output")

		if nameStr == "" && emailStr == "" && mobileStr == "" {
			return fmt.Errorf("至少需要指定 --name、--email 或 --mobile")
		}

		// 按姓名搜索（递归遍历部门）
		if nameStr != "" {
			users, err := client.SearchUsersByName(nameStr)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(users)
			}

			if len(users) == 0 {
				fmt.Printf("未找到姓名包含 \"%s\" 的用户\n", nameStr)
				return nil
			}

			fmt.Printf("查询结果（共 %d 条，姓名匹配 \"%s\"）:\n\n", len(users), nameStr)
			for i, u := range users {
				fmt.Printf("[%d] %s", i+1, u.Name)
				if u.EnName != "" {
					fmt.Printf(" (%s)", u.EnName)
				}
				fmt.Println()
				if u.OpenID != "" {
					fmt.Printf("    Open ID: %s\n", u.OpenID)
				}
				if u.Email != "" {
					fmt.Printf("    邮箱: %s\n", u.Email)
				}
				if u.JobTitle != "" {
					fmt.Printf("    职位: %s\n", u.JobTitle)
				}
				if u.Status != "" {
					fmt.Printf("    状态: %s\n", u.Status)
				}
				fmt.Println()
			}
			return nil
		}

		// 按邮箱/手机号搜索（原有逻辑）
		var emails, mobiles []string
		if emailStr != "" {
			emails = splitAndTrim(emailStr)
		}
		if mobileStr != "" {
			mobiles = splitAndTrim(mobileStr)
		}

		result, err := client.BatchGetUserID(emails, mobiles)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result) == 0 {
			fmt.Println("未找到匹配的用户")
			return nil
		}

		fmt.Printf("查询结果（共 %d 条）:\n\n", len(result))
		for i, item := range result {
			fmt.Printf("[%d] 用户 ID: %s\n", i+1, item.UserID)
			if item.Email != "" {
				fmt.Printf("    邮箱: %s\n", item.Email)
			}
			if item.Mobile != "" {
				fmt.Printf("    手机号: %s\n", item.Mobile)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	userCmd.AddCommand(userSearchCmd)
	userSearchCmd.Flags().String("name", "", "姓名（模糊匹配，递归搜索所有部门）")
	userSearchCmd.Flags().String("email", "", "邮箱列表，逗号分隔")
	userSearchCmd.Flags().String("mobile", "", "手机号列表，逗号分隔")
	userSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
