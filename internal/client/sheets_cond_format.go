package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// sheets_cond_format.go —— 电子表格条件格式（公开 v2 condition_formats 接口）。
//
//	GET    /open-apis/sheets/v2/spreadsheets/:token/condition_formats?sheet_ids=a,b
//	POST   /open-apis/sheets/v2/spreadsheets/:token/condition_formats/batch_create
//	POST   /open-apis/sheets/v2/spreadsheets/:token/condition_formats/batch_update
//	DELETE /open-apis/sheets/v2/spreadsheets/:token/condition_formats/batch_delete
//
// 单次最多 10 条，每个子表最多 20 条。批量接口顶层 code=0 不代表每条都成功，需逐条看 res_code。

// MaxCondFormatsPerRequest 是条件格式批量接口单次条数上限。
const MaxCondFormatsPerRequest = 10

// CondFormatFont 条件格式字体样式。
type CondFormatFont struct {
	Bold   *bool `json:"bold,omitempty"`
	Italic *bool `json:"italic,omitempty"`
}

// CondFormatStyle 条件格式命中时的样式。
type CondFormatStyle struct {
	Font           *CondFormatFont `json:"font,omitempty"`
	TextDecoration *int            `json:"text_decoration,omitempty"` // 0 无、1 下划线、2 删除线、3 两者
	ForeColor      string          `json:"fore_color,omitempty"`
	BackColor      string          `json:"back_color,omitempty"`
}

// CondFormatAttr 条件格式规则参数（字段随 rule_type 不同）。
type CondFormatAttr struct {
	Operator   string   `json:"operator,omitempty"`
	TimePeriod string   `json:"time_period,omitempty"`
	Formula    []string `json:"formula,omitempty"`
	Text       string   `json:"text,omitempty"`
}

// CondFormat 单条条件格式。
type CondFormat struct {
	CfID     string           `json:"cf_id,omitempty"`
	Ranges   []string         `json:"ranges"`
	RuleType string           `json:"rule_type"`
	Attrs    []CondFormatAttr `json:"attrs,omitempty"`
	Style    *CondFormatStyle `json:"style,omitempty"`
}

// SheetCondFormat 是批量创建/更新请求中的一项。
type SheetCondFormat struct {
	SheetID         string     `json:"sheet_id"`
	ConditionFormat CondFormat `json:"condition_format"`
}

// SheetCfID 是批量删除请求中的一项。
type SheetCfID struct {
	SheetID string `json:"sheet_id"`
	CfID    string `json:"cf_id"`
}

// CondFormatResult 是批量写接口的逐条结果。
type CondFormatResult struct {
	SheetID string `json:"sheet_id"`
	CfID    string `json:"cf_id"`
	ResCode int    `json:"res_code"`
	ResMsg  string `json:"res_msg"`
}

// CondFormatPath 返回条件格式接口路径；action 为空时是列表接口，否则为 batch_create/batch_update/batch_delete。
func CondFormatPath(spreadsheetToken, action string) string {
	p := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/condition_formats", spreadsheetToken)
	if action != "" {
		p += "/" + action
	}
	return p
}

// ListCondFormats 查询子表的条件格式，返回接口 data.sheet_condition_formats 原样数组（保留未知字段）。
func ListCondFormats(ctx context.Context, spreadsheetToken string, sheetIDs []string, userAccessToken string) ([]map[string]any, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	path := CondFormatPath(spreadsheetToken, "") + "?sheet_ids=" + url.QueryEscape(strings.Join(sheetIDs, ","))
	respBody, err := v2APICallWithToken(c, ctx, "GET", path, nil, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("查询条件格式失败: %w", err)
	}
	var apiResp struct {
		Data struct {
			SheetConditionFormats []map[string]any `json:"sheet_condition_formats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Data.SheetConditionFormats == nil {
		return []map[string]any{}, nil
	}
	return apiResp.Data.SheetConditionFormats, nil
}

// BatchCreateCondFormats 批量创建条件格式。
func BatchCreateCondFormats(ctx context.Context, spreadsheetToken string, items []SheetCondFormat, userAccessToken string) ([]CondFormatResult, error) {
	return condFormatWrite(ctx, spreadsheetToken, "POST", "batch_create", "创建条件格式",
		map[string]any{"sheet_condition_formats": items}, userAccessToken)
}

// BatchUpdateCondFormats 批量更新条件格式（整条覆盖：未传的可选字段会被重置）。
func BatchUpdateCondFormats(ctx context.Context, spreadsheetToken string, items []SheetCondFormat, userAccessToken string) ([]CondFormatResult, error) {
	return condFormatWrite(ctx, spreadsheetToken, "POST", "batch_update", "更新条件格式",
		map[string]any{"sheet_condition_formats": items}, userAccessToken)
}

// BatchDeleteCondFormats 批量删除条件格式。
func BatchDeleteCondFormats(ctx context.Context, spreadsheetToken string, items []SheetCfID, userAccessToken string) ([]CondFormatResult, error) {
	return condFormatWrite(ctx, spreadsheetToken, "DELETE", "batch_delete", "删除条件格式",
		map[string]any{"sheet_cf_ids": items}, userAccessToken)
}

// condFormatWrite 发送批量写请求，并把逐条失败（res_code≠0）汇总为错误；结果始终返回，便于调用方输出。
func condFormatWrite(ctx context.Context, spreadsheetToken, method, action, label string, body any, userAccessToken string) ([]CondFormatResult, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	respBody, err := v2APICallWithToken(c, ctx, method, CondFormatPath(spreadsheetToken, action), body, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", label, err)
	}
	var apiResp struct {
		Data struct {
			Responses []CondFormatResult `json:"responses"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	results := apiResp.Data.Responses
	var failed []string
	for _, r := range results {
		if r.ResCode != 0 {
			failed = append(failed, fmt.Sprintf("%s/%s: res_code=%d %s", r.SheetID, r.CfID, r.ResCode, r.ResMsg))
		}
	}
	if len(failed) > 0 {
		return results, fmt.Errorf("%s部分失败（%d/%d 条）: %s", label, len(failed), len(results), strings.Join(failed, "; "))
	}
	return results, nil
}
