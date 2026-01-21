package cmd

import (
	"encoding/json"
	"fmt"

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

		token, url, err := client.CreateFolder(name, parentToken)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"token": token,
				"url":   url,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("文件夹创建成功！\n")
			fmt.Printf("  名称:  %s\n", name)
			fmt.Printf("  Token: %s\n", token)
			if url != "" {
				fmt.Printf("  链接:  %s\n", url)
			}
		}

		return nil
	},
}

func init() {
	fileCmd.AddCommand(createFolderCmd)
	createFolderCmd.Flags().String("parent", "", "父文件夹 Token")
	createFolderCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
