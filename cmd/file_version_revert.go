package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var revertVersionCmd = &cobra.Command{
	Use:   "revert <file_token> <version>",
	Short: "回滚文件到指定历史版本",
	Long: `将文件回滚（还原）到某个历史版本。回滚后文件当前内容变为该历史版本的内容。

参数:
  file_token    文件的 Token
  version       目标版本号（file version list 返回的长数字 version，不是 tag）

底层接口：POST /open-apis/drive/v1/files/{file_token}/revert，请求体 {"version": version}

提示:
  - version 从 feishu-cli file version list <file_token> 获取
  - 回滚是写操作，默认以 Bot 身份执行；如需用户身份，传 --user-access-token

示例:
  feishu-cli file version revert doccnXXX 7633658129540910621
  feishu-cli file version revert doccnXXX 7633658129540910621 --user-access-token u-xxx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		version := args[1]
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		if err := client.RevertFileVersion(fileToken, version, userAccessToken); err != nil {
			return err
		}

		result := map[string]any{
			"file_token": fileToken,
			"version":    version,
			"reverted":   true,
		}
		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("文件已回滚到指定版本！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  版本号:     %s\n", version)
		return nil
	},
}

func init() {
	// 仅在此新文件中挂载 revert 子命令，不改动 cmd/file_version.go。
	versionCmd.AddCommand(revertVersionCmd)
	revertVersionCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
