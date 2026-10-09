package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createFolderCmd = &cobra.Command{
	Use:   "mkdir <name>",
	Short: "创建文件夹",
	Long: `在云空间中创建新文件夹。

参数:
  name       文件夹名称
  --parent   父文件夹 Token（不指定则在根目录创建）

Bot 身份创建时自动给当前 CLI 登录用户授予 full_access（JSON 输出 permission_grant）。

示例:
  # 在根目录创建
  feishu-cli file mkdir "我的文件夹"

  # 在指定文件夹下创建
  feishu-cli file mkdir "子文件夹" --parent fldcnXXXXXXXXX

  # JSON 格式输出
  feishu-cli file mkdir "新文件夹" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		name := args[0]
		parentToken, _ := cmd.Flags().GetString("parent")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		token, url, err := client.CreateFolder(name, parentToken, userAccessToken)
		if err != nil {
			return err
		}

		// Bot 身份创建时自动给当前 CLI 登录用户授予 full_access（User 身份创建不触发）
		grant := autoGrantCurrentUser(userAccessToken, token, client.ResourceTypeFolder)

		if output == "json" {
			if err := printJSON(withPermissionGrant(map[string]any{
				"token": token,
				"url":   url,
			}, grant)); err != nil {
				return err
			}
		} else {
			fmt.Printf("文件夹创建成功！\n")
			fmt.Printf("  名称:  %s\n", name)
			fmt.Printf("  Token: %s\n", token)
			if url != "" {
				fmt.Printf("  链接:  %s\n", url)
			}
			printPermissionGrantText(os.Stdout, grant)
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(createFolderCmd)
	createFolderCmd.Flags().String("parent", "", "父文件夹 Token")
	createFolderCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	createFolderCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
