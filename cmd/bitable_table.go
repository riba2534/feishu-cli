package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var bitableTableCmd = &cobra.Command{
	Use:   "table",
	Short: "数据表管理（list/get/create/update/delete）",
}

func bitableTablePath(baseToken string, extra ...string) string {
	parts := []string{"bases", baseToken, "tables"}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

var bitableTableListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部数据表（自动翻页）",
	Long: `GET /tables，列出多维表格中的数据表。

默认按 total 自动翻页取完，输出 {"tables":[...],"total":N}（服务端不传 limit 时只给 20 张）。
显式传 --offset / --limit 时只取单页；还有剩余时输出 has_more/next_offset 并在 stderr 提示。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		offset, _ := cmd.Flags().GetInt("offset")
		limit, _ := cmd.Flags().GetInt("limit")
		if !cmd.Flags().Changed("offset") && !cmd.Flags().Changed("limit") {
			return runBaseV3ListAll(cmd, "tables", func(bt string) string {
				return bitableTablePath(bt)
			})
		}
		if offset < 0 {
			return clierr.Usagef("--offset 不能为负数，当前 %d", offset)
		}
		if limit < 0 || limit > 500 {
			return clierr.Usagef("--limit 范围 1-500，当前 %d", limit)
		}
		params := map[string]any{}
		if offset > 0 {
			params["offset"] = offset
		}
		if limit > 0 {
			params["limit"] = limit
		}
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
		data, err := client.BaseV3Call("GET", bitableTablePath(baseToken), params, nil, token)
		if err != nil {
			return err
		}
		got := len(bitableAnySlice(data["tables"]))
		if total := bitableJSONInt(data["total"]); total > offset+got && got > 0 {
			next := offset + got
			data["has_more"] = true
			data["next_offset"] = next
			fmt.Fprintf(os.Stderr, "提示: 共 %d 张数据表，本页 %d 张；续翻加 --offset %d，或去掉 --offset/--limit 自动取全部\n", total, got, next)
		}
		return printJSON(data)
	},
}

var bitableTableGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取数据表信息",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
		}
		return runBaseV3Simple(cmd, "GET", func(bt string) string {
			return bitableTablePath(bt, tableID)
		}, nil)
	},
}

var bitableTableCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建数据表（可用 --fields 一次带上字段 schema）",
	Long: `创建新数据表。

  --name          数据表名称
  --fields        字段 JSON 数组（与 field create 的字段 JSON 同形，判别式是顶层 type），
                  建表时一次创建全部字段，如
                  '[{"name":"标题","type":"text"},{"name":"状态","type":"select","options":[{"name":"Todo"}]}]'
  --config/--config-file  完整 table 请求体（{"name":...,"fields":[...]}，与上面两个 flag 二选一）

输出新表对象（id/name/fields/views）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		fieldsJSON, _ := cmd.Flags().GetString("fields")
		configJSON, _ := cmd.Flags().GetString("config")
		configFile, _ := cmd.Flags().GetString("config-file")

		pathFn := func(bt string) string { return bitableTablePath(bt) }

		if configJSON != "" || configFile != "" {
			if fieldsJSON != "" {
				return clierr.Usagef("--fields 与 --config/--config-file 互斥，字段请写进 --config 的 fields 数组")
			}
			return runBaseV3WithJSON(cmd, "POST", pathFn)
		}
		if name == "" {
			return clierr.Usagef("需要 --name 或 --config/--config-file 至少一个")
		}
		// 官方 base/v3 body 顶层直接是字段，不包 "table" 一层
		body := map[string]any{"name": name}
		if fieldsJSON != "" {
			fields, err := parseBitableFieldsArray(fieldsJSON, "--fields")
			if err != nil {
				return err
			}
			body["fields"] = fields
		}
		return runBaseV3WithBody(cmd, "POST", pathFn, body)
	},
}

var bitableTableUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新数据表",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		name, _ := cmd.Flags().GetString("name")
		configJSON, _ := cmd.Flags().GetString("config")
		configFile, _ := cmd.Flags().GetString("config-file")

		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
		}

		pathFn := func(bt string) string { return bitableTablePath(bt, tableID) }

		if configJSON != "" || configFile != "" {
			return runBaseV3WithJSON(cmd, "PATCH", pathFn)
		}
		if name == "" {
			return fmt.Errorf("需要 --name 或 --config 至少一个")
		}
		body := map[string]any{"name": name}
		return runBaseV3WithBody(cmd, "PATCH", pathFn, body)
	},
}

var bitableTableDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除数据表",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		if tableID == "" {
			return fmt.Errorf("--table-id 必填")
		}
		return runBaseV3Simple(cmd, "DELETE", func(bt string) string {
			return bitableTablePath(bt, tableID)
		}, nil)
	},
}

func init() {
	bitableCmd.AddCommand(bitableTableCmd)

	tableSubs := []*cobra.Command{
		bitableTableListCmd, bitableTableGetCmd, bitableTableCreateCmd,
		bitableTableUpdateCmd, bitableTableDeleteCmd,
	}
	for _, c := range tableSubs {
		bitableTableCmd.AddCommand(c)
		addBaseTokenFlag(c)
		c.Flags().String("user-access-token", "", "User Access Token")
	}

	bitableTableListCmd.Flags().Int("offset", 0, "单页模式的 offset（不传 --offset/--limit 时自动取全部）")
	bitableTableListCmd.Flags().Int("limit", 0, "单页模式的 limit（1-500；不传 --offset/--limit 时自动取全部）")

	bitableTableGetCmd.Flags().String("table-id", "", "table_id（必填）")
	mustMarkFlagRequired(bitableTableGetCmd, "table-id")

	bitableTableCreateCmd.Flags().String("name", "", "数据表名称")
	bitableTableCreateCmd.Flags().String("fields", "", "字段 JSON 数组（建表时一次创建全部字段）")
	bitableTableCreateCmd.Flags().String("config", "", "JSON 配置（与 --config-file 互斥）")
	bitableTableCreateCmd.Flags().String("config-file", "", "JSON 配置文件路径")

	bitableTableUpdateCmd.Flags().String("table-id", "", "table_id（必填）")
	bitableTableUpdateCmd.Flags().String("name", "", "新名称")
	bitableTableUpdateCmd.Flags().String("config", "", "JSON 配置")
	bitableTableUpdateCmd.Flags().String("config-file", "", "JSON 配置文件")
	mustMarkFlagRequired(bitableTableUpdateCmd, "table-id")

	bitableTableDeleteCmd.Flags().String("table-id", "", "table_id（必填）")
	mustMarkFlagRequired(bitableTableDeleteCmd, "table-id")
}
