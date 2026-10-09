package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listWikiSpacesCmd = &cobra.Command{
	Use:   "spaces",
	Short: "列出知识空间",
	Long: `列出当前用户/应用有权限访问的知识空间列表。

空间类型（space_type）:
  team      团队空间
  person    个人空间

可见性（visibility）:
  public    公开空间
  private   私有空间

示例:
  # 列出所有知识空间
  feishu-cli wiki spaces

  # JSON 格式输出
  feishu-cli wiki spaces --output json

  # 指定每页数量
  feishu-cli wiki spaces --page-size 20

  # 自动翻页拉取全部（默认只取一页；has_more 时 stderr 会提示 page_token）
  feishu-cli wiki spaces --page-all -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")
		pageOpts, err := readListPageOptions(cmd)
		if err != nil {
			return err
		}

		userToken := resolveOptionalUserTokenWithFallback(cmd)
		res, err := collectListPages(pageOpts, func(pageToken string) ([]*client.WikiSpace, string, bool, error) {
			return client.ListWikiSpaces(pageSize, pageToken, userToken)
		})
		if err != nil {
			return err
		}
		spaces := res.Items
		// has_more 时把续翻 page_token 打到 stderr（stdout 保持原有 JSON 数组形状，管道安全）
		defer printListPageHint(cmd.ErrOrStderr(), res)

		if output == "json" {
			if err := printJSON(emptyOrSlice(spaces)); err != nil {
				return err
			}
		} else {
			if len(spaces) == 0 {
				fmt.Println("未找到知识空间（可能没有访问权限）")
				return nil
			}

			fmt.Printf("共找到 %d 个知识空间:\n\n", len(spaces))
			for i, space := range spaces {
				fmt.Printf("[%d] %s\n", i+1, space.Name)
				fmt.Printf("    空间 ID:   %s\n", space.SpaceID)
				if space.SpaceType != "" {
					fmt.Printf("    类型:      %s\n", space.SpaceType)
				}
				if space.Visibility != "" {
					fmt.Printf("    可见性:    %s\n", space.Visibility)
				}
				if space.Description != "" {
					fmt.Printf("    描述:      %s\n", space.Description)
				}
				fmt.Println()
			}
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(listWikiSpacesCmd)
	listWikiSpacesCmd.Flags().Int("page-size", 50, "每页数量")
	addListPageFlags(listWikiSpacesCmd)
	listWikiSpacesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	listWikiSpacesCmd.Flags().String("user-access-token", "", "User Access Token（可选；默认优先使用 auth login 登录态，失败时回退 App Token）")
}
