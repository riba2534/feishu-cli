package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建电子表格",
	Long:  "创建一个新的电子表格",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		folderToken, _ := cmd.Flags().GetString("folder")
		output, _ := cmd.Flags().GetString("output")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		info, err := client.CreateSpreadsheet(client.Context(), title, folderToken, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(info); err != nil {
				return err
			}
		} else {
			fmt.Printf("创建成功！\n")
			fmt.Printf("  Token: %s\n", info.SpreadsheetToken)
			fmt.Printf("  标题: %s\n", info.Title)
			fmt.Printf("  URL: %s\n", info.URL)
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetCreateCmd)

	sheetCreateCmd.Flags().StringP("title", "t", "新建电子表格", "表格标题")
	sheetCreateCmd.Flags().StringP("folder", "f", "", "目标文件夹 Token（可选）")
	sheetCreateCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetCreateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
