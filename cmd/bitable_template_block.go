package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ==================== template：多维表格模板中心（只读） ====================
// 对齐官方 base +template-categories/list/search（shortcuts/base/template_*.go），scope base:template:read：
//   GET /open-apis/base/v3/bases/templates/category
//   GET /open-apis/base/v3/bases/templates?category_key=&limit=&offset=
//   GET /open-apis/base/v3/bases/templates/search?keyword=&limit=&offset=
// 返回的 templates[].token 是模板 base_token，可直接 bitable copy --base-token <token> 建表；
// link 含 /app/ 的是应用模板（BaseApp），不能用 bitable copy 复制。

var bitableTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "多维表格模板中心（categories/list/search，只读）",
}

var bitableTemplateCategoriesCmd = &cobra.Command{
	Use:   "categories",
	Short: "列出模板中心的分类",
	Long: `GET /open-apis/base/v3/bases/templates/category

输出 categories[]，其中的 key 用于 bitable template list --category-key。scope: base:template:read

示例:
  feishu-cli bitable template categories --as user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return bitableRun(cmd, func(string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", "templates", "category")}
		})
	},
}

var bitableTemplateListCmd = &cobra.Command{
	Use:   "list",
	Short: "按分类列出模板（省略 --category-key 时为推荐分类）",
	Long: `GET /open-apis/base/v3/bases/templates

可选:
  --category-key   分类 key（来自 template categories）；省略时列推荐分类
  --limit          每页数量（1-100，默认 10）
  --offset         续翻游标（上一页输出的 offset）

输出 templates[].{name,introduction,scenarios,token,link}、has_more、offset。
用模板建表：bitable copy --base-token <templates[].token> --name "新表"（link 含 /app/ 的应用模板不能复制）。
scope: base:template:read

示例:
  feishu-cli bitable template list --category-key office --limit 10 --as user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := bitableTemplatePageParams(cmd)
		if err != nil {
			return err
		}
		if key, _ := cmd.Flags().GetString("category-key"); strings.TrimSpace(key) != "" {
			params["category_key"] = strings.TrimSpace(key)
		}
		return bitableRun(cmd, func(string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", "templates"), params: params}
		})
	},
}

var bitableTemplateSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "按关键词搜索模板",
	Long: `GET /open-apis/base/v3/bases/templates/search

必填:
  --keyword   搜索关键词（不支持空搜索）
可选:
  --limit     每页数量（1-100，默认 10）
  --offset    续翻游标（上一页输出的 offset）

输出与 template list 相同。云盘搜索（drive search）找不到模板中心的模板，用本命令。
scope: base:template:read

示例:
  feishu-cli bitable template search --keyword 项目管理 --limit 5 --as user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword, _ := cmd.Flags().GetString("keyword")
		if strings.TrimSpace(keyword) == "" {
			return clierr.Usagef("--keyword 必填（不支持空搜索）")
		}
		params, err := bitableTemplatePageParams(cmd)
		if err != nil {
			return err
		}
		params["keyword"] = strings.TrimSpace(keyword)
		return bitableRun(cmd, func(string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", "templates", "search"), params: params}
		})
	},
}

func bitableTemplatePageParams(cmd *cobra.Command) (map[string]any, error) {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > 100 {
		return nil, clierr.Usagef("--limit 范围 1-100，当前 %d", limit)
	}
	params := map[string]any{"limit": limit}
	if offset, _ := cmd.Flags().GetString("offset"); strings.TrimSpace(offset) != "" {
		params["offset"] = strings.TrimSpace(offset)
	}
	return params, nil
}

// ==================== block create/move/rename/delete：顶层块管理 ====================
// 对齐官方 base +base-block-create/move/rename/delete（shortcuts/base/base_block_ops.go）：
//   POST   /bases/{bt}/blocks                body {type,name[,parent_id]}       base:block:create
//   POST   /bases/{bt}/blocks/{id}/move      body {parent_id[,before_id|after_id]}  base:block:update
//   POST   /bases/{bt}/blocks/{id}/rename    body {name}                        base:block:update
//   DELETE /bases/{bt}/blocks/{id}                                              base:block:delete（高危，需确认）

var bitableBlockCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "新建顶层块（文件夹/数据表/文档/仪表盘/工作流）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/blocks

必填:
  --type        folder|table|docx|dashboard|workflow
  --name        块名称
可选:
  --parent-id   文件夹块 ID；省略时建在根目录

新建的数据表、文档、仪表盘、工作流的内容仍用各自的命令（table/field/record/dashboard/workflow）操作。
scope: base:block:create

示例:
  feishu-cli bitable block create --base-token bascnxxx --type folder --name "项目文档"
  feishu-cli bitable block create --base-token bascnxxx --type table --name "任务" --parent-id <folder_block_id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockType, _ := cmd.Flags().GetString("type")
		blockType = strings.TrimSpace(blockType)
		name, _ := cmd.Flags().GetString("name")
		name = strings.TrimSpace(name)
		if blockType == "" {
			return clierr.Usagef("--type 必填")
		}
		if err := validateEnum(blockType, " --type", bitableBlockTypes); err != nil {
			return err
		}
		if name == "" {
			return clierr.Usagef("--name 必填")
		}
		body := map[string]any{"type": blockType, "name": name}
		if parentID, _ := cmd.Flags().GetString("parent-id"); strings.TrimSpace(parentID) != "" {
			body["parent_id"] = strings.TrimSpace(parentID)
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "blocks"), body: body}
		})
	},
}

var bitableBlockMoveCmd = &cobra.Command{
	Use:   "move",
	Short: "移动顶层块（换文件夹或调整顺序）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/blocks/{block_id}/move

必填:
  --block-id    要移动的块
可选:
  --parent-id   目标文件夹块 ID；省略时移到根目录
  --before-id   移到该兄弟块之前
  --after-id    移到该兄弟块之后（与 --before-id 互斥）

移动文件夹时其子块随之移动。scope: base:block:update

示例:
  feishu-cli bitable block move --base-token bascnxxx --block-id <block_id> --parent-id <folder_block_id>
  feishu-cli bitable block move --base-token bascnxxx --block-id <block_id> --after-id <sibling_block_id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockID, err := requireBitableBlockID(cmd)
		if err != nil {
			return err
		}
		parentID, _ := cmd.Flags().GetString("parent-id")
		beforeID, _ := cmd.Flags().GetString("before-id")
		afterID, _ := cmd.Flags().GetString("after-id")
		parentID, beforeID, afterID = strings.TrimSpace(parentID), strings.TrimSpace(beforeID), strings.TrimSpace(afterID)
		if beforeID != "" && afterID != "" {
			return clierr.Usagef("--before-id 与 --after-id 互斥")
		}
		// parent_id 省略时显式传 null：服务端据此移到根目录（对齐官方）
		body := map[string]any{"parent_id": nil}
		if parentID != "" {
			body["parent_id"] = parentID
		}
		if beforeID != "" {
			body["before_id"] = beforeID
		}
		if afterID != "" {
			body["after_id"] = afterID
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "blocks", blockID, "move"), body: body}
		})
	},
}

var bitableBlockRenameCmd = &cobra.Command{
	Use:   "rename",
	Short: "重命名顶层块",
	Long: `POST /open-apis/base/v3/bases/{base_token}/blocks/{block_id}/rename  body {"name":"..."}

scope: base:block:update

示例:
  feishu-cli bitable block rename --base-token bascnxxx --block-id <block_id> --name "新名称"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockID, err := requireBitableBlockID(cmd)
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		if strings.TrimSpace(name) == "" {
			return clierr.Usagef("--name 必填")
		}
		body := map[string]any{"name": strings.TrimSpace(name)}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "blocks", blockID, "rename"), body: body}
		})
	},
}

var bitableBlockDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除顶层块（高危，需确认）",
	Long: `DELETE /open-apis/base/v3/bases/{base_token}/blocks/{block_id}

不支持递归删除文件夹：非空文件夹先移走或删除其子块。不同类型的块背后可能有独立资源（如数据表的数据），
删除语义以服务端为准。先用 bitable block list 确认 block_id。

交互终端会二次确认；非交互环境必须加 --yes，否则 exit 10、不执行。scope: base:block:delete

示例:
  feishu-cli bitable block delete --base-token bascnxxx --block-id <block_id> --dry-run
  feishu-cli bitable block delete --base-token bascnxxx --block-id <block_id> --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockID, err := requireBitableBlockID(cmd)
		if err != nil {
			return err
		}
		return bitableExecuteConfirmed(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: client.BaseV3Path("bases", bt, "blocks", blockID)}
		}, func(bitableReq) string {
			return fmt.Sprintf("确认删除多维表格顶层块 %s？", blockID)
		}, nil)
	},
}

func requireBitableBlockID(cmd *cobra.Command) (string, error) {
	blockID, _ := cmd.Flags().GetString("block-id")
	blockID = strings.TrimSpace(blockID)
	if blockID == "" {
		return "", clierr.Usagef("--block-id 必填")
	}
	return blockID, nil
}

func init() {
	bitableCmd.AddCommand(bitableTemplateCmd)
	for _, c := range []*cobra.Command{bitableTemplateCategoriesCmd, bitableTemplateListCmd, bitableTemplateSearchCmd} {
		bitableTemplateCmd.AddCommand(c)
		c.Flags().String("user-access-token", "", "User Access Token")
		addBitablePreviewFlags(c)
	}
	for _, c := range []*cobra.Command{bitableTemplateListCmd, bitableTemplateSearchCmd} {
		c.Flags().Int("limit", 10, "每页数量（1-100）")
		c.Flags().String("offset", "", "续翻游标（上一页输出的 offset）")
	}
	bitableTemplateListCmd.Flags().String("category-key", "", "分类 key（来自 template categories；省略为推荐分类）")
	bitableTemplateSearchCmd.Flags().String("keyword", "", "搜索关键词（必填）")
	mustMarkFlagRequired(bitableTemplateSearchCmd, "keyword")

	for _, c := range []*cobra.Command{bitableBlockCreateCmd, bitableBlockMoveCmd, bitableBlockRenameCmd, bitableBlockDeleteCmd} {
		bitableBlockCmd.AddCommand(c)
		addBitableWriteFlags(c)
	}
	bitableBlockCreateCmd.Flags().String("type", "", "块类型: folder|table|docx|dashboard|workflow（必填）")
	bitableBlockCreateCmd.Flags().String("name", "", "块名称（必填）")
	bitableBlockCreateCmd.Flags().String("parent-id", "", "文件夹块 ID（省略为根目录）")
	mustMarkFlagRequired(bitableBlockCreateCmd, "type", "name")
	for _, c := range []*cobra.Command{bitableBlockMoveCmd, bitableBlockRenameCmd, bitableBlockDeleteCmd} {
		c.Flags().String("block-id", "", "块 ID（必填）")
		mustMarkFlagRequired(c, "block-id")
	}
	bitableBlockMoveCmd.Flags().String("parent-id", "", "目标文件夹块 ID（省略为根目录）")
	bitableBlockMoveCmd.Flags().String("before-id", "", "移到该兄弟块之前")
	bitableBlockMoveCmd.Flags().String("after-id", "", "移到该兄弟块之后（与 --before-id 互斥）")
	bitableBlockRenameCmd.Flags().String("name", "", "新名称（必填）")
	mustMarkFlagRequired(bitableBlockRenameCmd, "name")
}
