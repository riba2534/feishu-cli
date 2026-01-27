package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larksheets "github.com/larksuite/oapi-sdk-go/v3/service/sheets/v3"
)

// ==================== 数据结构定义 ====================

// SpreadsheetInfo 电子表格基本信息
type SpreadsheetInfo struct {
	SpreadsheetToken string `json:"spreadsheet_token"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	OwnerID          string `json:"owner_id"`
}

// SheetInfo 工作表信息
type SheetInfo struct {
	SheetID    string           `json:"sheet_id"`
	Title      string           `json:"title"`
	Index      int              `json:"index"`
	RowCount   int              `json:"row_count"`
	ColCount   int              `json:"column_count"`
	FrozenRows int              `json:"frozen_row_count"`
	FrozenCols int              `json:"frozen_col_count"`
	Hidden     bool             `json:"hidden"`
	Merges     []*MergeRange    `json:"merges,omitempty"`
	Properties *GridProperties  `json:"grid_properties,omitempty"`
}

// MergeRange 合并单元格范围
type MergeRange struct {
	StartRowIndex    int `json:"start_row_index"`
	EndRowIndex      int `json:"end_row_index"`
	StartColumnIndex int `json:"start_column_index"`
	EndColumnIndex   int `json:"end_column_index"`
}

// GridProperties 网格属性
type GridProperties struct {
	FrozenRowCount    int `json:"frozen_row_count"`
	FrozenColumnCount int `json:"frozen_column_count"`
	RowCount          int `json:"row_count"`
	ColumnCount       int `json:"column_count"`
}

// CellRange 单元格范围数据
type CellRange struct {
	Range  string          `json:"range"`
	Values [][]interface{} `json:"values"`
}

// CellStyle 单元格样式
type CellStyle struct {
	Font       *FontStyle      `json:"font,omitempty"`
	TextFormat *TextFormat     `json:"text_format,omitempty"`
	HAlign     string          `json:"hAlign,omitempty"`     // LEFT, CENTER, RIGHT
	VAlign     string          `json:"vAlign,omitempty"`     // TOP, MIDDLE, BOTTOM
	Formatter  string          `json:"formatter,omitempty"`  // 数字格式
	BgColor    string          `json:"bgColor,omitempty"`    // 背景色
	ForeColor  string          `json:"foreColor,omitempty"`  // 前景色
	BorderType string          `json:"borderType,omitempty"` // 边框类型
	Clean      bool            `json:"clean,omitempty"`      // 是否清除样式
}

// FontStyle 字体样式
type FontStyle struct {
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	FontSize  string `json:"fontSize,omitempty"`
	Clean     bool   `json:"clean,omitempty"`
}

// TextFormat 文本格式
type TextFormat struct {
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Strikethrough bool   `json:"strikethrough,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	FontSize      int    `json:"fontSize,omitempty"`
	ForeColor     string `json:"foreColor,omitempty"`
}

// Dimension 维度信息
type Dimension struct {
	SheetID        string `json:"sheetId"`
	MajorDimension string `json:"majorDimension"` // ROWS or COLUMNS
	StartIndex     int    `json:"startIndex"`
	EndIndex       int    `json:"endIndex"`
}

// FindReplaceResult 查找/替换结果
type FindReplaceResult struct {
	MatchedCells        []string `json:"matched_cells"`
	MatchedFormulaCells []string `json:"matched_formula_cells"`
	RowsCount           int      `json:"rows_count"`
	CellsCount          int      `json:"cells_count"`
}

// SheetBatchUpdateRequest 工作表批量更新请求
type SheetBatchUpdateRequest struct {
	Requests []SheetRequest `json:"requests"`
}

// SheetRequest 单个工作表请求
type SheetRequest struct {
	AddSheet       *AddSheetRequest       `json:"addSheet,omitempty"`
	CopySheet      *CopySheetRequest      `json:"copySheet,omitempty"`
	DeleteSheet    *DeleteSheetRequest    `json:"deleteSheet,omitempty"`
	UpdateSheet    *UpdateSheetRequest    `json:"updateSheet,omitempty"`
}

// AddSheetRequest 添加工作表请求
type AddSheetRequest struct {
	Properties *SheetProperties `json:"properties"`
}

// CopySheetRequest 复制工作表请求
type CopySheetRequest struct {
	Source      *SheetSource `json:"source"`
	Destination *SheetDest   `json:"destination,omitempty"`
}

// DeleteSheetRequest 删除工作表请求
type DeleteSheetRequest struct {
	SheetID string `json:"sheetId"`
}

// UpdateSheetRequest 更新工作表请求
type UpdateSheetRequest struct {
	Properties *SheetProperties `json:"properties"`
}

// SheetProperties 工作表属性
type SheetProperties struct {
	SheetID string `json:"sheetId,omitempty"`
	Title   string `json:"title,omitempty"`
	Index   int    `json:"index,omitempty"`
	Hidden  bool   `json:"hidden,omitempty"`
}

// SheetSource 工作表来源
type SheetSource struct {
	SheetID string `json:"sheetId"`
}

// SheetDest 工作表目标
type SheetDest struct {
	Title string `json:"title,omitempty"`
}

// ProtectedRange 保护范围
type ProtectedRange struct {
	Dimension   *Dimension   `json:"dimension"`
	ProtectID   string       `json:"protectId,omitempty"`
	LockInfo    string       `json:"lockInfo,omitempty"`
	SheetID     string       `json:"sheetId"`
	Editors     *Editors     `json:"editors,omitempty"`
}

// Editors 编辑者
type Editors struct {
	Users       []string `json:"users,omitempty"`
	DepartmentIDs []string `json:"departmentIds,omitempty"`
}

// FloatImage 浮动图片
type FloatImage struct {
	FloatImageID    string  `json:"float_image_id,omitempty"`
	FloatImageToken string  `json:"float_image_token,omitempty"`
	Range           string  `json:"range"`
	Width           float64 `json:"width"`
	Height          float64 `json:"height"`
	OffsetX         float64 `json:"offset_x,omitempty"`
	OffsetY         float64 `json:"offset_y,omitempty"`
}

// FilterInfo 筛选信息
type FilterInfo struct {
	Range         string   `json:"range"`
	FilteredRows  []int    `json:"filtered_out_rows,omitempty"`
	FilterColumns []string `json:"filter_infos,omitempty"`
}

// ==================== V3 API (通过 SDK) ====================

// CreateSpreadsheet 创建电子表格 (V3 API)
func CreateSpreadsheet(ctx context.Context, title string, folderToken string) (*SpreadsheetInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewCreateSpreadsheetReqBuilder().
		Spreadsheet(larksheets.NewSpreadsheetBuilder().
			Title(title).
			FolderToken(folderToken).
			Build()).
		Build()

	resp, err := client.Sheets.Spreadsheet.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("创建电子表格失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建电子表格失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &SpreadsheetInfo{
		SpreadsheetToken: *resp.Data.Spreadsheet.SpreadsheetToken,
		Title:            *resp.Data.Spreadsheet.Title,
		URL:              *resp.Data.Spreadsheet.Url,
	}, nil
}

// GetSpreadsheet 获取电子表格信息 (V3 API)
func GetSpreadsheet(ctx context.Context, spreadsheetToken string) (*SpreadsheetInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewGetSpreadsheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		Build()

	resp, err := client.Sheets.Spreadsheet.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取电子表格信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取电子表格信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	info := &SpreadsheetInfo{
		Title: *resp.Data.Spreadsheet.Title,
	}
	if resp.Data.Spreadsheet.Token != nil {
		info.SpreadsheetToken = *resp.Data.Spreadsheet.Token
	}
	if resp.Data.Spreadsheet.Url != nil {
		info.URL = *resp.Data.Spreadsheet.Url
	}
	if resp.Data.Spreadsheet.OwnerId != nil {
		info.OwnerID = *resp.Data.Spreadsheet.OwnerId
	}

	return info, nil
}

// UpdateSpreadsheetTitle 更新表格标题 (V3 API)
func UpdateSpreadsheetTitle(ctx context.Context, spreadsheetToken, title string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larksheets.NewPatchSpreadsheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		UpdateSpreadsheetProperties(larksheets.NewUpdateSpreadsheetPropertiesBuilder().
			Title(title).
			Build()).
		Build()

	resp, err := client.Sheets.Spreadsheet.Patch(ctx, req)
	if err != nil {
		return fmt.Errorf("更新表格标题失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新表格标题失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// QuerySheets 查询所有工作表 (V3 API)
func QuerySheets(ctx context.Context, spreadsheetToken string) ([]*SheetInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewQuerySpreadsheetSheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		Build()

	resp, err := client.Sheets.SpreadsheetSheet.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("查询工作表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查询工作表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var sheets []*SheetInfo
	for _, s := range resp.Data.Sheets {
		info := &SheetInfo{
			SheetID: *s.SheetId,
			Title:   *s.Title,
		}
		if s.Index != nil {
			info.Index = *s.Index
		}
		if s.Hidden != nil {
			info.Hidden = *s.Hidden
		}
		if s.GridProperties != nil {
			if s.GridProperties.RowCount != nil {
				info.RowCount = *s.GridProperties.RowCount
			}
			if s.GridProperties.ColumnCount != nil {
				info.ColCount = *s.GridProperties.ColumnCount
			}
			if s.GridProperties.FrozenRowCount != nil {
				info.FrozenRows = *s.GridProperties.FrozenRowCount
			}
			if s.GridProperties.FrozenColumnCount != nil {
				info.FrozenCols = *s.GridProperties.FrozenColumnCount
			}
		}
		sheets = append(sheets, info)
	}

	return sheets, nil
}

// GetSheet 获取单个工作表信息 (V3 API)
func GetSheet(ctx context.Context, spreadsheetToken, sheetID string) (*SheetInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewGetSpreadsheetSheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Build()

	resp, err := client.Sheets.SpreadsheetSheet.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取工作表信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取工作表信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	s := resp.Data.Sheet
	info := &SheetInfo{
		SheetID: *s.SheetId,
		Title:   *s.Title,
	}
	if s.Index != nil {
		info.Index = *s.Index
	}
	if s.Hidden != nil {
		info.Hidden = *s.Hidden
	}
	if s.GridProperties != nil {
		if s.GridProperties.RowCount != nil {
			info.RowCount = *s.GridProperties.RowCount
		}
		if s.GridProperties.ColumnCount != nil {
			info.ColCount = *s.GridProperties.ColumnCount
		}
	}

	return info, nil
}

// FindCells 查找单元格 (V3 API)
func FindCells(ctx context.Context, spreadsheetToken, sheetID string, findStr string, matchCase, matchEntireCell, searchByRegex bool, rangeStr string) (*FindReplaceResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	conditionBuilder := larksheets.NewFindConditionBuilder().
		MatchCase(matchCase).
		MatchEntireCell(matchEntireCell).
		SearchByRegex(searchByRegex)

	// 范围需要包含 sheetId 前缀
	if rangeStr != "" {
		fullRange := rangeStr
		if !strings.Contains(rangeStr, "!") {
			fullRange = sheetID + "!" + rangeStr
		}
		conditionBuilder.Range(fullRange)
	}

	req := larksheets.NewFindSpreadsheetSheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Find(larksheets.NewFindBuilder().
			FindCondition(conditionBuilder.Build()).
			Find(findStr).
			Build()).
		Build()

	resp, err := client.Sheets.SpreadsheetSheet.Find(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("查找单元格失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查找单元格失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &FindReplaceResult{}
	if resp.Data.FindResult != nil {
		result.MatchedCells = resp.Data.FindResult.MatchedCells
		result.MatchedFormulaCells = resp.Data.FindResult.MatchedFormulaCells
		if resp.Data.FindResult.RowsCount != nil {
			result.RowsCount = *resp.Data.FindResult.RowsCount
		}
	}

	return result, nil
}

// ReplaceCells 替换单元格内容 (V3 API)
func ReplaceCells(ctx context.Context, spreadsheetToken, sheetID string, findStr, replacement string, matchCase, matchEntireCell bool, rangeStr string) (*FindReplaceResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	conditionBuilder := larksheets.NewFindConditionBuilder().
		MatchCase(matchCase).
		MatchEntireCell(matchEntireCell)

	// 范围需要包含 sheetId 前缀
	if rangeStr != "" {
		fullRange := rangeStr
		if !strings.Contains(rangeStr, "!") {
			fullRange = sheetID + "!" + rangeStr
		}
		conditionBuilder.Range(fullRange)
	}

	req := larksheets.NewReplaceSpreadsheetSheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Replace(larksheets.NewReplaceBuilder().
			FindCondition(conditionBuilder.Build()).
			Find(findStr).
			Replacement(replacement).
			Build()).
		Build()

	resp, err := client.Sheets.SpreadsheetSheet.Replace(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("替换单元格失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("替换单元格失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &FindReplaceResult{}
	if resp.Data.ReplaceResult != nil {
		result.MatchedCells = resp.Data.ReplaceResult.MatchedCells
		result.MatchedFormulaCells = resp.Data.ReplaceResult.MatchedFormulaCells
		if resp.Data.ReplaceResult.RowsCount != nil {
			result.RowsCount = *resp.Data.ReplaceResult.RowsCount
		}
	}

	return result, nil
}

// ==================== V2 API (通过 HTTP 请求) ====================

// v2APICall 封装 V2 API 调用
func v2APICall(client *lark.Client, ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var resp *larkcore.ApiResp
	var err error

	// 注意: SDK 的 Post/Put/Delete 方法接收 interface{} 并在内部进行 JSON 序列化
	// 不要在这里预先序列化，否则会导致双重序列化
	switch method {
	case "GET":
		resp, err = client.Get(ctx, path, nil, larkcore.AccessTokenTypeTenant)
	case "POST":
		resp, err = client.Post(ctx, path, body, larkcore.AccessTokenTypeTenant)
	case "PUT":
		resp, err = client.Put(ctx, path, body, larkcore.AccessTokenTypeTenant)
	case "DELETE":
		resp, err = client.Delete(ctx, path, body, larkcore.AccessTokenTypeTenant)
	default:
		return nil, fmt.Errorf("不支持的 HTTP 方法: %s", method)
	}

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 调用失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	return resp.RawBody, nil
}

// ReadCells 读取单元格数据 (V2 API)
func ReadCells(ctx context.Context, spreadsheetToken, rangeStr string, valueRenderOption, dateTimeRenderOption string) (*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构建 URL
	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values/%s", spreadsheetToken, url.PathEscape(rangeStr))

	// 添加查询参数
	params := url.Values{}
	if valueRenderOption != "" {
		params.Set("valueRenderOption", valueRenderOption)
	}
	if dateTimeRenderOption != "" {
		params.Set("dateTimeRenderOption", dateTimeRenderOption)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	respBody, err := v2APICall(client, ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("读取单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Revision         int             `json:"revision"`
			SpreadsheetToken string          `json:"spreadsheetToken"`
			ValueRange       *CellRange      `json:"valueRange"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("读取单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.ValueRange, nil
}

// ReadCellsBatch 批量读取多个范围 (V2 API)
func ReadCellsBatch(ctx context.Context, spreadsheetToken string, ranges []string, valueRenderOption, dateTimeRenderOption string) ([]*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_batch_get", spreadsheetToken)

	params := url.Values{}
	for _, r := range ranges {
		params.Add("ranges", r)
	}
	if valueRenderOption != "" {
		params.Set("valueRenderOption", valueRenderOption)
	}
	if dateTimeRenderOption != "" {
		params.Set("dateTimeRenderOption", dateTimeRenderOption)
	}
	path += "?" + params.Encode()

	respBody, err := v2APICall(client, ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("批量读取单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Revision    int          `json:"revision"`
			ValueRanges []*CellRange `json:"valueRanges"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("批量读取单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.ValueRanges, nil
}

// WriteCells 写入单元格数据 (V2 API)
func WriteCells(ctx context.Context, spreadsheetToken, rangeStr string, values [][]interface{}) (*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 转换不支持的类型（布尔值转为字符串）
	convertedValues := make([][]interface{}, len(values))
	for i, row := range values {
		convertedValues[i] = make([]interface{}, len(row))
		for j, cell := range row {
			switch v := cell.(type) {
			case bool:
				// API 不支持布尔类型，转换为字符串
				if v {
					convertedValues[i][j] = "TRUE"
				} else {
					convertedValues[i][j] = "FALSE"
				}
			default:
				convertedValues[i][j] = cell
			}
		}
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values", spreadsheetToken)

	reqBody := map[string]interface{}{
		"valueRange": map[string]interface{}{
			"range":  rangeStr,
			"values": convertedValues,
		},
	}

	respBody, err := v2APICall(client, ctx, "PUT", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("写入单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			SpreadsheetToken  string `json:"spreadsheetToken"`
			UpdatedRange      string `json:"updatedRange"`
			UpdatedRows       int    `json:"updatedRows"`
			UpdatedColumns    int    `json:"updatedColumns"`
			UpdatedCells      int    `json:"updatedCells"`
			Revision          int    `json:"revision"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("写入单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return &CellRange{
		Range:  apiResp.Data.UpdatedRange,
		Values: values,
	}, nil
}

// WriteCellsBatch 批量写入多个范围 (V2 API)
func WriteCellsBatch(ctx context.Context, spreadsheetToken string, valueRanges []*CellRange) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_batch_update", spreadsheetToken)

	reqBody := map[string]interface{}{
		"valueRanges": valueRanges,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("批量写入单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("批量写入单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// AppendCells 追加数据 (V2 API)
func AppendCells(ctx context.Context, spreadsheetToken, rangeStr string, values [][]interface{}, insertDataOption string) (*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 转换不支持的类型（布尔值转为字符串）
	convertedValues := make([][]interface{}, len(values))
	for i, row := range values {
		convertedValues[i] = make([]interface{}, len(row))
		for j, cell := range row {
			switch v := cell.(type) {
			case bool:
				if v {
					convertedValues[i][j] = "TRUE"
				} else {
					convertedValues[i][j] = "FALSE"
				}
			default:
				convertedValues[i][j] = cell
			}
		}
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_append", spreadsheetToken)

	if insertDataOption != "" {
		path += "?insertDataOption=" + insertDataOption
	}

	reqBody := map[string]interface{}{
		"valueRange": map[string]interface{}{
			"range":  rangeStr,
			"values": convertedValues,
		},
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("追加数据失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TableRange string `json:"tableRange"`
			Updates    struct {
				SpreadsheetToken string `json:"spreadsheetToken"`
				UpdatedRange     string `json:"updatedRange"`
				UpdatedRows      int    `json:"updatedRows"`
				UpdatedColumns   int    `json:"updatedColumns"`
				UpdatedCells     int    `json:"updatedCells"`
			} `json:"updates"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("追加数据失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return &CellRange{
		Range:  apiResp.Data.Updates.UpdatedRange,
		Values: values,
	}, nil
}

// PrependCells 前置插入数据 (V2 API)
func PrependCells(ctx context.Context, spreadsheetToken, rangeStr string, values [][]interface{}) (*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_prepend", spreadsheetToken)

	reqBody := map[string]interface{}{
		"valueRange": map[string]interface{}{
			"range":  rangeStr,
			"values": values,
		},
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("前置插入数据失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Updates struct {
				UpdatedRange string `json:"updatedRange"`
			} `json:"updates"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("前置插入数据失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return &CellRange{
		Range:  apiResp.Data.Updates.UpdatedRange,
		Values: values,
	}, nil
}

// BatchUpdateSheets 批量更新工作表（添加/删除/复制）(V2 API)
func BatchUpdateSheets(ctx context.Context, spreadsheetToken string, requests []SheetRequest) ([]map[string]interface{}, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/sheets_batch_update", spreadsheetToken)

	reqBody := map[string]interface{}{
		"requests": requests,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("批量更新工作表失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Replies []map[string]interface{} `json:"replies"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("批量更新工作表失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.Replies, nil
}

// AddSheet 添加工作表
func AddSheet(ctx context.Context, spreadsheetToken, title string, index int) (*SheetInfo, error) {
	requests := []SheetRequest{
		{
			AddSheet: &AddSheetRequest{
				Properties: &SheetProperties{
					Title: title,
					Index: index,
				},
			},
		},
	}

	replies, err := BatchUpdateSheets(ctx, spreadsheetToken, requests)
	if err != nil {
		return nil, err
	}

	if len(replies) > 0 {
		if addSheet, ok := replies[0]["addSheet"].(map[string]interface{}); ok {
			if props, ok := addSheet["properties"].(map[string]interface{}); ok {
				info := &SheetInfo{}
				if sheetID, ok := props["sheetId"].(string); ok {
					info.SheetID = sheetID
				}
				if title, ok := props["title"].(string); ok {
					info.Title = title
				}
				if index, ok := props["index"].(float64); ok {
					info.Index = int(index)
				}
				return info, nil
			}
		}
	}

	return nil, fmt.Errorf("添加工作表失败: 无法解析响应")
}

// DeleteSheet 删除工作表
func DeleteSheet(ctx context.Context, spreadsheetToken, sheetID string) error {
	requests := []SheetRequest{
		{
			DeleteSheet: &DeleteSheetRequest{
				SheetID: sheetID,
			},
		},
	}

	_, err := BatchUpdateSheets(ctx, spreadsheetToken, requests)
	return err
}

// CopySheet 复制工作表
func CopySheet(ctx context.Context, spreadsheetToken, sourceSheetID, newTitle string) (*SheetInfo, error) {
	requests := []SheetRequest{
		{
			CopySheet: &CopySheetRequest{
				Source: &SheetSource{
					SheetID: sourceSheetID,
				},
				Destination: &SheetDest{
					Title: newTitle,
				},
			},
		},
	}

	replies, err := BatchUpdateSheets(ctx, spreadsheetToken, requests)
	if err != nil {
		return nil, err
	}

	if len(replies) > 0 {
		if copySheet, ok := replies[0]["copySheet"].(map[string]interface{}); ok {
			if props, ok := copySheet["properties"].(map[string]interface{}); ok {
				info := &SheetInfo{}
				if sheetID, ok := props["sheetId"].(string); ok {
					info.SheetID = sheetID
				}
				if title, ok := props["title"].(string); ok {
					info.Title = title
				}
				return info, nil
			}
		}
	}

	return nil, fmt.Errorf("复制工作表失败: 无法解析响应")
}

// AddDimension 添加行/列 (V2 API)
func AddDimension(ctx context.Context, spreadsheetToken, sheetID string, majorDimension string, length int) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", spreadsheetToken)

	reqBody := map[string]interface{}{
		"dimension": map[string]interface{}{
			"sheetId":        sheetID,
			"majorDimension": majorDimension,
			"length":         length,
		},
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("添加行/列失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("添加行/列失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// InsertDimension 插入行/列 (V2 API)
func InsertDimension(ctx context.Context, spreadsheetToken, sheetID string, majorDimension string, startIndex, endIndex int, inheritStyle string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/insert_dimension_range", spreadsheetToken)

	reqBody := map[string]interface{}{
		"dimension": map[string]interface{}{
			"sheetId":        sheetID,
			"majorDimension": majorDimension,
			"startIndex":     startIndex,
			"endIndex":       endIndex,
		},
	}

	if inheritStyle != "" {
		reqBody["inheritStyle"] = inheritStyle
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("插入行/列失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("插入行/列失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// DeleteDimension 删除行/列 (V2 API)
func DeleteDimension(ctx context.Context, spreadsheetToken, sheetID string, majorDimension string, startIndex, endIndex int) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", spreadsheetToken)

	reqBody := map[string]interface{}{
		"dimension": map[string]interface{}{
			"sheetId":        sheetID,
			"majorDimension": majorDimension,
			"startIndex":     startIndex,
			"endIndex":       endIndex,
		},
	}

	respBody, err := v2APICall(client, ctx, "DELETE", path, reqBody)
	if err != nil {
		return fmt.Errorf("删除行/列失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("删除行/列失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// UpdateDimension 更新行/列属性（如行高、列宽、隐藏等） (V2 API)
func UpdateDimension(ctx context.Context, spreadsheetToken, sheetID string, majorDimension string, startIndex, endIndex int, visible *bool, fixedSize *int) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", spreadsheetToken)

	dimension := map[string]interface{}{
		"sheetId":        sheetID,
		"majorDimension": majorDimension,
		"startIndex":     startIndex,
		"endIndex":       endIndex,
	}

	dimensionProperties := map[string]interface{}{}
	if visible != nil {
		dimensionProperties["visible"] = *visible
	}
	if fixedSize != nil {
		dimensionProperties["fixedSize"] = *fixedSize
	}

	reqBody := map[string]interface{}{
		"dimension":           dimension,
		"dimensionProperties": dimensionProperties,
	}

	respBody, err := v2APICall(client, ctx, "PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("更新行/列属性失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("更新行/列属性失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// MergeCells 合并单元格 (V2 API)
func MergeCells(ctx context.Context, spreadsheetToken, rangeStr, mergeType string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/merge_cells", spreadsheetToken)

	reqBody := map[string]interface{}{
		"range":     rangeStr,
		"mergeType": mergeType, // MERGE_ALL, MERGE_ROWS, MERGE_COLUMNS
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("合并单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("合并单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// UnmergeCells 取消合并单元格 (V2 API)
func UnmergeCells(ctx context.Context, spreadsheetToken, rangeStr string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/unmerge_cells", spreadsheetToken)

	reqBody := map[string]interface{}{
		"range": rangeStr,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("取消合并单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("取消合并单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// SetCellStyle 设置单元格样式 (V2 API)
func SetCellStyle(ctx context.Context, spreadsheetToken, rangeStr string, style *CellStyle) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/style", spreadsheetToken)

	appendStyle := map[string]interface{}{}
	if style.Font != nil {
		appendStyle["font"] = style.Font
	}
	if style.TextFormat != nil {
		appendStyle["textFormat"] = style.TextFormat
	}
	if style.HAlign != "" {
		// API 需要整数值: 0=left, 1=center, 2=right
		hAlignMap := map[string]int{"LEFT": 0, "CENTER": 1, "RIGHT": 2}
		if val, ok := hAlignMap[strings.ToUpper(style.HAlign)]; ok {
			appendStyle["hAlign"] = val
		}
	}
	if style.VAlign != "" {
		// API 需要整数值: 0=top, 1=middle, 2=bottom
		vAlignMap := map[string]int{"TOP": 0, "MIDDLE": 1, "BOTTOM": 2}
		if val, ok := vAlignMap[strings.ToUpper(style.VAlign)]; ok {
			appendStyle["vAlign"] = val
		}
	}
	if style.Formatter != "" {
		appendStyle["formatter"] = style.Formatter
	}
	if style.BgColor != "" {
		appendStyle["bgColor"] = style.BgColor
	}
	if style.ForeColor != "" {
		appendStyle["foreColor"] = style.ForeColor
	}
	if style.BorderType != "" {
		appendStyle["borderType"] = style.BorderType
	}
	if style.Clean {
		appendStyle["clean"] = true
	}

	reqBody := map[string]interface{}{
		"appendStyle": map[string]interface{}{
			"range": rangeStr,
			"style": appendStyle,
		},
	}

	respBody, err := v2APICall(client, ctx, "PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("设置单元格样式失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("设置单元格样式失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// SetCellStyleBatch 批量设置单元格样式 (V2 API)
func SetCellStyleBatch(ctx context.Context, spreadsheetToken string, styles []map[string]interface{}) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/styles_batch_update", spreadsheetToken)

	reqBody := map[string]interface{}{
		"data": styles,
	}

	respBody, err := v2APICall(client, ctx, "PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("批量设置样式失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("批量设置样式失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// GetSpreadsheetMeta 获取表格元信息 (V2 API)
func GetSpreadsheetMeta(ctx context.Context, spreadsheetToken string, extFields string) (map[string]interface{}, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/metainfo", spreadsheetToken)
	if extFields != "" {
		path += "?extFields=" + extFields
	}

	respBody, err := v2APICall(client, ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取表格元信息失败: %w", err)
	}

	var apiResp struct {
		Code int                    `json:"code"`
		Msg  string                 `json:"msg"`
		Data map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("获取表格元信息失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data, nil
}

// ==================== 筛选相关 (V3 API) ====================

// CreateFilter 创建筛选 (V3 API)
func CreateFilter(ctx context.Context, spreadsheetToken, sheetID, rangeStr string, conditions map[string]interface{}) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	// 范围需要包含 sheetId 前缀
	fullRange := rangeStr
	if !strings.Contains(rangeStr, "!") {
		fullRange = sheetID + "!" + rangeStr
	}

	filterBuilder := larksheets.NewCreateSheetFilterBuilder().Range(fullRange)

	req := larksheets.NewCreateSpreadsheetSheetFilterReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		CreateSheetFilter(filterBuilder.Build()).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFilter.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("创建筛选失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("创建筛选失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// GetFilter 获取筛选信息 (V3 API)
func GetFilter(ctx context.Context, spreadsheetToken, sheetID string) (*FilterInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewGetSpreadsheetSheetFilterReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFilter.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取筛选信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取筛选信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	info := &FilterInfo{}
	if resp.Data.SheetFilterInfo != nil {
		if resp.Data.SheetFilterInfo.Range != nil {
			info.Range = *resp.Data.SheetFilterInfo.Range
		}
		info.FilteredRows = resp.Data.SheetFilterInfo.FilteredOutRows
	}

	return info, nil
}

// DeleteFilter 删除筛选 (V3 API)
func DeleteFilter(ctx context.Context, spreadsheetToken, sheetID string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larksheets.NewDeleteSpreadsheetSheetFilterReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFilter.Delete(ctx, req)
	if err != nil {
		return fmt.Errorf("删除筛选失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除筛选失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ==================== 浮动图片相关 (V3 API) ====================

// CreateFloatImage 创建浮动图片 (V3 API)
func CreateFloatImage(ctx context.Context, spreadsheetToken, sheetID string, image *FloatImage) (*FloatImage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	imgBuilder := larksheets.NewFloatImageBuilder().
		FloatImageToken(image.FloatImageToken).
		Range(image.Range).
		Width(image.Width).
		Height(image.Height)

	if image.OffsetX > 0 {
		imgBuilder.OffsetX(image.OffsetX)
	}
	if image.OffsetY > 0 {
		imgBuilder.OffsetY(image.OffsetY)
	}

	req := larksheets.NewCreateSpreadsheetSheetFloatImageReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FloatImage(imgBuilder.Build()).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFloatImage.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("创建浮动图片失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建浮动图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &FloatImage{}
	if resp.Data.FloatImage != nil {
		if resp.Data.FloatImage.FloatImageId != nil {
			result.FloatImageID = *resp.Data.FloatImage.FloatImageId
		}
		if resp.Data.FloatImage.Range != nil {
			result.Range = *resp.Data.FloatImage.Range
		}
		if resp.Data.FloatImage.Width != nil {
			result.Width = *resp.Data.FloatImage.Width
		}
		if resp.Data.FloatImage.Height != nil {
			result.Height = *resp.Data.FloatImage.Height
		}
	}

	return result, nil
}

// DeleteFloatImage 删除浮动图片 (V3 API)
func DeleteFloatImage(ctx context.Context, spreadsheetToken, sheetID, floatImageID string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larksheets.NewDeleteSpreadsheetSheetFloatImageReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FloatImageId(floatImageID).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFloatImage.Delete(ctx, req)
	if err != nil {
		return fmt.Errorf("删除浮动图片失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除浮动图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// QueryFloatImages 查询所有浮动图片 (V3 API)
func QueryFloatImages(ctx context.Context, spreadsheetToken, sheetID string) ([]*FloatImage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larksheets.NewQuerySpreadsheetSheetFloatImageReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		Build()

	resp, err := client.Sheets.SpreadsheetSheetFloatImage.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("查询浮动图片失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查询浮动图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var images []*FloatImage
	for _, img := range resp.Data.Items {
		item := &FloatImage{}
		if img.FloatImageId != nil {
			item.FloatImageID = *img.FloatImageId
		}
		if img.FloatImageToken != nil {
			item.FloatImageToken = *img.FloatImageToken
		}
		if img.Range != nil {
			item.Range = *img.Range
		}
		if img.Width != nil {
			item.Width = *img.Width
		}
		if img.Height != nil {
			item.Height = *img.Height
		}
		images = append(images, item)
	}

	return images, nil
}

// ==================== 保护范围相关 (V2 API) ====================

// CreateProtectedRange 创建保护范围 (V2 API)
func CreateProtectedRange(ctx context.Context, spreadsheetToken string, ranges []*ProtectedRange) ([]string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/protected_dimension", spreadsheetToken)

	var addProtected []map[string]interface{}
	for _, r := range ranges {
		item := map[string]interface{}{
			"dimension": map[string]interface{}{
				"sheetId":        r.SheetID,
				"majorDimension": r.Dimension.MajorDimension,
				"startIndex":     r.Dimension.StartIndex,
				"endIndex":       r.Dimension.EndIndex,
			},
		}
		if r.LockInfo != "" {
			item["lockInfo"] = r.LockInfo
		}
		if r.Editors != nil {
			item["editors"] = r.Editors
		}
		addProtected = append(addProtected, item)
	}

	reqBody := map[string]interface{}{
		"addProtectedDimension": addProtected,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建保护范围失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AddProtectedDimension []struct {
				Dimension struct {
					SheetID   string `json:"sheetId"`
					ProtectID string `json:"protectId"`
				} `json:"dimension"`
			} `json:"addProtectedDimension"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建保护范围失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var protectIDs []string
	for _, item := range apiResp.Data.AddProtectedDimension {
		protectIDs = append(protectIDs, item.Dimension.ProtectID)
	}

	return protectIDs, nil
}

// DeleteProtectedRange 删除保护范围 (V2 API)
func DeleteProtectedRange(ctx context.Context, spreadsheetToken string, protectIDs []string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/protected_range_batch_del", spreadsheetToken)

	reqBody := map[string]interface{}{
		"protectIds": protectIDs,
	}

	respBody, err := v2APICall(client, ctx, "DELETE", path, reqBody)
	if err != nil {
		return fmt.Errorf("删除保护范围失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("删除保护范围失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// ==================== V3 新版单元格 API ====================

// CellElement 单元格元素（V3 API）
type CellElement struct {
	Type            string           `json:"type"`
	Text            *TextElement     `json:"text,omitempty"`
	MentionUser     *MentionUserElem `json:"mention_user,omitempty"`
	MentionDocument *MentionDocElem  `json:"mention_document,omitempty"`
	Value           *ValueElement    `json:"value,omitempty"`
	DateTime        *DateTimeElement `json:"date_time,omitempty"`
	Image           *ImageElement    `json:"image,omitempty"`
	File            *FileElement     `json:"file,omitempty"`
	Link            *LinkElement     `json:"link,omitempty"`
	Reminder        *ReminderElement `json:"reminder,omitempty"`
	Formula         *FormulaElement  `json:"formula,omitempty"`
}

// TextElement 文本元素
type TextElement struct {
	Text         string        `json:"text"`
	SegmentStyle *SegmentStyle `json:"segment_style,omitempty"`
}

// SegmentStyle 局部样式
type SegmentStyle struct {
	Style        *TextStyleV3 `json:"style,omitempty"`
	AffectedText string       `json:"affected_text,omitempty"`
}

// TextStyleV3 文本样式（V3）
type TextStyleV3 struct {
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	StrikeThrough bool   `json:"strike_through,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	ForeColor     string `json:"fore_color,omitempty"`
	FontSize      int    `json:"font_size,omitempty"`
}

// MentionUserElem 提及用户元素
type MentionUserElem struct {
	Name          string        `json:"name,omitempty"`
	UserID        string        `json:"user_id"`
	Notify        bool          `json:"notify,omitempty"`
	SegmentStyles *SegmentStyle `json:"segment_styles,omitempty"`
}

// MentionDocElem 提及文档元素
type MentionDocElem struct {
	Title         string        `json:"title,omitempty"`
	ObjectType    string        `json:"object_type"`
	Token         string        `json:"token"`
	Link          string        `json:"link,omitempty"`
	SegmentStyles *SegmentStyle `json:"segment_styles,omitempty"`
}

// ValueElement 数值元素
type ValueElement struct {
	Value string `json:"value"`
}

// DateTimeElement 日期时间元素
type DateTimeElement struct {
	DateTime string `json:"date_time"`
}

// ImageElement 图片元素
type ImageElement struct {
	ImageToken string `json:"image_token"`
}

// FileElement 附件元素
type FileElement struct {
	FileToken    string        `json:"file_token"`
	Name         string        `json:"name,omitempty"`
	SegmentStyle *SegmentStyle `json:"segment_style,omitempty"`
}

// LinkElement 链接元素
type LinkElement struct {
	Text          string          `json:"text,omitempty"`
	Link          string          `json:"link"`
	SegmentStyles []*SegmentStyle `json:"segment_styles,omitempty"`
}

// ReminderElement 提醒元素
type ReminderElement struct {
	NotifyDateTime string   `json:"notify_date_time"`
	NotifyUserID   []string `json:"notify_user_id,omitempty"`
	NotifyText     string   `json:"notify_text,omitempty"`
	NotifyStrategy int      `json:"notify_strategy"`
}

// FormulaElement 公式元素
type FormulaElement struct {
	Formula       string `json:"formula"`
	FormulaValue  string `json:"formula_value,omitempty"`
	AffectedRange string `json:"affected_range,omitempty"`
}

// CellRangeV3 V3 API 单元格范围数据
type CellRangeV3 struct {
	Range  string            `json:"range"`
	Values [][][]*CellElement `json:"values"` // 三维数组：行 -> 列 -> 元素
}

// ValueRangeV3 V3 API 值范围
type ValueRangeV3 struct {
	Range  string            `json:"range"`
	Values [][][]*CellElement `json:"values"`
}

// WriteCellsV3 写入单元格数据 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/batch_update
func WriteCellsV3(ctx context.Context, spreadsheetToken, sheetID string, valueRanges []*ValueRangeV3, userIDType string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/batch_update", spreadsheetToken, sheetID)
	if userIDType != "" {
		path += "?user_id_type=" + userIDType
	}

	reqBody := map[string]interface{}{
		"value_ranges": valueRanges,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("V3 写入单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("V3 写入单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// InsertCellsV3 插入数据 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/:range/insert
func InsertCellsV3(ctx context.Context, spreadsheetToken, sheetID, rangeStr string, values [][][]*CellElement, userIDType string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/%s/insert",
		spreadsheetToken, sheetID, url.PathEscape(rangeStr))
	if userIDType != "" {
		path += "?user_id_type=" + userIDType
	}

	reqBody := map[string]interface{}{
		"values": values,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("V3 插入数据失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("V3 插入数据失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// AppendCellsV3 追加数据 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/:range/append
func AppendCellsV3(ctx context.Context, spreadsheetToken, sheetID, rangeStr string, values [][][]*CellElement, userIDType string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/%s/append",
		spreadsheetToken, sheetID, url.PathEscape(rangeStr))
	if userIDType != "" {
		path += "?user_id_type=" + userIDType
	}

	reqBody := map[string]interface{}{
		"values": values,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("V3 追加数据失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("V3 追加数据失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// ReadCellsPlainV3 获取纯文本内容 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/batch_get_plain
func ReadCellsPlainV3(ctx context.Context, spreadsheetToken, sheetID string, ranges []string) ([]*CellRange, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/batch_get_plain",
		spreadsheetToken, sheetID)

	reqBody := map[string]interface{}{
		"ranges": ranges,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("V3 获取纯文本失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ValueRanges []struct {
				Range  string     `json:"range"`
				Values [][]string `json:"values"`
			} `json:"value_ranges"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("V3 获取纯文本失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var result []*CellRange
	for _, vr := range apiResp.Data.ValueRanges {
		// 转换 [][]string 为 [][]interface{}
		values := make([][]interface{}, len(vr.Values))
		for i, row := range vr.Values {
			values[i] = make([]interface{}, len(row))
			for j, cell := range row {
				values[i][j] = cell
			}
		}
		result = append(result, &CellRange{
			Range:  vr.Range,
			Values: values,
		})
	}

	return result, nil
}

// ReadCellsRichV3 获取富文本内容 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/batch_get
func ReadCellsRichV3(ctx context.Context, spreadsheetToken, sheetID string, ranges []string, dateTimeRenderOption, valueRenderOption, userIDType string) ([]*CellRangeV3, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/batch_get",
		spreadsheetToken, sheetID)

	// 添加查询参数
	params := url.Values{}
	if dateTimeRenderOption != "" {
		params.Set("datetime_render_option", dateTimeRenderOption)
	}
	if valueRenderOption != "" {
		params.Set("value_render_option", valueRenderOption)
	}
	if userIDType != "" {
		params.Set("user_id_type", userIDType)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	reqBody := map[string]interface{}{
		"ranges": ranges,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("V3 获取富文本失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ValueRanges []*CellRangeV3 `json:"value_ranges"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("V3 获取富文本失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.ValueRanges, nil
}

// ClearCellsV3 清除单元格内容 (V3 API)
// POST /open-apis/sheets/v3/spreadsheets/:spreadsheet_token/sheets/:sheet_id/values/batch_clear
func ClearCellsV3(ctx context.Context, spreadsheetToken, sheetID string, ranges []string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/values/batch_clear",
		spreadsheetToken, sheetID)

	reqBody := map[string]interface{}{
		"ranges": ranges,
	}

	respBody, err := v2APICall(client, ctx, "POST", path, reqBody)
	if err != nil {
		return fmt.Errorf("V3 清除单元格失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("V3 清除单元格失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return nil
}

// ConvertSimpleToV3Values 将简单二维数组转换为 V3 格式的三维数组
func ConvertSimpleToV3Values(values [][]interface{}) [][][]*CellElement {
	result := make([][][]*CellElement, len(values))
	for i, row := range values {
		result[i] = make([][]*CellElement, len(row))
		for j, cell := range row {
			// 每个单元格是一个元素数组
			result[i][j] = []*CellElement{
				ConvertToV3Element(cell),
			}
		}
	}
	return result
}

// ConvertToV3Element 将单个值转换为 V3 元素
func ConvertToV3Element(value interface{}) *CellElement {
	switch v := value.(type) {
	case string:
		return &CellElement{
			Type: "text",
			Text: &TextElement{Text: v},
		}
	case float64:
		return &CellElement{
			Type:  "value",
			Value: &ValueElement{Value: fmt.Sprintf("%v", v)},
		}
	case int:
		return &CellElement{
			Type:  "value",
			Value: &ValueElement{Value: fmt.Sprintf("%d", v)},
		}
	case bool:
		if v {
			return &CellElement{
				Type: "text",
				Text: &TextElement{Text: "TRUE"},
			}
		}
		return &CellElement{
			Type: "text",
			Text: &TextElement{Text: "FALSE"},
		}
	default:
		return &CellElement{
			Type: "text",
			Text: &TextElement{Text: fmt.Sprintf("%v", v)},
		}
	}
}

// ==================== 辅助函数 ====================

// ParseSheetRange 解析范围字符串，返回 sheetID 和范围
// 格式: SheetID!A1:B2 或 A1:B2
func ParseSheetRange(rangeStr string) (sheetID, cellRange string) {
	parts := strings.SplitN(rangeStr, "!", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", rangeStr
}

// BuildSheetRange 构建范围字符串
func BuildSheetRange(sheetID, cellRange string) string {
	if sheetID == "" {
		return cellRange
	}
	return sheetID + "!" + cellRange
}

// ColumnToIndex 将列字母转换为索引（A=0, B=1, ...）
func ColumnToIndex(col string) int {
	col = strings.ToUpper(col)
	result := 0
	for i := 0; i < len(col); i++ {
		result = result*26 + int(col[i]-'A') + 1
	}
	return result - 1
}

// IndexToColumn 将索引转换为列字母（0=A, 1=B, ...）
func IndexToColumn(index int) string {
	result := ""
	index++
	for index > 0 {
		index--
		result = string(rune('A'+index%26)) + result
		index /= 26
	}
	return result
}
