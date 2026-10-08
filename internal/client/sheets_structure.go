package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// sheets_structure.go —— 工作表结构操作（公开 v2/v3 接口）：子表属性更新（改名/隐藏/移动/冻结）、
// 行列移动。

// UpdateSheetProperties 更新子表属性（v2 sheets_batch_update.updateSheet）。
// 可同时修改标题、位置、隐藏状态与冻结行列数；nil 字段不修改。
func UpdateSheetProperties(ctx context.Context, spreadsheetToken string, props *SheetPropertiesUpdate, userAccessToken ...string) error {
	if props == nil || props.SheetID == "" {
		return fmt.Errorf("更新工作表属性失败: sheetId 不能为空")
	}
	if props.Title == nil && props.Index == nil && props.Hidden == nil && props.FrozenRowCount == nil && props.FrozenColCount == nil {
		return fmt.Errorf("更新工作表属性失败: 至少需要指定一个待更新属性")
	}
	_, err := BatchUpdateSheets(ctx, spreadsheetToken, []SheetRequest{{
		UpdateSheet: &UpdateSheetRequest{Properties: props},
	}}, firstString(userAccessToken))
	return err
}

// MoveDimension 移动行/列（v3 move_dimension）。
// 索引口径（与官方 +dim-move 一致）：start/end 从 0 开始、两端包含；destination 为移动后
// 区块首行/首列的目标位置（0 起始）。
// POST /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/move_dimension
func MoveDimension(ctx context.Context, spreadsheetToken, sheetID, majorDimension string, start, end, destination int, userAccessToken ...string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if start < 0 || end < start || destination < 0 {
		return fmt.Errorf("移动行/列失败: 索引须从 0 开始且 end ≥ start、destination ≥ 0，得到 %d..%d → %d", start, end, destination)
	}
	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/move_dimension",
		url.PathEscape(spreadsheetToken), url.PathEscape(sheetID))
	reqBody := map[string]any{
		"source": map[string]any{
			"major_dimension": majorDimension,
			"start_index":     start,
			"end_index":       end,
		},
		"destination_index": destination,
	}
	respBody, err := v2APICallWithToken(cli, ctx, "POST", path, reqBody, firstString(userAccessToken))
	if err != nil {
		return fmt.Errorf("移动行/列失败: %w", err)
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("移动行/列失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return nil
}
