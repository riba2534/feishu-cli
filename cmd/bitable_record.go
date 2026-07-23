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
	Short: "列出记录（支持 --filter-json/--sort-json 结构化过滤排序，无需关键词）",
	Long: `GET /records，列出数据表记录。与 search 不同，list 不要求 keyword，
适合纯结构化条件筛选（按状态/数值/日期/空值过滤）。

filter DSL（--filter-json，实测验证）:
  {"logic":"and|or","conditions":[[字段名或ID, operator, 值], ...]}
  operator: == != > >= < <= intersects disjoint empty non_empty
  值按字段类型: 文本→字符串（intersects 做包含匹配）；数字→数字；
    单选/多选→选项名数组（单选也必须写数组，如 ["P0"]）；复选框→true/false；
    人员/群/关联→对象数组 [{"id":"ou_xxx"}]；
    日期→"ExactDate(2026-01-01)" 或 "Today"/"Yesterday"/"Tomorrow"；
    empty/non_empty 可省略值写二元组 [字段, "empty"]

示例:
  # 状态为 Doing 且分数 >= 70
  feishu-cli bitable record list --base-token <bt> --table-id <tid> \
    --filter-json '{"logic":"and","conditions":[["状态","==",["Doing"]],["分数",">=",70]]}'

  # 按分数降序
  feishu-cli bitable record list --base-token <bt> --table-id <tid> \
    --sort-json '[{"field":"分数","desc":true}]'

  # 字段投影：只返回指定字段（大表控制输出体积，可重复，最多 100 个）
  feishu-cli bitable record list --base-token <bt> --table-id <tid> \
    --field-id 名称 --field-id 状态`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		viewID, _ := cmd.Flags().GetString("view-id")
		offset, _ := cmd.Flags().GetInt("offset")
		limit, _ := cmd.Flags().GetInt("limit")
		filterJSON, _ := cmd.Flags().GetString("filter-json")
		sortJSON, _ := cmd.Flags().GetString("sort-json")
		selectFields, err := recordSelectFields(cmd, 100)
		if err != nil {
			return err
		}
		params := map[string]any{}
		if viewID != "" {
			params["view_id"] = viewID
		}
		// 字段投影：field_id 作为重复 query 参数下发（?field_id=A&field_id=B）
		if len(selectFields) > 0 {
			params["field_id"] = selectFields
		}
		if offset > 0 {
			params["offset"] = offset
		}
		if limit > 0 {
			params["limit"] = limit
		}
		// filter/sort 在 GET 端点作为 JSON 串 query 参数下发（服务端解析字符串）
		if filterJSON != "" {
			if err := validateCompactJSON(filterJSON, "--filter-json"); err != nil {
				return err
			}
			params["filter"] = filterJSON
		}
		if sortJSON != "" {
			if err := validateCompactJSON(sortJSON, "--sort-json"); err != nil {
				return err
			}
			params["sort"] = sortJSON
		}
		return runBaseV3Simple(cmd, "GET", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID)
		}, params)
	},
}

// recordSelectFields 读取 --field-id 投影 flag（可重复，值为字段名或字段 ID）并做数量上限校验。
// 服务端上限：list/batch_get 100 个，search 50 个。
func recordSelectFields(cmd *cobra.Command, max int) ([]string, error) {
	raw, _ := cmd.Flags().GetStringArray("field-id")
	fields := make([]string, 0, len(raw))
	for _, f := range raw {
		if s := strings.TrimSpace(f); s != "" {
			fields = append(fields, s)
		}
	}
	if len(fields) > max {
		return nil, fmt.Errorf("--field-id 最多 %d 个，当前 %d 个", max, len(fields))
	}
	return fields, nil
}

// validateCompactJSON 校验 flag 值是合法 JSON，尽早给出可读错误。
func validateCompactJSON(s, flagName string) error {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return fmt.Errorf("解析 %s 失败（需为合法 JSON）: %w", flagName, err)
	}
	return nil
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
  --field-id          仅返回指定字段（投影，可重复，最多 50 个；对应 body select_fields）
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
  本命令兼容 v1 格式：如果传入 {"fields":{...}}，会自动解包为 {...}。

select 字段写法（实测）:
  单选写字符串 "Todo" 或数组 ["Todo"] 均可（服务端归一化）；多选写数组。
  单条端点（本命令的 POST/PATCH）会静默自动创建未知选项；批量端点
  （batch_create/batch_update）则拒绝未知选项报 not_found——
  写前先 field list 确认选项存在，防止拼写错误静默产生脏选项。

层级关系（子记录）:
  通过 link 字段写父记录引用数组实现，如 {"父任务":[{"id":"rec_xxx"}]}；
  不存在 parent_record_id 参数或独立的子记录 API。`,
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
	selectFields, err := recordSelectFields(cmd, 50)
	if err != nil {
		return nil, err
	}
	filterJSON, _ := cmd.Flags().GetString("filter-json")
	sortJSON, _ := cmd.Flags().GetString("sort-json")
	viewID, _ := cmd.Flags().GetString("view-id")
	usedConvenience := strings.TrimSpace(keyword) != "" || len(searchFields) > 0 || len(selectFields) > 0 ||
		strings.TrimSpace(filterJSON) != "" || strings.TrimSpace(sortJSON) != "" || strings.TrimSpace(viewID) != "" ||
		cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

	if configProvided {
		if usedConvenience {
			return nil, fmt.Errorf("--config/--config-file 与 --keyword/--search-field/--field-id/--filter-json/--sort-json/--view-id/--offset/--limit 互斥，请二选一")
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
	if len(selectFields) > 0 {
		body["select_fields"] = selectFields
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
	Short: "批量创建记录（推荐行式：{\"create_records\":[{\"字段\":\"值\"}]}）",
	Long: `POST /records/batch_create，批量创建记录。两种 body 形态（实测均可用）：

推荐 · create_records 行式 —— 每条记录独立字段 map，可各带不同字段、无需 null 占位:
  {"create_records":[{"名称":"Task A","状态":"Todo"},{"名称":"Task B"}]}
  返回 record_id_list（+ 可选 ignored_fields）

备选 · fields+rows 列式 —— 所有行共享同一字段顺序:
  {"fields":["名称","状态"],"rows":[["Task A","Todo"],["Task B",null]]}

单次最多 200 条记录。select 字段写法见 record upsert --help。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		return runBaseV3WithJSON(cmd, "POST", func(baseToken string) string {
			return bitableRecordPath(baseToken, tableID, "batch_create")
		})
	},
}

var bitableRecordBatchUpdateCmd = &cobra.Command{
	Use:   "batch-update",
	Short: "批量更新记录（支持统一 patch 与逐记录差异化两种形态）",
	Long: `POST /records/batch_update，批量更新记录。两种 body 形态（实测均可用）：

形态一 · 统一 patch —— 一组记录打同一份修改:
  {"record_id_list":["rec1","rec2"],"patch":{"状态":["Done"]}}

形态二 · 逐记录差异化 —— 每条记录各改各的字段和值:
  {"update_records":{
     "rec1":{"分数":88},
     "rec2":{"分数":77,"状态":["Blocked"]}
  }}

示例:
  feishu-cli bitable record batch-update --base-token <bt> --table-id <tid> \
    --config '{"update_records":{"recXXX":{"分数":88},"recYYY":{"状态":["Done"]}}}'

select 字段（实测）: 单选写字符串 "Done" 或数组 ["Done"] 均可（服务端归一化）；多选写数组。
批量端点（batch_create/batch_update）只接受字段中已有的选项，未知选项报 not_found
（hint 会列出可用选项）；单条 upsert 端点才会静默自动创建未知选项——
写前先 field list 确认选项存在，防拼错产生脏选项。

注意: 单次建议 ≤200 条；响应不校验 record_id 是否存在，需要确认时读回记录。`,
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
	bitableRecordListCmd.Flags().String("filter-json", "", `结构化过滤 JSON（tuple DSL，见 --help）`)
	bitableRecordListCmd.Flags().String("sort-json", "", `排序 JSON 数组，如 [{"field":"分数","desc":true}]`)
	bitableRecordListCmd.Flags().StringArray("field-id", nil, "仅返回指定字段（字段名或字段 ID，可重复，最多 100 个）")

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
	bitableRecordSearchCmd.Flags().StringArray("field-id", nil, "仅返回指定字段（投影，可重复，最多 50 个）")
	bitableRecordSearchCmd.Flags().String("filter-json", "", "结构化过滤条件 JSON（logic/conditions，需配合 --keyword/--search-field），见命令 Long 示例")
	bitableRecordSearchCmd.Flags().String("sort-json", "", "排序 JSON 数组（field/desc），见命令 Long 示例")
	bitableRecordSearchCmd.Flags().String("view-id", "", "限定视图 ID")
	bitableRecordSearchCmd.Flags().Int("offset", 0, "分页 offset")
	bitableRecordSearchCmd.Flags().Int("limit", 10, "分页大小（1-200，默认 10）")
}
