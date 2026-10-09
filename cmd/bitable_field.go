package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// field 子命令组
var bitableFieldCmd = &cobra.Command{
	Use:   "field",
	Short: "字段管理（list/get/create/update/delete/search-options）",
}

// ------- 通用 helper -------

func bitableFieldPath(baseToken, tableID string, extra ...string) string {
	parts := []string{"bases", baseToken, "tables", tableID, "fields"}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

// ------- 子命令 -------

var bitableFieldListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部字段（自动翻页）",
	Long: `GET /fields，列出数据表的全部字段。

服务端按 offset/limit 分页且只返回 total（不传 limit 时只给 20 个字段）；
本命令按 total 自动翻页取完，输出 {"fields":[...],"total":N}。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		return runBaseV3ListAll(cmd, "fields", func(baseToken string) string {
			return bitableFieldPath(baseToken, tableID)
		})
	},
}

var bitableFieldGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取字段",
	RunE: func(cmd *cobra.Command, args []string) error {
		fieldID, _ := cmd.Flags().GetString("field-id")
		if fieldID == "" {
			return fmt.Errorf("--field-id 必填")
		}
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			tableID, _ := cmd.Flags().GetString("table-id")
			return bitableFieldPath(baseToken, tableID, fieldID)
		}, nil)
	},
}

var bitableFieldCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建字段",
	Long:  `创建字段。通过 --config/--config-file 传入 JSON 请求体。参考 base/v3 官方文档。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBaseV3WithJSON(cmd, "POST", func(baseToken string) string {
			tableID, _ := cmd.Flags().GetString("table-id")
			return bitableFieldPath(baseToken, tableID)
		})
	},
}

var bitableFieldUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新字段",
	RunE: func(cmd *cobra.Command, args []string) error {
		fieldID, _ := cmd.Flags().GetString("field-id")
		if fieldID == "" {
			return fmt.Errorf("--field-id 必填")
		}
		// 官方 base/v3 字段更新是 PUT（全量替换），不是 PATCH
		return runBaseV3WithJSON(cmd, "PUT", func(baseToken string) string {
			tableID, _ := cmd.Flags().GetString("table-id")
			return bitableFieldPath(baseToken, tableID, fieldID)
		})
	},
}

var bitableFieldDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除字段",
	RunE: func(cmd *cobra.Command, args []string) error {
		fieldID, _ := cmd.Flags().GetString("field-id")
		if fieldID == "" {
			return fmt.Errorf("--field-id 必填")
		}
		return runBaseV3Simple(cmd, "DELETE", func(baseToken string) string {
			tableID, _ := cmd.Flags().GetString("table-id")
			return bitableFieldPath(baseToken, tableID, fieldID)
		}, nil)
	},
}

var bitableFieldSearchOptionsCmd = &cobra.Command{
	Use:   "search-options",
	Short: "搜索字段选项（单选/多选字段的候选值）",
	RunE: func(cmd *cobra.Command, args []string) error {
		fieldID, _ := cmd.Flags().GetString("field-id")
		if fieldID == "" {
			return fmt.Errorf("--field-id 必填")
		}
		query, _ := cmd.Flags().GetString("query")
		offset, _ := cmd.Flags().GetInt("offset")
		limit, _ := cmd.Flags().GetInt("limit")
		params := map[string]any{}
		if query != "" {
			params["query"] = query
		}
		if offset > 0 {
			params["offset"] = offset
		}
		if limit > 0 {
			params["limit"] = limit
		}
		// 官方 base/v3 路径段是 "options" 而非 "search_options"
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			tableID, _ := cmd.Flags().GetString("table-id")
			return bitableFieldPath(baseToken, tableID, fieldID, "options")
		}, params)
	},
}

// ------- Generic runner helpers -------

// runBaseV3Simple 运行一个简单的 GET/DELETE 请求（无 body）
func runBaseV3Simple(cmd *cobra.Command, method string, pathFn func(baseToken string) string, params map[string]any) error {
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
	path := pathFn(baseToken)
	data, err := client.BaseV3Call(method, path, params, nil, token)
	if err != nil {
		return err
	}
	return printJSON(data)
}

// runBaseV3ListAll 按 offset/limit 翻页取完 table/field/view 列表，输出 {key:[...],"total":N}。
func runBaseV3ListAll(cmd *cobra.Command, key string, pathFn func(baseToken string) string) error {
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
	items, total, err := listBaseV3Items(pathFn(baseToken), key, token)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{key: items, "total": total})
}

// runBaseV3WithJSON 运行一个带 JSON body 的 POST/PUT/PATCH 请求
// 从 --config / --config-file 读取 JSON 请求体
func runBaseV3WithJSON(cmd *cobra.Command, method string, pathFn func(baseToken string) string) error {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
	if err != nil {
		return err
	}
	var body any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return fmt.Errorf("解析 --config 失败: %w", err)
	}
	return runBaseV3WithBody(cmd, method, pathFn, body)
}

// runBaseV3WithBody 运行一个带明确 body 的 POST/PUT/PATCH 请求
// 用于命令层自己构造 body 的场景（如 view create/rename 的快捷方式）
func runBaseV3WithBody(cmd *cobra.Command, method string, pathFn func(baseToken string) string, body any) error {
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
	data, err := client.BaseV3Call(method, pathFn(baseToken), nil, body, token)
	if err != nil {
		return err
	}
	return printJSON(data)
}

func init() {
	bitableCmd.AddCommand(bitableFieldCmd)

	bitableFieldCmd.AddCommand(bitableFieldListCmd)
	addBaseTokenFlag(bitableFieldListCmd)
	bitableFieldListCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldListCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldListCmd, "table-id")

	bitableFieldCmd.AddCommand(bitableFieldGetCmd)
	addBaseTokenFlag(bitableFieldGetCmd)
	bitableFieldGetCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldGetCmd.Flags().String("field-id", "", "field_id（必填）")
	bitableFieldGetCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldGetCmd, "table-id")

	bitableFieldCmd.AddCommand(bitableFieldCreateCmd)
	addBaseTokenFlag(bitableFieldCreateCmd)
	bitableFieldCreateCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldCreateCmd.Flags().String("config", "", "JSON 请求体（字段定义）")
	bitableFieldCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")
	bitableFieldCreateCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldCreateCmd, "table-id")

	bitableFieldCmd.AddCommand(bitableFieldUpdateCmd)
	addBaseTokenFlag(bitableFieldUpdateCmd)
	bitableFieldUpdateCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldUpdateCmd.Flags().String("field-id", "", "field_id（必填）")
	bitableFieldUpdateCmd.Flags().String("config", "", "JSON 请求体")
	bitableFieldUpdateCmd.Flags().String("config-file", "", "JSON 请求体文件")
	bitableFieldUpdateCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldUpdateCmd, "table-id")

	bitableFieldCmd.AddCommand(bitableFieldDeleteCmd)
	addBaseTokenFlag(bitableFieldDeleteCmd)
	bitableFieldDeleteCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldDeleteCmd.Flags().String("field-id", "", "field_id（必填）")
	bitableFieldDeleteCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldDeleteCmd, "table-id")

	bitableFieldCmd.AddCommand(bitableFieldSearchOptionsCmd)
	addBaseTokenFlag(bitableFieldSearchOptionsCmd)
	bitableFieldSearchOptionsCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableFieldSearchOptionsCmd.Flags().String("field-id", "", "field_id（必填）")
	bitableFieldSearchOptionsCmd.Flags().String("query", "", "搜索关键词")
	bitableFieldSearchOptionsCmd.Flags().Int("offset", 0, "分页 offset")
	bitableFieldSearchOptionsCmd.Flags().Int("limit", 0, "分页 limit（默认 30）")
	bitableFieldSearchOptionsCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableFieldSearchOptionsCmd, "table-id", "field-id")
}
