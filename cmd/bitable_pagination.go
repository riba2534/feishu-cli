package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// base/v3 的 table / field / view 列表按 offset/limit 分页，只返回 total、不返回 has_more；
// 不传 limit 时服务端默认只给 20 条（实测 150 个字段的表 field list 只返回 20 个）。
//
// 每页直接取服务端上限 500（实测 501 报 800010701 "Number must be less than or equal to 500"）：
// 实测 field list 每次请求返回的字段顺序并不稳定，按 offset 翻两页（limit=100）去重后
// 150 个字段只拿到 115~119 个——多页拼接会重复和遗漏。用最大页让绝大多数表一次取完，
// 万一仍需翻页则按 id 去重，去重后少于 total 时在 stderr 告警。
const (
	baseV3ListPageLimit = 500
	baseV3ListMaxPages  = 100 // 防御：500×100=5 万项，远超表/字段/视图的平台上限
)

// listBaseV3Items 按 offset/limit 翻页，取完 path 下 key 数组的全部元素（按 id 去重）。
// 返回全部元素与服务端 total（缺失时为已取到的数量）。
func listBaseV3Items(path, key, token string) ([]any, int, error) {
	items := make([]any, 0)
	seen := make(map[string]bool)
	total := 0
	offset := 0
	pages := 0
	for ; ; pages++ {
		if pages >= baseV3ListMaxPages {
			return nil, 0, fmt.Errorf("列表翻页超过 %d 页仍未取完（已取 %d 项，total=%d），为防止死循环已停止", baseV3ListMaxPages, len(items), total)
		}
		data, err := client.BaseV3Call("GET", path, map[string]any{"offset": offset, "limit": baseV3ListPageLimit}, nil, token)
		if err != nil {
			return nil, 0, err
		}
		batch, _ := data[key].([]any)
		for _, item := range batch {
			if m, ok := item.(map[string]any); ok {
				if id, _ := m["id"].(string); id != "" {
					if seen[id] {
						continue
					}
					seen[id] = true
				}
			}
			items = append(items, item)
		}
		if t := bitableJSONInt(data["total"]); t > 0 {
			total = t
		}
		if len(batch) == 0 {
			break
		}
		offset += len(batch)
		if total > 0 {
			if offset >= total {
				break
			}
		} else if len(batch) < baseV3ListPageLimit {
			break
		}
	}
	if pages > 0 && total > len(items) {
		fmt.Fprintf(os.Stderr, "警告: %s 共 %d 项，跨页去重后只取到 %d 项（服务端分页顺序不稳定），结果可能不完整，请重试\n", key, total, len(items))
	}
	if total < len(items) {
		total = len(items)
	}
	return items, total, nil
}

// bitableJSONInt 把 json.Number / float64 / int 转成 int，其他类型返回 0。
func bitableJSONInt(v any) int {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// ==================== record list 分页 ====================

const (
	recordListDefaultLimit        = 100  // 对齐官方 json 模式默认 100（服务端不传 limit 时只给 20 条）
	recordListMaxLimit            = 2000 // 服务端上限（实测 2001 报 800004006: limit must be between 1 and 2000）
	recordListPageAllDefaultLimit = 500  // --page-all 未指定 --limit 时的每页条数
	recordListPageAllMaxPages     = 1000 // --page-all 翻页上限，防止异常响应导致无限循环
)

// recordPageFetcher 拉取一页记录（offset/limit 已写入 params）。
type recordPageFetcher func(params map[string]any) (map[string]any, error)

// recordListOptions 控制 record list 的翻页行为。
type recordListOptions struct {
	Offset   int
	Limit    int
	PageAll  bool
	MaxPages int // 0 表示 recordListPageAllMaxPages
}

// fetchRecordList 执行 record list：单页模式 has_more=true 时补 next_offset 并在 stderr 提示；
// --page-all 按 offset 循环取完，合并矩阵（record_id_list/data），校验各页表结构一致，
// 发现 rev 变化（分页期间表数据被修改）时在 stderr 告警并输出 rev_changed=true。
func fetchRecordList(fetch recordPageFetcher, baseParams map[string]any, opts recordListOptions, warn io.Writer) (map[string]any, error) {
	if warn == nil {
		warn = os.Stderr
	}
	pageParams := func(offset int) map[string]any {
		p := make(map[string]any, len(baseParams)+2)
		for k, v := range baseParams {
			p[k] = v
		}
		p["offset"] = offset
		p["limit"] = opts.Limit
		return p
	}

	first, err := fetch(pageParams(opts.Offset))
	if err != nil {
		return nil, err
	}
	if first == nil {
		first = map[string]any{}
	}
	firstCount := len(bitableAnySlice(first["record_id_list"]))

	if !opts.PageAll {
		if hasMore, _ := first["has_more"].(bool); hasMore {
			next := opts.Offset + firstCount
			first["next_offset"] = next
			fmt.Fprintf(warn, "提示: 还有更多记录（has_more=true，本页 %d 条）。续翻加 --offset %d，或用 --page-all 自动翻页取全部\n", firstCount, next)
		}
		return first, nil
	}

	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = recordListPageAllMaxPages
	}
	merged := make(map[string]any, len(first)+2)
	for k, v := range first {
		merged[k] = v
	}
	recordIDs := append([]any{}, bitableAnySlice(first["record_id_list"])...)
	rows := append([]any{}, bitableAnySlice(first["data"])...)
	firstRev, hasRev := first["rev"]
	revChanged := false
	offset := opts.Offset + firstCount
	hasMore, _ := first["has_more"].(bool)
	pages := 1

	for hasMore {
		if pages >= maxPages {
			merged["next_offset"] = offset
			fmt.Fprintf(warn, "警告: --page-all 已达翻页上限 %d 页（已取 %d 条），结果未取完；续翻加 --offset %d --page-all\n", maxPages, len(recordIDs), offset)
			break
		}
		page, err := fetch(pageParams(offset))
		if err != nil {
			return nil, fmt.Errorf("--page-all 第 %d 页（offset=%d）失败，已取 %d 条: %w", pages+1, offset, len(recordIDs), err)
		}
		pages++
		if page == nil {
			page = map[string]any{}
		}
		if err := checkRecordPageSchema(first, page); err != nil {
			return nil, fmt.Errorf("--page-all 第 %d 页（offset=%d）%w；请重试", pages, offset, err)
		}
		if hasRev && !revChanged {
			if rev, ok := page["rev"]; ok && fmt.Sprint(rev) != fmt.Sprint(firstRev) {
				revChanged = true
				fmt.Fprintf(warn, "警告: 分页期间数据表发生变更（rev %v → %v），合并结果可能有重复或遗漏；需要一致快照请在数据静止时重试\n", firstRev, rev)
			}
		}
		ids := bitableAnySlice(page["record_id_list"])
		recordIDs = append(recordIDs, ids...)
		rows = append(rows, bitableAnySlice(page["data"])...)
		hasMore, _ = page["has_more"].(bool)
		if hasMore && len(ids) == 0 {
			return nil, fmt.Errorf("服务端返回 has_more=true 但第 %d 页（offset=%d）为空，为防止死循环已停止（已取 %d 条）", pages, offset, len(recordIDs))
		}
		offset += len(ids)
	}

	merged["record_id_list"] = recordIDs
	merged["data"] = rows
	merged["has_more"] = hasMore
	if !hasMore {
		delete(merged, "next_offset")
	}
	if revChanged {
		merged["rev_changed"] = true
	}
	return merged, nil
}

// checkRecordPageSchema 校验后续页的列结构与首页一致（列不同无法按矩阵合并）。
func checkRecordPageSchema(first, page map[string]any) error {
	for _, key := range []string{"fields", "field_id_list", "field_type_list"} {
		if !reflect.DeepEqual(first[key], page[key]) {
			return fmt.Errorf("的列结构（%s）与首页不一致，表结构在分页期间被修改", key)
		}
	}
	return nil
}

func bitableAnySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// ==================== page_token 分页（workflow / form / dashboard 列表） ====================

const bitablePageTokenMaxPages = 200

// bitablePageTokenList 描述一个按 page_token 分页的 base/v3 列表端点。
type bitablePageTokenList struct {
	Method   string         // GET（参数走 query）或 POST（参数走 body，如 workflows/list）
	Path     string         // 完整 API 路径
	Key      string         // 列表数组字段名：forms / items
	Params   map[string]any // 额外过滤参数（不含 page_size/page_token）
	PageSize int            // 每页大小；<=0 时不传
}

// fetchBitablePageTokenList 自动按 page_token 翻页取完（重复游标与页数上限防护），
// 输出最后一页的信封并把 Key 替换为全部元素：has_more=false、page_token=""、total=全部数量。
func fetchBitablePageTokenList(spec bitablePageTokenList, token string) (map[string]any, error) {
	all := make([]any, 0)
	seenTokens := map[string]bool{}
	pageToken := ""
	var last map[string]any
	for page := 0; ; page++ {
		if page >= bitablePageTokenMaxPages {
			return nil, fmt.Errorf("自动翻页超过 %d 页仍未取完（已取 %d 项），为防止死循环已停止", bitablePageTokenMaxPages, len(all))
		}
		args := make(map[string]any, len(spec.Params)+2)
		for k, v := range spec.Params {
			args[k] = v
		}
		if spec.PageSize > 0 {
			args["page_size"] = spec.PageSize
		}
		if pageToken != "" {
			args["page_token"] = pageToken
		}
		var (
			data map[string]any
			err  error
		)
		if spec.Method == "POST" {
			data, err = client.BaseV3Call("POST", spec.Path, nil, args, token)
		} else {
			data, err = client.BaseV3Call("GET", spec.Path, args, nil, token)
		}
		if err != nil {
			if page > 0 {
				return nil, fmt.Errorf("自动翻页第 %d 页失败（已取 %d 项）: %w", page+1, len(all), err)
			}
			return nil, err
		}
		if data == nil {
			data = map[string]any{}
		}
		last = data
		all = append(all, bitableAnySlice(data[spec.Key])...)
		hasMore, _ := data["has_more"].(bool)
		next, _ := data["page_token"].(string)
		if !hasMore || next == "" {
			break
		}
		if seenTokens[next] {
			return nil, fmt.Errorf("服务端返回了重复的 page_token（已取 %d 项），为防止死循环已停止", len(all))
		}
		seenTokens[next] = true
		pageToken = next
	}
	out := make(map[string]any, len(last)+3)
	for k, v := range last {
		out[k] = v
	}
	out[spec.Key] = all
	out["has_more"] = false
	if _, ok := out["page_token"]; ok {
		out["page_token"] = ""
	}
	out["total"] = len(all)
	return out, nil
}

// warnBitableHasMore 单页模式（显式 --page-token）还有下一页时在 stderr 提示。
func warnBitableHasMore(data map[string]any) {
	if hasMore, _ := data["has_more"].(bool); hasMore {
		if next, _ := data["page_token"].(string); next != "" {
			fmt.Fprintf(os.Stderr, "提示: 还有更多结果（has_more=true），续翻加 --page-token %s；不传 --page-token 时自动取全部\n", next)
		}
	}
}

// runBitablePageTokenList 列表命令统一入口：未传 --page-token 时自动翻页取全部；
// 显式传 --page-token 时只取该页（兼容旧用法），还有下一页时 stderr 提示。
func runBitablePageTokenList(cmd *cobra.Command, build func(baseToken string) bitablePageTokenList) error {
	if err := config.Validate(); err != nil {
		return err
	}
	baseToken, err := resolveBaseToken(cmd)
	if err != nil {
		return err
	}
	spec := build(baseToken)
	pageToken, _ := cmd.Flags().GetString("page-token")
	pageToken = strings.TrimSpace(pageToken)

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		args := map[string]any{}
		for k, v := range spec.Params {
			args[k] = v
		}
		if spec.PageSize > 0 {
			args["page_size"] = spec.PageSize
		}
		if pageToken != "" {
			args["page_token"] = pageToken
		}
		preview := map[string]any{"api": "base/v3", "method": spec.Method, "path": spec.Path, "auto_paginate": pageToken == ""}
		if spec.Method == "POST" {
			preview["body"] = args
		} else {
			preview["params"] = args
		}
		o, oerr := output.ParseOptions(cmd)
		if oerr != nil {
			return oerr
		}
		return output.Render(o, preview)
	}

	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	if pageToken == "" {
		data, err := fetchBitablePageTokenList(spec, token)
		if err != nil {
			return err
		}
		return renderBitableResult(cmd, data)
	}
	args := map[string]any{"page_token": pageToken}
	for k, v := range spec.Params {
		args[k] = v
	}
	if spec.PageSize > 0 {
		args["page_size"] = spec.PageSize
	}
	var data map[string]any
	if spec.Method == "POST" {
		data, err = client.BaseV3Call("POST", spec.Path, nil, args, token)
	} else {
		data, err = client.BaseV3Call("GET", spec.Path, args, nil, token)
	}
	if err != nil {
		return err
	}
	warnBitableHasMore(data)
	return renderBitableResult(cmd, data)
}
