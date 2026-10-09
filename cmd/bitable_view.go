package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// view 子命令组（基础 + 6 种配置的 get/set）
var bitableViewCmd = &cobra.Command{
	Use:   "view",
	Short: "视图管理（list/get/create/delete/rename + 6 种配置的 get/set）",
}

func bitableViewPath(baseToken, tableID string, extra ...string) string {
	parts := []string{"bases", baseToken, "tables", tableID, "views"}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

// ---- 基础 5 命令 ----

var bitableViewListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部视图（自动翻页）",
	Long: `GET /views，列出数据表的全部视图（含表单视图）。

服务端按 offset/limit 分页且只返回 total（不传 limit 时只给 20 个）；
本命令按 total 自动翻页取完，输出 {"views":[...],"total":N}。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		return runBaseV3ListAll(cmd, "views", func(bt string) string {
			return bitableViewPath(bt, tableID)
		})
	},
}

var bitableViewGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取视图",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		viewID, _ := cmd.Flags().GetString("view-id")
		if viewID == "" {
			return fmt.Errorf("--view-id 必填")
		}
		return runBaseV3Simple(cmd, "GET", func(bt string) string {
			return bitableViewPath(bt, tableID, viewID)
		}, nil)
	},
}

var bitableViewCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建视图",
	Long:  `通过 --config 传入完整 view 定义（name/view_type 等），或使用 --name + --view-type 快捷方式`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		name, _ := cmd.Flags().GetString("name")
		viewType, _ := cmd.Flags().GetString("view-type")
		configJSON, _ := cmd.Flags().GetString("config")
		configFile, _ := cmd.Flags().GetString("config-file")

		pathFn := func(bt string) string { return bitableViewPath(bt, tableID) }

		if configJSON != "" || configFile != "" {
			return runBaseV3WithJSON(cmd, "POST", pathFn)
		}
		if name == "" {
			return fmt.Errorf("需要 --name 或 --config 至少一个")
		}
		// 官方 base/v3 body 顶层直接是 {name, type}，不包 "view" 一层
		body := map[string]any{"name": name, "type": viewType}
		return runBaseV3WithBody(cmd, "POST", pathFn, body)
	},
}

var bitableViewDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除视图",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		viewID, _ := cmd.Flags().GetString("view-id")
		if viewID == "" {
			return fmt.Errorf("--view-id 必填")
		}
		return runBaseV3Simple(cmd, "DELETE", func(bt string) string {
			return bitableViewPath(bt, tableID, viewID)
		}, nil)
	},
}

var bitableViewRenameCmd = &cobra.Command{
	Use:   "rename",
	Short: "重命名视图",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		viewID, _ := cmd.Flags().GetString("view-id")
		name, _ := cmd.Flags().GetString("name")
		if viewID == "" || name == "" {
			return fmt.Errorf("--view-id 和 --name 必填")
		}
		body := map[string]any{"name": name}
		return runBaseV3WithBody(cmd, "PATCH", func(bt string) string {
			return bitableViewPath(bt, tableID, viewID)
		}, body)
	},
}

// ---- 视图配置 get/set（6 种 × 2 = 12 命令）----
// 官方 base/v3 路径段是简写形式（filter/sort/group/visible_fields/timebar/card），
// set 方法用 PUT（全量替换），不是 PATCH。
// sort/group 的 body 会自动包装为 {sort_config: [...]} / {group_config: [...]}。
// group/sort/visible_fields 的读写响应 data 是数组，统一走 BaseV3CallAny 解包后输出
// （BaseV3Call 遇到非对象 data 会回落成整个 {"code","data","msg"} 信封）。

// viewConfigSuffixes CLI 子命令 kind → 官方 base/v3 API 路径段
var viewConfigSuffixes = map[string]string{
	"filter":         "filter",
	"sort":           "sort",
	"group":          "group",
	"visible-fields": "visible_fields",
	"timebar":        "timebar",
	"card":           "card",
}

// viewConfigWrapKey 某些 set 命令需要把用户传的数组自动包装成 {"<key>": [...]}
// 避免每次都让用户手写外层 key。key 名称来自官方 base/v3 API。
var viewConfigWrapKey = map[string]string{
	"sort":           "sort_config",
	"group":          "group_config",
	"visible-fields": "visible_fields",
}

// viewConfigSetHelp 各配置 set 的 v3 请求体说明（实测）。
var viewConfigSetHelp = map[string]string{
	"filter": `请求体（与 record list --filter-json 同一套 tuple DSL）:
  {"logic":"and","conditions":[["状态","intersects",["进行中"]],["截止","empty"]]}
  清空: {"conditions":[]}
支持视图: grid / kanban / gallery / calendar / gantt`,
	"sort": `请求体: {"sort_config":[{"field":"截止时间","desc":false}]}（最多 10 条，空数组清除）
也可直接传数组 [{"field":"截止时间","desc":false}]，自动包成 {"sort_config":[...]}`,
	"group": `请求体: {"group_config":[{"field":"状态","desc":false}]}（最多 3 条，空数组清除）
也可直接传数组，自动包成 {"group_config":[...]}；支持视图: grid / kanban / gantt`,
	"visible-fields": `请求体: {"visible_fields":["任务名称","负责人","截止时间"]}（字段名或 ID 的有序完整列表，
未列出的字段被隐藏，不删除数据）；也可直接传数组，自动包成 {"visible_fields":[...]}`,
	"timebar": `请求体: {"start_time":"开始时间","end_time":"结束时间","title":"任务名称"}
仅 gantt / calendar 视图支持`,
	"card": `请求体: {"cover_field":"产品图片"}（附件字段；null 清除封面）
仅 gallery / kanban 视图支持`,
}

func newViewConfigCmd(kind, action string) *cobra.Command {
	suffix := viewConfigSuffixes[kind]
	fullName := fmt.Sprintf("view-%s", kind)
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s-%s", fullName, action),
		Short: fmt.Sprintf("%s 视图配置：%s", action, fullName),
	}
	switch action {
	case "get":
		cmd.Long = fmt.Sprintf("GET /views/{view_id}/%s，读取视图 %s 配置（输出 data 本体，group/sort/visible-fields 为数组）。", suffix, kind)
		cmd.RunE = func(cc *cobra.Command, _ []string) error {
			tableID, _ := cc.Flags().GetString("table-id")
			viewID, _ := cc.Flags().GetString("view-id")
			if viewID == "" {
				return fmt.Errorf("--view-id 必填")
			}
			return runViewConfigCall(cc, "GET", func(bt string) string {
				return bitableViewPath(bt, tableID, viewID, suffix)
			}, nil)
		}
	case "set":
		cmd.Long = fmt.Sprintf("PUT /views/{view_id}/%s，全量替换视图 %s 配置（base/v3 结构，实测）。\n\n%s", suffix, kind, viewConfigSetHelp[kind])
		cmd.RunE = func(cc *cobra.Command, _ []string) error {
			tableID, _ := cc.Flags().GetString("table-id")
			viewID, _ := cc.Flags().GetString("view-id")
			if viewID == "" {
				return fmt.Errorf("--view-id 必填")
			}
			configJSON, _ := cc.Flags().GetString("config")
			configFile, _ := cc.Flags().GetString("config-file")
			raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
			if err != nil {
				return err
			}
			var parsed any
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				return fmt.Errorf("解析 --config 失败: %w", err)
			}
			// 用户既可能传数组（[]）也可能传已经包装好的对象（{wrapKey:[...]}）
			if wrapKey, ok := viewConfigWrapKey[kind]; ok {
				if _, isObject := parsed.(map[string]any); !isObject {
					parsed = map[string]any{wrapKey: parsed}
				}
			}
			if kind == "sort" {
				if m, ok := parsed.(map[string]any); ok {
					if arr, ok := m["sort_config"].([]any); ok && len(arr) > maxRecordSortItems {
						return clierr.Usagef("sort_config 最多 %d 条，当前 %d 条", maxRecordSortItems, len(arr))
					}
				}
			}
			// 官方 base/v3 set 方法是 PUT，不是 PATCH
			return runViewConfigCall(cc, "PUT", func(bt string) string {
				return bitableViewPath(bt, tableID, viewID, suffix)
			}, parsed)
		}
	}
	return cmd
}

// runViewConfigCall 调用视图配置端点并输出解包后的 data（可能是对象或数组）。
func runViewConfigCall(cmd *cobra.Command, method string, pathFn func(baseToken string) string, body any) error {
	if err := config.Validate(); err != nil {
		return err
	}
	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	baseToken, err := resolveBaseToken(cmd)
	if err != nil {
		return err
	}
	data, err := client.BaseV3CallAny(method, pathFn(baseToken), nil, body, token)
	if err != nil {
		return err
	}
	return printJSON(data)
}

func init() {
	bitableCmd.AddCommand(bitableViewCmd)

	// 5 个基础命令
	for _, c := range []*cobra.Command{
		bitableViewListCmd, bitableViewGetCmd, bitableViewCreateCmd,
		bitableViewDeleteCmd, bitableViewRenameCmd,
	} {
		bitableViewCmd.AddCommand(c)
		addBaseTokenFlag(c)
		c.Flags().String("table-id", "", "table_id（必填）")
		c.Flags().String("user-access-token", "", "User Access Token")
		mustMarkFlagRequired(c, "table-id")
	}
	bitableViewGetCmd.Flags().String("view-id", "", "view_id（必填）")
	bitableViewCreateCmd.Flags().String("name", "", "视图名称")
	bitableViewCreateCmd.Flags().String("view-type", "grid", "视图类型: grid/kanban/gallery/gantt/calendar")
	bitableViewCreateCmd.Flags().String("config", "", "完整 view JSON 配置（可选）")
	bitableViewCreateCmd.Flags().String("config-file", "", "完整 view JSON 配置文件")
	bitableViewDeleteCmd.Flags().String("view-id", "", "view_id（必填）")
	bitableViewRenameCmd.Flags().String("view-id", "", "view_id（必填）")
	bitableViewRenameCmd.Flags().String("name", "", "新名称（必填）")

	// 12 个视图配置命令（6 种 × get/set）
	kinds := []string{"filter", "sort", "group", "visible-fields", "timebar", "card"}
	for _, kind := range kinds {
		getCmd := newViewConfigCmd(kind, "get")
		setCmd := newViewConfigCmd(kind, "set")
		for _, c := range []*cobra.Command{getCmd, setCmd} {
			bitableViewCmd.AddCommand(c)
			addBaseTokenFlag(c)
			c.Flags().String("table-id", "", "table_id（必填）")
			c.Flags().String("view-id", "", "view_id（必填）")
			c.Flags().String("user-access-token", "", "User Access Token")
			mustMarkFlagRequired(c, "table-id", "view-id")
		}
		setCmd.Flags().String("config", "", "JSON 配置（必填）")
		setCmd.Flags().String("config-file", "", "JSON 配置文件")
	}
}
