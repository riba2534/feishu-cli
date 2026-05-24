package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// wikiNodeRecord 复制后返回的节点信息
type wikiNodeRecord struct {
	SpaceID         string `json:"space_id"`
	NodeToken       string `json:"node_token"`
	ObjToken        string `json:"obj_token,omitempty"`
	ObjType         string `json:"obj_type,omitempty"`
	NodeType        string `json:"node_type,omitempty"`
	Title           string `json:"title,omitempty"`
	ParentNodeToken string `json:"parent_node_token,omitempty"`
	HasChild        bool   `json:"has_child,omitempty"`
}

var wikiNodeCopyCmd = &cobra.Command{
	Use:   "node-copy",
	Short: "复制 wiki 节点到目标 space 或父节点下",
	Long: `复制一个 wiki 节点到目标 space 或目标父节点下。

API: POST /open-apis/wiki/v2/spaces/{space_id}/nodes/{node_token}/copy

参数:
  --space-id                 源 wiki space ID（必填）
  --node-token               要复制的源节点 token（必填）
  --target-space-id          目标 space ID（与 --target-parent-node-token 二选一）
  --target-parent-node-token 目标父节点 token（与 --target-space-id 二选一）
  --title                    复制后的新标题（可选，留空保留原标题）
  --dry-run                  仅打印请求

示例:
  # 复制节点到另一个 space 根目录
  feishu-cli wiki node-copy --space-id 7044xxx --node-token wikcnxxx --target-space-id 7330yyy

  # 复制到同 space 内某个父节点下，改标题
  feishu-cli wiki node-copy --space-id 7044xxx --node-token wikcnxxx \
    --target-parent-node-token wikcnzzz --title "副本-2025"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		spaceID, _ := cmd.Flags().GetString("space-id")
		nodeToken, _ := cmd.Flags().GetString("node-token")
		targetSpace, _ := cmd.Flags().GetString("target-space-id")
		targetParent, _ := cmd.Flags().GetString("target-parent-node-token")
		title, _ := cmd.Flags().GetString("title")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		spaceID = strings.TrimSpace(spaceID)
		nodeToken = strings.TrimSpace(nodeToken)
		targetSpace = strings.TrimSpace(targetSpace)
		targetParent = strings.TrimSpace(targetParent)

		if spaceID == "" || nodeToken == "" {
			return fmt.Errorf("--space-id 和 --node-token 必填")
		}
		if targetSpace == "" && targetParent == "" {
			return fmt.Errorf("--target-space-id 或 --target-parent-node-token 至少传一个")
		}
		if targetSpace != "" && targetParent != "" {
			return fmt.Errorf("--target-space-id 和 --target-parent-node-token 互斥，只能传一个")
		}

		body := map[string]any{}
		if targetSpace != "" {
			body["target_space_id"] = targetSpace
		}
		if targetParent != "" {
			body["target_parent_token"] = targetParent
		}
		if title != "" {
			body["title"] = title
		}

		// URL path 段需要转义（虽然 wiki space_id 都是纯数字，node_token 都是字母数字，但 belt-and-suspenders）
		apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s/copy",
			url.PathEscape(spaceID), url.PathEscape(nodeToken))

		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"method":  "POST",
				"path":    apiPath,
				"body":    body,
			})
		}

		// 复制属于"高风险写"，先 auto token（User 优先回退 Bot）
		token := resolveOptionalUserTokenWithFallback(cmd)

		fmt.Fprintf(cmd.ErrOrStderr(), "复制 wiki 节点 %s (space %s)...\n", nodeToken, spaceID)

		raw, status, err := driveAPICall(http.MethodPost, apiPath, nil, body, token)
		if err != nil {
			return fmt.Errorf("复制节点失败: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("HTTP %d: %s", status, string(raw))
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Node wikiNodeRecord `json:"node"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
		if resp.Code != 0 {
			return fmt.Errorf("飞书业务错误: code=%d msg=%s", resp.Code, resp.Msg)
		}

		n := resp.Data.Node
		fmt.Printf("✅ 节点复制成功！\n")
		fmt.Printf("  title:             %s\n", n.Title)
		fmt.Printf("  node_token:        %s\n", n.NodeToken)
		fmt.Printf("  space_id:          %s\n", n.SpaceID)
		fmt.Printf("  obj_type:          %s\n", n.ObjType)
		fmt.Printf("  obj_token:         %s\n", n.ObjToken)
		if n.ParentNodeToken != "" {
			fmt.Printf("  parent_node_token: %s\n", n.ParentNodeToken)
		}
		return nil
	},
}

func init() {
	wikiCmd.AddCommand(wikiNodeCopyCmd)
	wikiNodeCopyCmd.Flags().String("space-id", "", "源 wiki space ID（必填）")
	wikiNodeCopyCmd.Flags().String("node-token", "", "要复制的源节点 token（必填）")
	wikiNodeCopyCmd.Flags().String("target-space-id", "", "目标 space ID（与 --target-parent-node-token 二选一）")
	wikiNodeCopyCmd.Flags().String("target-parent-node-token", "", "目标父节点 token（与 --target-space-id 二选一）")
	wikiNodeCopyCmd.Flags().String("title", "", "复制后的新标题（可选）")
	wikiNodeCopyCmd.Flags().Bool("dry-run", false, "仅打印请求")
	wikiNodeCopyCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(wikiNodeCopyCmd, "space-id", "node-token")
}
