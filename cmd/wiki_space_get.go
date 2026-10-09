package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var wikiSpaceGetCmd = &cobra.Command{
	Use:   "space-get <space_id>",
	Short: "获取知识空间详情",
	Long: `获取指定知识空间的详细信息。

参数:
  space_id    知识空间 ID（位置参数）

示例:
  feishu-cli wiki space-get SPACE_ID
  feishu-cli wiki space-get SPACE_ID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		output, _ := cmd.Flags().GetString("output")

		space, err := client.GetWikiSpace(spaceID, resolveOptionalUserTokenWithFallback(cmd))
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(space)
		}

		fmt.Printf("空间 ID:     %s\n", space.SpaceID)
		fmt.Printf("名称:        %s\n", space.Name)
		if space.Description != "" {
			fmt.Printf("描述:        %s\n", space.Description)
		}
		fmt.Printf("类型:        %s\n", space.SpaceType)
		fmt.Printf("可见性:      %s\n", space.Visibility)
		if space.OpenSharing != "" {
			fmt.Printf("分享状态:    %s\n", space.OpenSharing)
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(wikiSpaceGetCmd)
	wikiSpaceGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	wikiSpaceGetCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
}
