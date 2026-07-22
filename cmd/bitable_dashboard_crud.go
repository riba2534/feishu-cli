package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
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

// dashboardBlockType 合法块类型。
var dashboardBlockTypes = []string{
	"column", "bar", "line", "pie", "ring", "area", "combo",
	"scatter", "funnel", "wordCloud", "radar", "statistics", "text",
}

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
	Short: "仪表盘块管理（create/get/list/update/delete）",
}

var bitableDashboardBlockCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建仪表盘块",
	Long: `POST /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks

便捷字段:
  --name          块名称
  --type          块类型: column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|statistics|text（必填）
  --data-config   数据配置 JSON 对象（图表: table_name/series|count_all/group_by/filter；文本: text）

或用 --config/--config-file 传完整 JSON 请求体（与便捷字段二选一）。`,
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
			return fmt.Errorf("未提供任何更新字段（用 --name/--data-config 或 --config）")
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
	}
	if cmd.Flags().Changed("data-config") {
		raw, _ := cmd.Flags().GetString("data-config")
		var dataConfig any
		if err := json.Unmarshal([]byte(raw), &dataConfig); err != nil {
			return nil, fmt.Errorf("解析 --data-config 失败: %w", err)
		}
		body["data_config"] = dataConfig
	}
	return body, nil
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
	bitableDashboardBlockCreateCmd.Flags().String("type", "", "块类型: column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|statistics|text（必填）")
	bitableDashboardBlockCreateCmd.Flags().String("data-config", "", "数据配置 JSON 对象")
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
	bitableDashboardBlockUpdateCmd.Flags().String("config", "", "完整 JSON 请求体（与便捷字段二选一）")
	bitableDashboardBlockUpdateCmd.Flags().String("config-file", "", "JSON 请求体文件")

	// block delete
	bitableDashboardBlockCmd.AddCommand(bitableDashboardBlockDeleteCmd)
	addBitableWriteFlags(bitableDashboardBlockDeleteCmd)
	bitableDashboardBlockDeleteCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardBlockDeleteCmd.Flags().String("block-id", "", "块 ID（必填）")
}
