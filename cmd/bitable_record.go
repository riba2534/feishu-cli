package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// record 子命令组
var bitableRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "记录管理（list/get/search/upsert/batch-create/batch-update/batch-get/delete/history-list/share-link/*-attachment）",
}

func bitableRecordPath(baseToken, tableID string, extra ...string) string {
	parts := []string{"bases", baseToken, "tables", tableID, "records"}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

var bitableRecordListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出记录",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		viewID, _ := cmd.Flags().GetString("view-id")
		offset, _ := cmd.Flags().GetInt("offset")
		limit, _ := cmd.Flags().GetInt("limit")
		params := map[string]any{}
		if viewID != "" {
			params["view_id"] = viewID
		}
		if offset > 0 {
			params["offset"] = offset
		}
		if limit > 0 {
			params["limit"] = limit
		}
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID)
		}, params)
	},
}

var bitableRecordGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取单条记录",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		if recordID == "" {
			return fmt.Errorf("--record-id 必填")
		}
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, recordID)
		}, nil)
	},
}

var bitableRecordSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "搜索记录（关键词搜索 + 结构化过滤/排序）",
	Long: `POST /records/search，按关键词或结构化条件搜索记录。

便捷 flag（推荐）:
  --keyword           搜索关键词
  --search-field      搜索字段名/ID（可重复；便捷模式必填）
  --filter-json       结构化过滤 JSON，需叠加在 keyword 搜索上
  --sort-json         排序 JSON 数组，如 [{"field":"名称","desc":true}]
  --view-id           限定视图
  --offset / --limit  分页（limit 1-200，默认 10）

示例:
  # 关键词搜索
  feishu-cli bitable record search --base-token <bt> --table-id <tid> --keyword 测试 --search-field 名称

  # 在关键词搜索上叠加结构化过滤（filter 结构遵循 base/v3 records/search 规范）
  feishu-cli bitable record search --base-token <bt> --table-id <tid> \
    --keyword 测试 --search-field 名称 --filter-json '<filter JSON>'

  # 完整请求体（逃生舱，--config）
  feishu-cli bitable record search --base-token <bt> --table-id <tid> \
    --config '{"keyword":"测试","search_fields":["名称"],"filter":{...}}'

注意: search 用 filter/keyword 结构，不是 upsert 的 {"fields":{...}} 格式。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		body, err := buildRecordSearchBody(cmd)
		if err != nil {
			return err
		}
		return runBaseV3WithBody(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "search")
		}, body)
	},
}

var bitableRecordUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "记录 upsert（传 --record-id 时 PATCH 更新，不传时 POST 创建）",
	Long: `官方 base/v3 没有专用 upsert 端点。本命令根据 --record-id 是否存在自动选择：
  - 不传 --record-id: POST /records 创建新记录
  - 传 --record-id:   PATCH /records/{record_id} 更新已有记录

必填:
  --table-id  目标数据表
  --config / --config-file  记录 body（形如 {"fields":{"字段名":"值"}}）

v3 API 说明:
  base/v3 的单条 POST/PATCH 端点要求字段映射放在 body 顶层（不带 "fields" 包装）。
  本命令兼容 v1 格式：如果传入 {"fields":{...}}，会自动解包为 {...}。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")

		// 读取用户输入的 JSON body，自动适配 v3 格式
		body, err := loadRecordBody(cmd)
		if err != nil {
			return err
		}

		method := "POST"
		pathFn := func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID)
		}
		if recordID != "" {
			method = "PATCH"
			pathFn = func(baseToken string) string {
				return bitableRecordPath(baseToken, tableID, recordID)
			}
		}
		return runBaseV3WithBody(cmd, method, pathFn, body)
	},
}

// loadRecordBody 读取 --config/--config-file 并适配 v3 格式。
// v3 单条 POST/PATCH 要求字段映射在 body 顶层，不带 "fields" 包装。
// 兼容用户传 v1 格式 {"fields":{"name":"value"}}，自动解包。
func loadRecordBody(cmd *cobra.Command) (any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("解析 --config 失败: %w", err)
	}
	// 如果用户传了 {"fields": {...}}，提取 fields 的值作为 body
	if fields, ok := parsed["fields"]; ok {
		if fm, ok := fields.(map[string]any); ok && len(parsed) == 1 {
			return fm, nil
		}
	}
	return parsed, nil
}

// buildRecordSearchBody 构造 record search 请求体。
// 优先级：--config/--config-file（完整 body 逃生舱）> 便捷 flag 拼装。
// 对 --config 做友好检测：若误传 upsert 的 {"fields":{...}} 格式，给出明确指引，
// 避免用户把 upsert 的请求体套到 search 端点（会触发 800010701 校验失败）。
func buildRecordSearchBody(cmd *cobra.Command) (any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	configProvided := strings.TrimSpace(configJSON) != "" || strings.TrimSpace(configFile) != ""

	keyword, _ := cmd.Flags().GetString("keyword")
	searchFields, _ := cmd.Flags().GetStringArray("search-field")
	filterJSON, _ := cmd.Flags().GetString("filter-json")
	sortJSON, _ := cmd.Flags().GetString("sort-json")
	viewID, _ := cmd.Flags().GetString("view-id")
	usedConvenience := strings.TrimSpace(keyword) != "" || len(searchFields) > 0 ||
		strings.TrimSpace(filterJSON) != "" || strings.TrimSpace(sortJSON) != "" || strings.TrimSpace(viewID) != "" ||
		cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

	if configProvided {
		if usedConvenience {
			return nil, fmt.Errorf("--config/--config-file 与 --keyword/--search-field/--filter-json/--sort-json/--view-id/--offset/--limit 互斥，请二选一")
		}
		raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "搜索请求体")
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("解析 --config 失败: %w", err)
		}
		// 友好检测：误传 upsert 的 {"fields":{...}} 格式（顶层只有 fields，无 filter/keyword/search_fields）
		if _, hasFields := parsed["fields"]; hasFields {
			_, hasFilter := parsed["filter"]
			_, hasKw := parsed["keyword"]
			_, hasSf := parsed["search_fields"]
			if !hasFilter && !hasKw && !hasSf {
				return nil, fmt.Errorf("--config 顶层是 fields（这是 record upsert 的请求体格式），record search 需要 keyword/filter 结构\n建议改用关键词搜索: --keyword 值 --search-field 字段名（或参考 base/v3 records/search 文档构造 filter 请求体）")
			}
		}
		return parsed, nil
	}

	// 便捷 flag 模式校验
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, fmt.Errorf("record search 便捷模式必须提供 --keyword（或用 --config 传完整请求体）")
	}
	if len(searchFields) == 0 {
		return nil, fmt.Errorf("--keyword 必须配合至少一个 --search-field（指定在哪些字段里搜索）")
	}
	offset, _ := cmd.Flags().GetInt("offset")
	limit, _ := cmd.Flags().GetInt("limit")
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("--limit 范围 1-200，当前 %d", limit)
	}

	body := map[string]any{"offset": offset, "limit": limit}
	if kw != "" {
		body["keyword"] = kw
	}
	if len(searchFields) > 0 {
		body["search_fields"] = searchFields
	}
	if v := strings.TrimSpace(viewID); v != "" {
		body["view_id"] = v
	}
	if raw := strings.TrimSpace(filterJSON); raw != "" {
		var f any
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, fmt.Errorf("解析 --filter-json 失败: %w", err)
		}
		body["filter"] = f
	}
	if raw := strings.TrimSpace(sortJSON); raw != "" {
		var s any
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, fmt.Errorf("解析 --sort-json 失败: %w", err)
		}
		body["sort"] = s
	}
	return body, nil
}

var bitableRecordBatchCreateCmd = &cobra.Command{
	Use:   "batch-create",
	Short: "批量创建记录（v3 格式：{\"fields\":[\"fld1\"],\"rows\":[[\"val1\"]]}）",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		return runBaseV3WithJSON(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "batch_create")
		})
	},
}

var bitableRecordBatchUpdateCmd = &cobra.Command{
	Use:   "batch-update",
	Short: "批量更新记录（v3 格式：{\"record_id_list\":[\"rec1\"],\"patch\":{\"字段\":\"值\"}}）",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		return runBaseV3WithJSON(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "batch_update")
		})
	},
}

var bitableRecordDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除单条记录",
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		if recordID == "" {
			return fmt.Errorf("--record-id 必填")
		}
		return runBaseV3Simple(cmd, "DELETE", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, recordID)
		}, nil)
	},
}

var bitableRecordBatchDeleteCmd = &cobra.Command{
	Use:   "batch-delete",
	Short: "批量删除记录（POST batch_delete，单次最多 500 条）",
	Long: `批量删除多条记录，对应 base/v3 的 records/batch_delete 接口。

参数（任选其一）:
  --record-ids   逗号分隔的 record_id 列表
  --from-file    每行一个 record_id 的文本文件

可选:
  --table-id     目标数据表（必填）
  --base-token   多维表格 token（必填）

注意:
  - 单次最多 500 条；超过会报 400
  - 与 record delete 单条接口的区别：batch-delete 走 POST batch_delete，对大量删除场景效率更高（少一次握手）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordIDsCSV, _ := cmd.Flags().GetString("record-ids")
		fromFile, _ := cmd.Flags().GetString("from-file")

		ids, err := loadBatchDeleteRecordIDs(recordIDsCSV, fromFile)
		if err != nil {
			return err
		}
		if len(ids) > 500 {
			return fmt.Errorf("单次最多 500 条，当前传入 %d 条", len(ids))
		}

		body := map[string]any{"record_id_list": ids}
		return runBaseV3WithBody(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "batch_delete")
		}, body)
	},
}

// loadBatchDeleteRecordIDs 解析 --record-ids（逗号分隔）或 --from-file（每行一个）。
// 至少需要其中一个，且最终 record_id 列表不能为空。
func loadBatchDeleteRecordIDs(csv, fromFile string) ([]string, error) {
	var ids []string
	if csv != "" {
		ids = append(ids, splitAndTrim(csv)...)
	}
	if fromFile != "" {
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, fmt.Errorf("读取 --from-file 失败: %w", err)
		}
		for _, raw := range strings.Split(string(data), "\n") {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				ids = append(ids, raw)
			}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("--record-ids 或 --from-file 至少需要提供一个")
	}
	return ids, nil
}

var bitableRecordHistoryListCmd = &cobra.Command{
	Use:   "history-list",
	Short: "记录修改历史",
	Long: `查询单条记录的修改历史。

必填:
  --table-id    目标数据表
  --record-id   目标记录

可选:
  --page-size    分页大小
  --max-version  最大版本号`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordID, _ := cmd.Flags().GetString("record-id")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		maxVersion, _ := cmd.Flags().GetInt("max-version")
		if recordID == "" {
			return fmt.Errorf("--record-id 必填")
		}
		params := map[string]any{
			"table_id":  tableID,
			"record_id": recordID,
		}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if maxVersion > 0 {
			params["max_version"] = maxVersion
		}
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			return client.BaseV3Path("bases", baseToken, "record_history")
		}, params)
	},
}

var bitableRecordShareLinkCmd = &cobra.Command{
	Use:   "share-link",
	Short: "为一个或多个记录批量生成共享链接（最多 100 条/次）",
	Long: `批量生成记录的共享链接，对应 POST /records/share_links/batch。

参数（任选其一）:
  --record-ids   逗号分隔的 record_id 列表（最多 100 条）
  --from-file    每行一个 record_id 的文本文件

必填:
  --base-token   多维表格 token
  --table-id     目标数据表

示例:
  # 单条记录
  feishu-cli bitable record share-link --base-token <bt> --table-id <tid> --record-ids recxxx

  # 多条记录
  feishu-cli bitable record share-link --base-token <bt> --table-id <tid> --record-ids rec001,rec002,rec003`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		recordIDsCSV, _ := cmd.Flags().GetString("record-ids")
		fromFile, _ := cmd.Flags().GetString("from-file")

		ids, err := loadBatchDeleteRecordIDs(recordIDsCSV, fromFile)
		if err != nil {
			return err
		}
		if len(ids) > 100 {
			return fmt.Errorf("单次最多 100 条，当前传入 %d 条", len(ids))
		}

		body := map[string]any{"record_ids": ids}
		return runBaseV3WithBody(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "share_links", "batch")
		}, body)
	},
}

func init() {
	bitableCmd.AddCommand(bitableRecordCmd)

	recordSubs := []*cobra.Command{
		bitableRecordListCmd, bitableRecordGetCmd, bitableRecordSearchCmd,
		bitableRecordUpsertCmd, bitableRecordBatchCreateCmd, bitableRecordBatchUpdateCmd,
		bitableRecordDeleteCmd, bitableRecordBatchDeleteCmd, bitableRecordHistoryListCmd,
		bitableRecordShareLinkCmd,
	}
	for _, c := range recordSubs {
		bitableRecordCmd.AddCommand(c)
		addBaseTokenFlag(c)
		c.Flags().String("table-id", "", "table_id（必填）")
		c.Flags().String("user-access-token", "", "User Access Token")
		mustMarkFlagRequired(c, "table-id")
	}

	// list 额外参数
	bitableRecordListCmd.Flags().String("view-id", "", "视图 ID 过滤")
	bitableRecordListCmd.Flags().Int("offset", 0, "offset")
	bitableRecordListCmd.Flags().Int("limit", 0, "limit")

	// get 需要 record-id
	bitableRecordGetCmd.Flags().String("record-id", "", "record_id（必填）")

	// delete 需要 record-id
	bitableRecordDeleteCmd.Flags().String("record-id", "", "record_id（必填）")

	// batch-delete 通过 --record-ids 或 --from-file 传入
	bitableRecordBatchDeleteCmd.Flags().String("record-ids", "", "逗号分隔的 record_id 列表")
	bitableRecordBatchDeleteCmd.Flags().String("from-file", "", "每行一个 record_id 的文件")

	// share-link 同样用 --record-ids 或 --from-file
	bitableRecordShareLinkCmd.Flags().String("record-ids", "", "逗号分隔的 record_id 列表（最多 100 条）")
	bitableRecordShareLinkCmd.Flags().String("from-file", "", "每行一个 record_id 的文件")

	// upsert 可选 record-id（有则 PATCH 更新，无则 POST 创建）
	bitableRecordUpsertCmd.Flags().String("record-id", "", "record_id（不传则创建新记录）")

	// history-list 不用 --config，用 query params
	bitableRecordHistoryListCmd.Flags().String("record-id", "", "record_id（必填）")
	bitableRecordHistoryListCmd.Flags().Int("page-size", 0, "分页大小")
	bitableRecordHistoryListCmd.Flags().Int("max-version", 0, "最大版本号")
	mustMarkFlagRequired(bitableRecordHistoryListCmd, "record-id")

	// upsert / batch-create / batch-update 需要 --config（裸 JSON 透传）
	for _, c := range []*cobra.Command{bitableRecordUpsertCmd,
		bitableRecordBatchCreateCmd, bitableRecordBatchUpdateCmd} {
		c.Flags().String("config", "", "JSON 请求体")
		c.Flags().String("config-file", "", "JSON 请求体文件")
	}

	// search 既有 --config 逃生舱，也有便捷 flag（二者互斥）
	bitableRecordSearchCmd.Flags().String("config", "", "完整搜索请求体 JSON（逃生舱，与便捷 flag 互斥）")
	bitableRecordSearchCmd.Flags().String("config-file", "", "完整搜索请求体 JSON 文件")
	bitableRecordSearchCmd.Flags().String("keyword", "", "搜索关键词")
	bitableRecordSearchCmd.Flags().StringArray("search-field", nil, "搜索字段名/ID（可重复；便捷模式必填）")
	bitableRecordSearchCmd.Flags().String("filter-json", "", "结构化过滤条件 JSON（logic/conditions，需配合 --keyword/--search-field），见命令 Long 示例")
	bitableRecordSearchCmd.Flags().String("sort-json", "", "排序 JSON 数组（field/desc），见命令 Long 示例")
	bitableRecordSearchCmd.Flags().String("view-id", "", "限定视图 ID")
	bitableRecordSearchCmd.Flags().Int("offset", 0, "分页 offset")
	bitableRecordSearchCmd.Flags().Int("limit", 10, "分页大小（1-200，默认 10）")
}
