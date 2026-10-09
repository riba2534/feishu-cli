package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var uploadFileCmd = &cobra.Command{
	Use:   "upload <local_path>",
	Short: "上传文件到云空间",
	Long: `上传本地文件到飞书云空间。

参数:
  local_path    本地文件路径

选项:
  --parent      父文件夹 Token（默认上传到根目录）
  --name        文件名（默认使用本地文件名）

Bot 身份上传时自动给当前 CLI 登录用户授予 full_access（JSON 输出 permission_grant）。

示例:
  # 上传文件到根目录
  feishu-cli file upload /tmp/report.pdf

  # 上传到指定文件夹
  feishu-cli file upload /tmp/report.pdf --parent fldcnXXXXXXXXX

  # 上传并重命名
  feishu-cli file upload /tmp/report.pdf --parent fldcnXXXXXXXXX --name "月度报告.pdf"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		localPath := args[0]
		parentToken, _ := cmd.Flags().GetString("parent")
		fileName, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")
		// 本地文件在任何网络请求之前校验：敏感目录、不存在、是目录、无权限读取均为用法错误
		if _, err := safefile.StatInputFile(localPath); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)

		fileToken, err := client.UploadFileWithToken(localPath, parentToken, fileName, userAccessToken)
		if err != nil {
			return err
		}

		displayName := fileName
		if displayName == "" {
			displayName = filepath.Base(localPath)
		}

		// Bot 身份上传时自动给当前 CLI 登录用户授予 full_access（User 身份上传不触发）
		grant := autoGrantCurrentUser(userAccessToken, fileToken, client.ResourceTypeFile)

		if output == "json" {
			return printJSON(withPermissionGrant(map[string]any{
				"file_token": fileToken,
				"file_name":  displayName,
			}, grant))
		}

		fmt.Printf("文件上传成功！\n")
		fmt.Printf("  文件名:     %s\n", displayName)
		fmt.Printf("  文件 Token: %s\n", fileToken)
		printPermissionGrantText(os.Stdout, grant)

		return nil
	},
}

func init() {
	fileCmd.AddCommand(uploadFileCmd)
	uploadFileCmd.Flags().String("parent", "", "父文件夹 Token（默认根目录）")
	uploadFileCmd.Flags().String("name", "", "文件名（默认使用本地文件名）")
	uploadFileCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	uploadFileCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
