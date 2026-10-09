package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// wikiSpacesAPIPath 飞书 wiki space 接口路径
const wikiSpacesAPIPath = "/open-apis/wiki/v2/spaces"

// wikiSpaceItem 拉取/创建后归一化的 space 数据
type wikiSpaceItem struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SpaceType   string `json:"space_type,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	OpenSharing string `json:"open_sharing,omitempty"`
}

// =====================================================
// wiki space-create
// =====================================================

var wikiSpaceCreateCmd = &cobra.Command{
	Use:   "space-create",
	Short: "创建知识库 space",
	Long: `创建一个新的知识库 space。

⚠️ 必需 User Token（接口不支持 Bot Token）；推荐 scope: wiki:space:write_only

参数:
  --name           知识库名称（必填）
  --description    知识库描述（可选）
  --dry-run        仅打印请求，不实际创建

示例:
  feishu-cli wiki space-create --name "我的新知识库"
  feishu-cli wiki space-create --name "项目 A" --description "项目 A 的文档归档"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("--name 不能为空")
		}

		body := map[string]any{"name": name}
		if desc != "" {
			body["description"] = desc
		}

		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"method":  "POST",
				"path":    wikiSpacesAPIPath,
				"body":    body,
			})
		}

		userToken, err := requireUserToken(cmd, "wiki space-create")
		if err != nil {
			return err
		}

		raw, status, err := driveAPICall(http.MethodPost, wikiSpacesAPIPath, nil, body, userToken)
		if err != nil {
			return fmt.Errorf("创建知识库失败: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("HTTP %d: %s", status, string(raw))
		}

		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Space wikiSpaceItem `json:"space"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
		if resp.Code != 0 {
			return fmt.Errorf("飞书业务错误: code=%d msg=%s", resp.Code, resp.Msg)
		}

		fmt.Printf("✅ 知识库创建成功！\n")
		fmt.Printf("  space_id:    %s\n", resp.Data.Space.SpaceID)
		fmt.Printf("  name:        %s\n", resp.Data.Space.Name)
		if resp.Data.Space.Description != "" {
			fmt.Printf("  description: %s\n", resp.Data.Space.Description)
		}
		if resp.Data.Space.SpaceType != "" {
			fmt.Printf("  space_type:  %s\n", resp.Data.Space.SpaceType)
		}
		return nil
	},
}

// =====================================================
// wiki space-list
// =====================================================

var wikiSpaceListCmd = &cobra.Command{
	Use:   "space-list",
	Short: "列出当前用户/Bot 可见的所有知识库 space",
	Long: `列出当前 User Token / Bot Token 可访问的所有知识库 space。

参数:
  --page-size      每页大小（1-50，默认 50）
  --page-token     分页起始 token（指定时禁用自动翻页）
  --page-all       自动翻完所有页（默认 false，仅返回第一页）
  --page-limit     --page-all 时最多翻多少页（默认 10，0 = 不限）
  --output, -o     输出格式 (json)
  --as             身份选择 (bot/user/auto)，默认 auto

注意:
  - 默认只返回第一页，要全量请加 --page-all
  - my_library 不会出现在列表里（飞书 API 限制），需单独查 spaces/get

示例:
  feishu-cli wiki space-list                          # 第一页
  feishu-cli wiki space-list --page-all               # 全部
  feishu-cli wiki space-list --page-all --page-limit 0 -o json | jq '.spaces[].name'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		pageSize, _ := cmd.Flags().GetInt("page-size")
		startToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		output, _ := cmd.Flags().GetString("output")
		asFlag, _ := cmd.Flags().GetString("as")

		if pageSize < 1 || pageSize > 50 {
			return fmt.Errorf("--page-size 必须在 1-50 之间")
		}
		if pageLimit < 0 {
			return fmt.Errorf("--page-limit 必须 >= 0")
		}

		// 显式 --page-token 时禁用自动翻页（与官方对齐）
		if strings.TrimSpace(startToken) != "" && pageAll {
			fmt.Fprintln(cmd.ErrOrStderr(), "⚠️ --page-token 已指定，--page-all 被忽略")
			pageAll = false
		}

		token, err := resolveChatToken(cmd, asFlag)
		if err != nil {
			return err
		}

		spaces := make([]wikiSpaceItem, 0)
		pageToken := startToken
		var lastHasMore bool
		var lastPageToken string
		for page := 0; ; page++ {
			params := map[string]string{
				"page_size": fmt.Sprintf("%d", pageSize),
			}
			if pageToken != "" {
				params["page_token"] = pageToken
			}
			raw, status, err := driveAPICall(http.MethodGet, wikiSpacesAPIPath, params, nil, token)
			if err != nil {
				return fmt.Errorf("拉取 wiki spaces 失败: %w", err)
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("HTTP %d: %s", status, string(raw))
			}
			var resp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
				Data struct {
					Items     []wikiSpaceItem `json:"items"`
					HasMore   bool            `json:"has_more"`
					PageToken string          `json:"page_token"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("解析响应失败: %w", err)
			}
			if resp.Code != 0 {
				return fmt.Errorf("飞书业务错误: code=%d msg=%s", resp.Code, resp.Msg)
			}
			spaces = append(spaces, resp.Data.Items...)
			lastHasMore = resp.Data.HasMore
			lastPageToken = resp.Data.PageToken
			if !pageAll {
				break
			}
			if !lastHasMore || lastPageToken == "" {
				break
			}
			if pageLimit > 0 && page+1 >= pageLimit {
				break
			}
			pageToken = lastPageToken
		}

		if output == "json" {
			return printJSON(map[string]any{
				"spaces":     spaces,
				"has_more":   lastHasMore,
				"page_token": lastPageToken,
				"count":      len(spaces),
			})
		}

		fmt.Printf("找到 %d 个 wiki space:\n\n", len(spaces))
		for i, s := range spaces {
			fmt.Printf("[%d] %s\n", i+1, s.Name)
			fmt.Printf("    space_id:    %s\n", s.SpaceID)
			if s.SpaceType != "" {
				fmt.Printf("    space_type:  %s\n", s.SpaceType)
			}
			if s.Visibility != "" {
				fmt.Printf("    visibility:  %s\n", s.Visibility)
			}
			if s.Description != "" {
				fmt.Printf("    description: %s\n", s.Description)
			}
		}
		if lastHasMore && lastPageToken != "" {
			fmt.Printf("\n还有更多页。继续: --page-token %s 或加 --page-all\n", lastPageToken)
		}
		return nil
	},
}

func init() {
	// space-create
	wikiCmd.AddCommand(wikiSpaceCreateCmd)
	wikiSpaceCreateCmd.Flags().String("name", "", "知识库名称（必填）")
	wikiSpaceCreateCmd.Flags().String("description", "", "知识库描述")
	wikiSpaceCreateCmd.Flags().Bool("dry-run", false, "仅打印请求，不实际创建")
	wikiSpaceCreateCmd.Flags().String("user-access-token", "", "User Access Token（必需）")
	mustMarkFlagRequired(wikiSpaceCreateCmd, "name")

	// space-list
	wikiCmd.AddCommand(wikiSpaceListCmd)
	wikiSpaceListCmd.Flags().Int("page-size", 50, "每页大小（1-50）")
	wikiSpaceListCmd.Flags().String("page-token", "", "分页起始 token")
	wikiSpaceListCmd.Flags().Bool("page-all", false, "自动翻完所有页")
	wikiSpaceListCmd.Flags().Int("page-limit", 10, "自动翻页最多翻多少页（0 = 不限）")
	wikiSpaceListCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	wikiSpaceListCmd.Flags().String("as", "auto", "身份选择: bot | user | auto")
	wikiSpaceListCmd.Flags().String("user-access-token", "", "User Access Token")

	// 顺便加 silentlyAdd: 防止 lark_compat 旧测试断言列表存在两个常量
	_ = larkcore.AccessTokenTypeUser

	_ = client.Context // 强制引用避免 unused
}
