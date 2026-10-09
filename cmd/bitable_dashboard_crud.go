package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// ==================== dashboard CRUD + arrange ====================
// 端点全部为 base/v3（useV1:false，带 X-App-Id），与现有 dashboard list 一致；
// dashboard copy（bitable_dashboard.go）是唯一走 bitable/v1 的特例。
// 端点 ground truth：已逐个实测印证（见 PR 报告 spec 表）。

// dashboardPath 构造 base/v3 仪表盘路径。
func dashboardPath(baseToken, dashboardID string, extra ...string) string {
	parts := []string{"bases", baseToken, "dashboards"}
	if dashboardID != "" {
		parts = append(parts, dashboardID)
	}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

// dashboardBlockTypes 合法块类型（对齐官方 +dashboard-block-create：含 ranking 排行榜、nps NPS 图）。
var dashboardBlockTypes = []string{
	"column", "bar", "line", "pie", "ring", "area", "combo",
	"scatter", "funnel", "wordCloud", "radar", "ranking", "statistics", "nps", "text",
}

const dashboardBlockTypesHelp = "column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|ranking|statistics|nps|text"

var bitableDashboardCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建仪表盘",
	Long: `POST /open-apis/base/v3/bases/{base_token}/dashboards

便捷字段:
  --name          仪表盘名称
  --theme-style   主题风格（写入 body.theme.theme_style）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := dashboardBuildCreateOrUpdateBody(cmd)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("未提供任何字段（用 --name/--theme-style 或 --config）")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: dashboardPath(bt, ""), body: body}
		})
	},
}

var bitableDashboardGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取仪表盘",
	Long:  `GET /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: dashboardPath(bt, dashboardID)}
		})
	},
}

var bitableDashboardUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新仪表盘",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}

便捷字段（仅显式设置的才提交）:
  --name          新名称
  --theme-style   主题风格（写入 body.theme.theme_style）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		body, err := dashboardBuildCreateOrUpdateBody(cmd)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("未提供任何更新字段（用 --name/--theme-style 或 --config）")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PATCH", path: dashboardPath(bt, dashboardID), body: body}
		})
	},
}

var bitableDashboardDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除仪表盘",
	Long:  `DELETE /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: dashboardPath(bt, dashboardID)}
		})
	},
}

var bitableDashboardArrangeCmd = &cobra.Command{
	Use:   "arrange",
	Short: "自动排版仪表盘块（服务端智能布局）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/arrange

服务端智能布局，无请求体（实测 body 为空对象）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: dashboardPath(bt, dashboardID, "arrange"), body: map[string]any{}}
		})
	},
}

// dashboardBuildCreateOrUpdateBody 构造 dashboard create/update 请求体。
// 优先 --config/--config-file；否则从便捷 flag 收集（只取显式设置的）。
// --theme-style 写入嵌套 body.theme.theme_style（实测结构）。
func dashboardBuildCreateOrUpdateBody(cmd *cobra.Command) (map[string]any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	if configJSON != "" || configFile != "" {
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, fmt.Errorf("解析 --config 失败: %w", err)
		}
		return body, nil
	}

	body := map[string]any{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		body["name"] = v
	}
	if cmd.Flags().Changed("theme-style") {
		v, _ := cmd.Flags().GetString("theme-style")
		body["theme"] = map[string]any{"theme_style": v}
	}
	return body, nil
}

// ==================== dashboard block ====================
// 块路径在仪表盘下：.../dashboards/{dashboard_id}/blocks[/{block_id}]。

var bitableDashboardBlockCmd = &cobra.Command{
	Use:   "block",
	Short: "仪表盘块管理（create/get/get-data/list/update/delete）",
}

var bitableDashboardBlockCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建仪表盘块",
	Long: `POST /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks

便捷字段:
  --name          块名称
  --type          块类型: column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|ranking|statistics|nps|text（必填）
  --data-config   数据配置 JSON 对象（图表: table_name/series|count_all/group_by/filter；文本: text）；
                  ranking / nps 必须提供（nps 需 table_name 与 group_by）
  --position      可选，12 列栅格中的位置与大小 {"x":0,"y":0,"w":6,"h":4}，x/y/w/h 必须同时给出且为数字；
                  省略则由服务端自动布局（越界或重叠的坐标会被服务端自动调整）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。

示例:
  feishu-cli bitable dashboard block create --base-token <bt> --dashboard-id <did> --name "订单数" \
    --type statistics --data-config '{"table_name":"订单","count_all":true}' --position '{"x":0,"y":0,"w":6,"h":4}'
  feishu-cli bitable dashboard block create --base-token <bt> --dashboard-id <did> --name "Top 负责人" \
    --type ranking --data-config '{"table_name":"订单","group_by":[{"field_name":"负责人"}],"series":[{"field_name":"金额","rollup":"SUM"}]}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		body, err := dashboardBuildBlockBody(cmd, true)
		if err != nil {
			return err
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: dashboardPath(bt, dashboardID, "blocks"), body: body}
		})
	},
}

var bitableDashboardBlockGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取仪表盘块",
	Long:  `GET /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks/{block_id}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		blockID, _ := cmd.Flags().GetString("block-id")
		if dashboardID == "" || blockID == "" {
			return fmt.Errorf("--dashboard-id 和 --block-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: dashboardPath(bt, dashboardID, "blocks", blockID)}
		})
	},
}

var bitableDashboardBlockListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出仪表盘块",
	Long: `GET /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks

可选:
  --page-size    分页大小（≤100）
  --page-token   下一页 token`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		params := map[string]any{}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: dashboardPath(bt, dashboardID, "blocks"), params: params}
		})
	},
}

var bitableDashboardBlockUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新仪表盘块",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks/{block_id}

便捷字段（仅显式设置的才提交）:
  --name          新块名称
  --data-config   数据配置 JSON（图表/文本）
  --position      新位置与大小 {"x","y","w","h"}（整体提交，四个键必须同时给出）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		blockID, _ := cmd.Flags().GetString("block-id")
		if dashboardID == "" || blockID == "" {
			return fmt.Errorf("--dashboard-id 和 --block-id 必填")
		}
		body, err := dashboardBuildBlockBody(cmd, false)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return fmt.Errorf("未提供任何更新字段（用 --name/--data-config/--position 或 --config）")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "PATCH", path: dashboardPath(bt, dashboardID, "blocks", blockID), body: body}
		})
	},
}

var bitableDashboardBlockDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除仪表盘块",
	Long:  `DELETE /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks/{block_id}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		blockID, _ := cmd.Flags().GetString("block-id")
		if dashboardID == "" || blockID == "" {
			return fmt.Errorf("--dashboard-id 和 --block-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: dashboardPath(bt, dashboardID, "blocks", blockID)}
		})
	},
}

// dashboardBuildBlockBody 构造 block create/update 请求体。
// 优先 --config/--config-file；否则用便捷 flag。
// createMode=true 时 --type 必填并校验枚举（create 需要类型；update 不允许改类型）。
func dashboardBuildBlockBody(cmd *cobra.Command, createMode bool) (map[string]any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	if configJSON != "" || configFile != "" {
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, fmt.Errorf("解析 --config 失败: %w", err)
		}
		return body, nil
	}

	body := map[string]any{}
	if cmd.Flags().Changed("name") {
		v, _ := cmd.Flags().GetString("name")
		body["name"] = v
	}
	if createMode {
		blockType, _ := cmd.Flags().GetString("type")
		if blockType == "" {
			return nil, fmt.Errorf("--type 必填（或用 --config 传完整请求体）")
		}
		if err := validateEnum(blockType, "type", dashboardBlockTypes); err != nil {
			return nil, err
		}
		body["type"] = blockType
		if (blockType == "ranking" || blockType == "nps") && !cmd.Flags().Changed("data-config") {
			return nil, clierr.Usagef("%s 类型组件必须提供 --data-config（nps 需包含 table_name 与 group_by）", blockType)
		}
	}
	if cmd.Flags().Changed("data-config") {
		raw, _ := cmd.Flags().GetString("data-config")
		var dataConfig any
		if err := json.Unmarshal([]byte(raw), &dataConfig); err != nil {
			return nil, fmt.Errorf("解析 --data-config 失败: %w", err)
		}
		body["data_config"] = dataConfig
	}
	if cmd.Flags().Changed("position") {
		raw, _ := cmd.Flags().GetString("position")
		pos, err := parseDashboardBlockPosition(raw)
		if err != nil {
			return nil, err
		}
		body["position"] = pos
	}
	return body, nil
}

// parseDashboardBlockPosition 解析 --position：JSON 对象且 x/y/w/h 四个键都是数字
// （position 按整体提交，残缺对象无法表达完整位置；坐标取值不做本地校验，交给服务端自动调整）。
func parseDashboardBlockPosition(raw string) (map[string]any, error) {
	var pos map[string]any
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	dec.UseNumber()
	if err := dec.Decode(&pos); err != nil || pos == nil {
		return nil, clierr.Usagef(`--position 需要 JSON 对象，如 {"x":0,"y":0,"w":6,"h":4}`)
	}
	var missing []string
	for _, key := range []string{"x", "y", "w", "h"} {
		if _, ok := pos[key].(json.Number); !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, clierr.Usagef("--position 的 %s 缺失或不是数字；x/y/w/h 必须同时提供且为数值", strings.Join(missing, "/"))
	}
	return pos, nil
}

var bitableDashboardBlockGetDataCmd = &cobra.Command{
	Use:   "get-data",
	Short: "读取仪表盘图表块的计算结果数据",
	Long: `GET /open-apis/base/v3/bases/{base_token}/dashboards/blocks/{block_id}/data

返回图表块计算后的数据（图表协议 JSON），不需要 --dashboard-id。
块的名称、类型、data_config 用 dashboard block get 读取；文本块内容在 data_config.text。
不支持计算数据的图表类型，可按其 data_config 用 data-query 以相同的表、维度、度量和筛选查询。

示例:
  feishu-cli bitable dashboard block get-data --base-token <bt> --block-id <block_id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockID, _ := cmd.Flags().GetString("block-id")
		if strings.TrimSpace(blockID) == "" {
			return clierr.Usagef("--block-id 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", bt, "dashboards", "blocks", blockID, "data")}
		})
	},
}

func init() {
	// dashboard create
	bitableDashboardCmd.AddCommand(bitableDashboardCreateCmd)
	addBitableWriteFlags(bitableDashboardCreateCmd)
	bitableDashboardCreateCmd.Flags().String("name", "", "仪表盘名称")
	bitableDashboardCreateCmd.Flags().String("theme-style", "", "主题风格（写入 theme.theme_style）")
	bitableDashboardCreateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableDashboardCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// dashboard get
	bitableDashboardCmd.AddCommand(bitableDashboardGetCmd)
	addBitableCommonFlags(bitableDashboardGetCmd)
	bitableDashboardGetCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")

	// dashboard update
	bitableDashboardCmd.AddCommand(bitableDashboardUpdateCmd)
	addBitableWriteFlags(bitableDashboardUpdateCmd)
	bitableDashboardUpdateCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardUpdateCmd.Flags().String("name", "", "新名称")
	bitableDashboardUpdateCmd.Flags().String("theme-style", "", "主题风格（写入 theme.theme_style）")
	bitableDashboardUpdateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableDashboardUpdateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// dashboard delete
	bitableDashboardCmd.AddCommand(bitableDashboardDeleteCmd)
	addBitableWriteFlags(bitableDashboardDeleteCmd)
	bitableDashboardDeleteCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")

	// dashboard arrange
	bitableDashboardCmd.AddCommand(bitableDashboardArrangeCmd)
	addBitableWriteFlags(bitableDashboardArrangeCmd)
	bitableDashboardArrangeCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")

	// dashboard block
	bitableDashboardCmd.AddCommand(bitableDashboardBlockCmd)

	// block create
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockCreateCmd)
	addBitableWriteFlags(bitableDashboardBlockCreateCmd)
	bitableDashboardBlockCreateCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockCreateCmd.Flags().String("name", "", "块名称")
	bitableDashboardBlockCreateCmd.Flags().String("type", "", "块类型: "+dashboardBlockTypesHelp+"（必填）")
	bitableDashboardBlockCreateCmd.Flags().String("data-config", "", "数据配置 JSON 对象")
	bitableDashboardBlockCreateCmd.Flags().String("position", "", `位置与大小 JSON {"x","y","w","h"}（可选，省略则自动布局）`)
	bitableDashboardBlockCreateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableDashboardBlockCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// block get
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockGetCmd)
	addBitableCommonFlags(bitableDashboardBlockGetCmd)
	bitableDashboardBlockGetCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockGetCmd.Flags().String("block-id", "", "块 ID（必填）")

	// block list
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockListCmd)
	addBitableCommonFlags(bitableDashboardBlockListCmd)
	bitableDashboardBlockListCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockListCmd.Flags().Int("page-size", 0, "分页大小（≤100）")
	bitableDashboardBlockListCmd.Flags().String("page-token", "", "分页 token")

	// block update
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockUpdateCmd)
	addBitableWriteFlags(bitableDashboardBlockUpdateCmd)
	bitableDashboardBlockUpdateCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockUpdateCmd.Flags().String("block-id", "", "块 ID（必填）")
	bitableDashboardBlockUpdateCmd.Flags().String("name", "", "新块名称")
	bitableDashboardBlockUpdateCmd.Flags().String("data-config", "", "数据配置 JSON")
	bitableDashboardBlockUpdateCmd.Flags().String("position", "", `新位置与大小 JSON {"x","y","w","h"}`)

	// block get-data（读取计算结果，不需要 dashboard-id）
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockGetDataCmd)
	addBitableCommonFlags(bitableDashboardBlockGetDataCmd)
	output.AddDryRunFlag(bitableDashboardBlockGetDataCmd)
	bitableDashboardBlockGetDataCmd.Flags().String("block-id", "", "图表块 ID（必填）")
	bitableDashboardBlockGetDataCmd.Flags().String("dashboard-id", "", "兼容参数，get-data 不需要")
	_ = bitableDashboardBlockGetDataCmd.Flags().MarkHidden("dashboard-id")
	bitableDashboardBlockUpdateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableDashboardBlockUpdateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// block delete
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockDeleteCmd)
	addBitableWriteFlags(bitableDashboardBlockDeleteCmd)
	bitableDashboardBlockDeleteCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockDeleteCmd.Flags().String("block-id", "", "块 ID（必填）")
}
