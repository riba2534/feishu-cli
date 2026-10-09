package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

var sheetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建电子表格",
	Long: `创建一个新的电子表格。

以 Bot 身份创建（--as bot，或未登录）时，会自动为当前登录用户授予 full_access，
避免表格只有 Bot 能访问；授权结果写入 JSON 输出的 permission_grant 字段。

示例:
  feishu-cli sheet create --title "周报数据"
  feishu-cli sheet create --title "周报数据" --folder fldcnxxxxxx -o json
  feishu-cli sheet create --title "定时任务产出" --as bot`,
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		folderToken, _ := cmd.Flags().GetString("folder")
		output, _ := cmd.Flags().GetString("output")

		userAccessToken, err := resolveSheetUserToken(cmd)
		if err != nil {
			return err
		}

		info, err := client.CreateSpreadsheet(client.Context(), title, folderToken, userAccessToken)
		if err != nil {
			return err
		}
		// Bot 创建的表格默认只有应用可访问：自动给当前登录用户授 full_access（User 身份创建时跳过）
		grant := autoGrantCurrentUser(userAccessToken, info.SpreadsheetToken, client.ResourceTypeSheet)

		if output == "json" {
			return printJSON(withPermissionGrant(map[string]any{
				"spreadsheet_token": info.SpreadsheetToken,
				"title":             info.Title,
				"url":               info.URL,
				"owner_id":          info.OwnerID,
			}, grant))
		}
		fmt.Printf("创建成功！\n")
		fmt.Printf("  Token: %s\n", info.SpreadsheetToken)
		fmt.Printf("  标题: %s\n", info.Title)
		fmt.Printf("  URL: %s\n", info.URL)
		printPermissionGrantText(os.Stdout, grant)
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
