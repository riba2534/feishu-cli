package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createWikiNodeCmd = &cobra.Command{
	Use:   "create",
	Short: "创建知识库节点",
	Long: `在指定知识空间中创建新的节点（文档）。

参数:
  --space-id      知识空间 ID（必填）
  --title         节点标题（必填）
  --parent-node   父节点 Token（可选，不指定则创建在根目录）
  --obj-type      文档类型（可选，默认 docx）
  --node-type     节点类型（可选，默认 origin）

文档类型（--obj-type）:
  docx      新版文档（默认）
  doc       旧版文档
  sheet     电子表格

节点类型（--node-type）:
  origin    实体节点（默认）
  shortcut  快捷方式

Bot 身份创建时自动给当前 CLI 登录用户授予节点 full_access（容器权限，JSON 输出 url 与 permission_grant）。

示例:
  # 在知识空间根目录创建文档
  feishu-cli wiki create --space-id 7012345678901234567 --title "新文档"

  # 在指定父节点下创建文档
  feishu-cli wiki create --space-id 7012345678901234567 --title "子文档" --parent-node wikcnXXXXXX

  # 创建电子表格
  feishu-cli wiki create --space-id 7012345678901234567 --title "数据表" --obj-type sheet

  # JSON 格式输出
  feishu-cli wiki create --space-id 7012345678901234567 --title "新文档" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID, _ := cmd.Flags().GetString("space-id")
		title, _ := cmd.Flags().GetString("title")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		objType, _ := cmd.Flags().GetString("obj-type")
		nodeType, _ := cmd.Flags().GetString("node-type")
		originNodeToken, _ := cmd.Flags().GetString("origin-node-token")
		output, _ := cmd.Flags().GetString("output")

		if nodeType == "shortcut" && originNodeToken == "" {
			return fmt.Errorf("--origin-node-token 在 --node-type=shortcut 时必填")
		}
		if nodeType != "shortcut" && originNodeToken != "" {
			return fmt.Errorf("--origin-node-token 仅在 --node-type=shortcut 时可用")
		}

		userAccessToken := resolveOptionalUserToken(cmd)
		result, err := client.CreateWikiNode(spaceID, title, parentNode, objType, nodeType, originNodeToken, userAccessToken)
		if err != nil {
			return err
		}

		// Bot 身份创建时自动给当前 CLI 登录用户授予节点 full_access（wiki 授予容器权限；User 身份创建不触发）
		grant := autoGrantCurrentUser(userAccessToken, result.NodeToken, client.ResourceTypeWiki)

		if output == "json" {
			out := map[string]any{
				"space_id":   result.SpaceID,
				"node_token": result.NodeToken,
				"obj_token":  result.ObjToken,
				"obj_type":   result.ObjType,
			}
			if result.NodeType != "" {
				out["node_type"] = result.NodeType
			}
			if result.OriginNodeToken != "" {
				out["origin_node_token"] = result.OriginNodeToken
			}
			if url := client.BuildResourceURL(client.ResourceTypeWiki, result.NodeToken); url != "" {
				out["url"] = url
			}
			if err := printJSON(withPermissionGrant(out, grant)); err != nil {
				return err
			}
		} else {
			fmt.Printf("知识库节点创建成功！\n")
			fmt.Printf("  空间 ID:    %s\n", result.SpaceID)
			fmt.Printf("  节点 Token: %s\n", result.NodeToken)
			fmt.Printf("  文档 Token: %s\n", result.ObjToken)
			fmt.Printf("  文档类型:   %s\n", result.ObjType)
			if result.OriginNodeToken != "" {
				fmt.Printf("  源节点:     %s\n", result.OriginNodeToken)
			}
			if url := client.BuildResourceURL(client.ResourceTypeWiki, result.NodeToken); url != "" {
				fmt.Printf("  链接:       %s\n", url)
			}
			printPermissionGrantText(os.Stdout, grant)
		}

		return nil
	},
}

func init() {
	wikiCmd.AddCommand(createWikiNodeCmd)
	createWikiNodeCmd.Flags().String("space-id", "", "知识空间 ID（必填）")
	createWikiNodeCmd.Flags().String("title", "", "节点标题（必填）")
	createWikiNodeCmd.Flags().String("parent-node", "", "父节点 Token（可选）")
	createWikiNodeCmd.Flags().String("obj-type", "docx", "文档类型：docx/doc/sheet（默认 docx）")
	createWikiNodeCmd.Flags().String("node-type", "origin", "节点类型：origin/shortcut（默认 origin）")
	createWikiNodeCmd.Flags().String("origin-node-token", "", "快捷方式指向的源节点 Token（当 --node-type=shortcut 时必填）")
	createWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(createWikiNodeCmd, "space-id", "title")
	createWikiNodeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
