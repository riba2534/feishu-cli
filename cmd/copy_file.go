package cmd

import (
	"fmt"
	"net/url"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var copyFileCmd = &cobra.Command{
	Use:   "copy <file_token>",
	Short: "复制文件",
	Long: `复制文件到指定位置。

参数:
  file_token    文件的 Token
  --target      目标文件夹 Token（必填）
  --type        文件类型（必填）
  --name        新文件名称（可选）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件

Bot 身份复制时自动给当前 CLI 登录用户授予副本的 full_access（JSON 输出 permission_grant）。

示例:
  # 复制文档
  feishu-cli file copy doccnXXX --target fldcnYYY --type docx

  # 复制并重命名
  feishu-cli file copy doccnXXX --target fldcnYYY --type docx --name "副本"

  # 预览请求（--dry-run 只打印将要发出的请求，不读取凭据、不联网）
  feishu-cli file copy doccnXXX --target fldcnXXX --type docx --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileToken := args[0]
		targetFolder, _ := cmd.Flags().GetString("target")
		fileType, _ := cmd.Flags().GetString("type")
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			body := map[string]any{"type": fileType, "folder_token": targetFolder}
			if name != "" {
				body["name"] = name
			}
			return printDryRunPlan(cmd, "Copy file", nil, []dryRunStep{{
				Method: "POST",
				URL:    "/open-apis/drive/v1/files/" + url.PathEscape(fileToken) + "/copy",
				Body:   body,
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)

		newToken, url, err := client.CopyFile(fileToken, targetFolder, name, fileType, userAccessToken)
		if err != nil {
			return err
		}

		// Bot 身份复制时自动给当前 CLI 登录用户授予副本的 full_access（User 身份复制不触发）
		grant := autoGrantCurrentUser(userAccessToken, newToken, fileType)

		if output == "json" {
			if err := printJSON(withPermissionGrant(map[string]any{
				"token": newToken,
				"url":   url,
			}, grant)); err != nil {
				return err
			}
		} else {
			fmt.Printf("文件复制成功！\n")
			fmt.Printf("  原文件 Token: %s\n", fileToken)
			fmt.Printf("  新文件 Token: %s\n", newToken)
			if url != "" {
				fmt.Printf("  链接:         %s\n", url)
			}
			printPermissionGrantText(os.Stdout, grant)
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(copyFileCmd)
	copyFileCmd.Flags().String("target", "", "目标文件夹 Token（必填）")
	copyFileCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求（不读取凭据、不联网）")
	copyFileCmd.Flags().String("type", "", "文件类型（必填）")
	copyFileCmd.Flags().String("name", "", "新文件名称")
	copyFileCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	copyFileCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(copyFileCmd, "target", "type")
}
