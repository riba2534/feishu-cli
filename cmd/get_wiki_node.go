package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getWikiNodeCmd = &cobra.Command{
	Use:   "get <node_token|url>",
	Short: "获取知识库节点信息",
	Long: `获取知识库节点的详细信息，包括文档类型、标题、创建时间等。

参数:
  node_token    节点 Token（从 URL 中提取）
  url           知识库文档 URL

URL 格式示例:
  https://xxx.feishu.cn/wiki/Ad8Iw0oz3iSp4kkIi7QctVhin3e
  https://xxx.larkoffice.com/wiki/Ad8Iw0oz3iSp4kkIi7QctVhin3e

返回信息:
  space_id        知识空间 ID
  node_token      节点 Token
  obj_token       文档 Token（用于调用文档 API）
  obj_type        文档类型（docx/doc/sheet/bitable/mindnote/file/slides）
  title           文档标题
  has_child       是否有子节点

示例:
  # 通过 Token 获取
  feishu-cli wiki get Ad8Iw0oz3iSp4kkIi7QctVhin3e

  # 通过 URL 获取
  feishu-cli wiki get https://xxx.feishu.cn/wiki/Ad8Iw0oz3iSp4kkIi7QctVhin3e

  # JSON 格式输出
  feishu-cli wiki get Ad8Iw0oz3iSp4kkIi7QctVhin3e --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		// 解析 node_token（支持 URL 或直接 token）
		nodeToken, err := extractWikiToken(args[0])
		if err != nil {
			return err
		}

		node, err := client.GetWikiNode(nodeToken)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(node, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("知识库节点信息:\n")
			fmt.Printf("  空间 ID:     %s\n", node.SpaceID)
			fmt.Printf("  节点 Token:  %s\n", node.NodeToken)
			fmt.Printf("  文档 Token:  %s\n", node.ObjToken)
			fmt.Printf("  文档类型:    %s\n", node.ObjType)
			fmt.Printf("  节点类型:    %s\n", node.NodeType)
			fmt.Printf("  标题:        %s\n", node.Title)
			fmt.Printf("  有子节点:    %v\n", node.HasChild)
			if node.ObjCreateTime != "" {
				fmt.Printf("  创建时间:    %s\n", node.ObjCreateTime)
			}
			if node.ObjEditTime != "" {
				fmt.Printf("  编辑时间:    %s\n", node.ObjEditTime)
			}
		}

		return nil
	},
}

// extractWikiToken 从 URL 或直接的 token 中提取 node_token
func extractWikiToken(input string) (string, error) {
	// 尝试匹配 wiki URL
	re := regexp.MustCompile(`/wiki/([a-zA-Z0-9]+)`)
	matches := re.FindStringSubmatch(input)
	token := input
	if len(matches) > 1 {
		token = matches[1]
	}

	// 验证 token 格式
	if !isValidToken(token) {
		return "", fmt.Errorf("无效的节点 token: %s", token)
	}

	return token, nil
}

func init() {
	wikiCmd.AddCommand(getWikiNodeCmd)
	getWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
