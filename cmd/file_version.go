package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "文件版本管理",
	Long: `文件版本管理命令，包括创建、获取、列出、删除文件版本。

子命令:
  list      列出文件版本
  create    创建文件版本
  get       获取版本详情
  delete    删除文件版本

文档类型（--obj-type）:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

示例:
  # 列出文件版本
  feishu-cli file version list <file_token> --obj-type docx

  # 创建文件版本
  feishu-cli file version create <file_token> --obj-type docx --name "v1.0"

  # 获取版本详情
  feishu-cli file version get <file_token> <version_id> --obj-type docx

  # 删除文件版本
  feishu-cli file version delete <file_token> <version_id> --obj-type docx`,
}

var listVersionCmd = &cobra.Command{
	Use:   "list <file_token>",
	Short: "列出文件版本",
	Long: `列出指定文件的所有版本。

参数:
  file_token    文件的 Token

示例:
  feishu-cli file version list doccnXXX --obj-type docx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		objType, _ := cmd.Flags().GetString("obj-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		versions, _, _, err := client.ListFileVersions(fileToken, objType, pageSize, "", userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(versions)
		}

		if len(versions) == 0 {
			fmt.Println("暂无版本记录")
			return nil
		}

		fmt.Printf("共 %d 个版本:\n\n", len(versions))
		for i, v := range versions {
			fmt.Printf("[%d] %s (版本号: %s)\n", i+1, v.Name, v.Version)
			if v.CreateTime != "" {
				fmt.Printf("    创建时间: %s\n", v.CreateTime)
			}
			if v.Status != "" {
				fmt.Printf("    状态: %s\n", v.Status)
			}
			fmt.Println()
		}

		return nil
	},
}

var createVersionCmd = &cobra.Command{
	Use:   "create <file_token>",
	Short: "创建文件版本",
	Long: `创建指定文件的一个新版本。

参数:
  file_token    文件的 Token

示例:
  feishu-cli file version create doccnXXX --obj-type docx --name "v1.0"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		objType, _ := cmd.Flags().GetString("obj-type")
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		version, err := client.CreateFileVersion(fileToken, objType, name, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(version)
		}

		fmt.Printf("版本创建成功！\n")
		fmt.Printf("  名称:     %s\n", version.Name)
		fmt.Printf("  版本号:   %s\n", version.Version)
		if version.CreateTime != "" {
			fmt.Printf("  创建时间: %s\n", version.CreateTime)
		}

		return nil
	},
}

var getVersionCmd = &cobra.Command{
	Use:   "get <file_token> <version_id>",
	Short: "获取版本详情",
	Long: `获取指定文件版本的详细信息。

参数:
  file_token    文件的 Token
  version_id    版本号

示例:
  feishu-cli file version get doccnXXX ver_xxxxx --obj-type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		versionID := args[1]
		objType, _ := cmd.Flags().GetString("obj-type")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		version, err := client.GetFileVersion(fileToken, versionID, objType, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(version)
		}

		fmt.Printf("版本详情:\n")
		fmt.Printf("  名称:     %s\n", version.Name)
		fmt.Printf("  版本号:   %s\n", version.Version)
		if version.CreatorID != "" {
			fmt.Printf("  创建者:   %s\n", version.CreatorID)
		}
		if version.CreateTime != "" {
			fmt.Printf("  创建时间: %s\n", version.CreateTime)
		}
		if version.UpdateTime != "" {
			fmt.Printf("  更新时间: %s\n", version.UpdateTime)
		}
		if version.Status != "" {
			fmt.Printf("  状态:     %s\n", version.Status)
		}

		return nil
	},
}

var deleteVersionCmd = &cobra.Command{
	Use:   "delete <file_token> <version_id>",
	Short: "删除文件版本",
	Long: `删除指定的文件版本。

参数:
  file_token    文件的 Token
  version_id    版本号

示例:
  feishu-cli file version delete doccnXXX ver_xxxxx --obj-type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		versionID := args[1]
		objType, _ := cmd.Flags().GetString("obj-type")
		userAccessToken := resolveOptionalUserToken(cmd)

		if err := client.DeleteFileVersion(fileToken, versionID, objType, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("版本删除成功！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  版本号:     %s\n", versionID)

		return nil
	},
}

func init() {
	fileCmd.AddCommand(versionCmd)
	versionCmd.PersistentFlags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")

	// list 子命令
	versionCmd.AddCommand(listVersionCmd)
	listVersionCmd.Flags().String("obj-type", "docx", "文档类型（doc/docx/sheet/bitable）")
	listVersionCmd.Flags().Int("page-size", 50, "每页数量")
	listVersionCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	// create 子命令
	versionCmd.AddCommand(createVersionCmd)
	createVersionCmd.Flags().String("obj-type", "docx", "文档类型（doc/docx/sheet/bitable）")
	createVersionCmd.Flags().String("name", "", "版本名称（必填）")
	createVersionCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(createVersionCmd, "name")

	// get 子命令
	versionCmd.AddCommand(getVersionCmd)
	getVersionCmd.Flags().String("obj-type", "docx", "文档类型（doc/docx/sheet/bitable）")
	getVersionCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	// delete 子命令
	versionCmd.AddCommand(deleteVersionCmd)
	deleteVersionCmd.Flags().String("obj-type", "docx", "文档类型（doc/docx/sheet/bitable）")
}
