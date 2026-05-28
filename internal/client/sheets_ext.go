package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	larksheets "github.com/larksuite/oapi-sdk-go/v3/service/sheets/v3"
)

// ==================== 浮动图片：获取 / 更新 / 上传 / 写入 (V3 + drive + V2 values_image) ====================

// GetFloatImage 获取单个浮动图片 (V3 API)。
// GET /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/:float_image_id
func GetFloatImage(ctx context.Context, spreadsheetToken, sheetID, floatImageID string, userAccessToken ...string) (*FloatImage, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewGetSpreadsheetSheetFloatImageReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FloatImageId(floatImageID).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFloatImage.Get(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("获取浮动图片失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("获取浮动图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	out := &FloatImage{}
	if resp.Data != nil {
		sheetFloatImageToLocal(resp.Data.FloatImage, out)
	}
	return out, nil
}

// UpdateFloatImage 更新浮动图片 (V3 API, PATCH)。
// range/width/height 沿用哨兵语义（非空/>0 才写）；offsetX/offsetY 用 *float64 指针表达
// 「是否更新」——nil=不更新，非 nil=更新（含合法值 0）。
// PATCH /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/float_images/:float_image_id
func UpdateFloatImage(ctx context.Context, spreadsheetToken, sheetID, floatImageID string, image *FloatImage, offsetX, offsetY *float64, userAccessToken ...string) (*FloatImage, error) {
	if image == nil {
		return nil, fmt.Errorf("image 不能为 nil")
	}
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	imgBuilder := larksheets.NewFloatImageBuilder()
	if image.Range != "" {
		imgBuilder.Range(image.Range)
	}
	if image.Width > 0 {
		imgBuilder.Width(image.Width)
	}
	if image.Height > 0 {
		imgBuilder.Height(image.Height)
	}
	if offsetX != nil {
		imgBuilder.OffsetX(*offsetX)
	}
	if offsetY != nil {
		imgBuilder.OffsetY(*offsetY)
	}

	req := larksheets.NewPatchSpreadsheetSheetFloatImageReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FloatImageId(floatImageID).
		FloatImage(imgBuilder.Build()).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFloatImage.Patch(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("更新浮动图片失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("更新浮动图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	out := &FloatImage{}
	if resp.Data != nil {
		sheetFloatImageToLocal(resp.Data.FloatImage, out)
	}
	return out, nil
}

// sheetFloatImageToLocal 把 SDK 的 FloatImage 拷贝到本地结构。
func sheetFloatImageToLocal(src *larksheets.FloatImage, dst *FloatImage) {
	if src == nil || dst == nil {
		return
	}
	if src.FloatImageId != nil {
		dst.FloatImageID = *src.FloatImageId
	}
	if src.FloatImageToken != nil {
		dst.FloatImageToken = *src.FloatImageToken
	}
	if src.Range != nil {
		dst.Range = *src.Range
	}
	if src.Width != nil {
		dst.Width = *src.Width
	}
	if src.Height != nil {
		dst.Height = *src.Height
	}
	if src.OffsetX != nil {
		dst.OffsetX = *src.OffsetX
	}
	if src.OffsetY != nil {
		dst.OffsetY = *src.OffsetY
	}
}

// UploadSheetImageMedia 上传本地图片作为浮动图片素材，返回 file_token (drive medias/upload_all)。
// parent_type 固定 sheet_image，parent_node 为电子表格 token。
func UploadSheetImageMedia(filePath, spreadsheetToken, fileName string, userAccessToken ...string) (string, error) {
	token, _, err := UploadMedia(filePath, "sheet_image", spreadsheetToken, fileName, firstString(userAccessToken))
	if err != nil {
		return "", fmt.Errorf("上传浮动图片素材失败: %w", err)
	}
	return token, nil
}

// WriteSheetImage 把本地图片写入指定单元格 (V2 API, values_image)。
// 起止单元格必须相同（单格）。rangeStr 形如 "<sheetId>!A1" 或 "<sheetId>!A1:A1"。
// POST /open-apis/sheets/v2/spreadsheets/:token/values_image
func WriteSheetImage(ctx context.Context, spreadsheetToken, rangeStr, filePath, name string, userAccessToken ...string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	uat := firstString(userAccessToken)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取图片文件失败: %w", err)
	}
	// values_image 的 image 字段是字节数组（[]int）。
	imageBytes := make([]int, len(data))
	for i, b := range data {
		imageBytes[i] = int(b)
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values_image", spreadsheetToken)
	reqBody := map[string]any{
		"range": rangeStr,
		"image": imageBytes,
		"name":  name,
	}

	respBody, err := v2APICallWithToken(cli, ctx, "POST", path, reqBody, uat)
	if err != nil {
		return fmt.Errorf("写入单元格图片失败: %w", err)
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("写入单元格图片失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return nil
}

// ==================== 筛选视图：获取 / 更新 (V3 API) ====================

// GetFilterView 获取单个筛选视图 (V3 API)。
// GET /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id
func GetFilterView(ctx context.Context, spreadsheetToken, sheetID, filterViewID string, userAccessToken ...string) (*FilterViewSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewGetSpreadsheetSheetFilterViewReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterView.Get(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("获取筛选视图失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("获取筛选视图失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	out := &FilterViewSummary{}
	if resp.Data != nil {
		sheetFilterViewToLocal(resp.Data.FilterView, out)
	}
	return out, nil
}

// UpdateFilterView 更新筛选视图（名称 / 范围，V3 API, PATCH）。
// name / rangeStr 任一非空才会写入。
// PATCH /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id
func UpdateFilterView(ctx context.Context, spreadsheetToken, sheetID, filterViewID, name, rangeStr string, userAccessToken ...string) (*FilterViewSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	builder := larksheets.NewFilterViewBuilder()
	if name != "" {
		builder.FilterViewName(name)
	}
	if rangeStr != "" {
		builder.Range(rangeStr)
	}

	req := larksheets.NewPatchSpreadsheetSheetFilterViewReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		FilterView(builder.Build()).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterView.Patch(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("更新筛选视图失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("更新筛选视图失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	out := &FilterViewSummary{}
	if resp.Data != nil {
		sheetFilterViewToLocal(resp.Data.FilterView, out)
	}
	return out, nil
}

func sheetFilterViewToLocal(src *larksheets.FilterView, dst *FilterViewSummary) {
	if src == nil || dst == nil {
		return
	}
	if src.FilterViewId != nil {
		dst.FilterViewID = *src.FilterViewId
	}
	if src.FilterViewName != nil {
		dst.FilterViewName = *src.FilterViewName
	}
	if src.Range != nil {
		dst.Range = *src.Range
	}
}

// ==================== 筛选条件 (filter view condition, V3 API) ====================

// FilterViewConditionSummary 筛选条件摘要。
type FilterViewConditionSummary struct {
	ConditionID string   `json:"condition_id"`
	FilterType  string   `json:"filter_type"`
	CompareType string   `json:"compare_type"`
	Expected    []string `json:"expected"`
}

// sheetBuildConditionBuilder 构造 SDK FilterViewCondition；conditionID 在创建时需带，更新时由路径携带可省略。
func sheetBuildConditionBuilder(conditionID, filterType, compareType string, expected []string) *larksheets.FilterViewConditionBuilder {
	b := larksheets.NewFilterViewConditionBuilder()
	if conditionID != "" {
		b.ConditionId(conditionID)
	}
	if filterType != "" {
		b.FilterType(filterType)
	}
	if compareType != "" {
		b.CompareType(compareType)
	}
	if expected != nil {
		b.Expected(expected)
	}
	return b
}

func sheetConditionToLocal(src *larksheets.FilterViewCondition) *FilterViewConditionSummary {
	out := &FilterViewConditionSummary{}
	if src == nil {
		return out
	}
	if src.ConditionId != nil {
		out.ConditionID = *src.ConditionId
	}
	if src.FilterType != nil {
		out.FilterType = *src.FilterType
	}
	if src.CompareType != nil {
		out.CompareType = *src.CompareType
	}
	if src.Expected != nil {
		out.Expected = src.Expected
	}
	return out
}

// CreateFilterViewCondition 创建筛选条件 (V3 API, POST)。
// POST /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions
func CreateFilterViewCondition(ctx context.Context, spreadsheetToken, sheetID, filterViewID, conditionID, filterType, compareType string, expected []string, userAccessToken ...string) (*FilterViewConditionSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewCreateSpreadsheetSheetFilterViewConditionReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		FilterViewCondition(sheetBuildConditionBuilder(conditionID, filterType, compareType, expected).Build()).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterViewCondition.Create(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("创建筛选条件失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("创建筛选条件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil {
		return sheetConditionToLocal(resp.Data.Condition), nil
	}
	return &FilterViewConditionSummary{}, nil
}

// GetFilterViewCondition 获取单个筛选条件 (V3 API)。
// GET /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id
func GetFilterViewCondition(ctx context.Context, spreadsheetToken, sheetID, filterViewID, conditionID string, userAccessToken ...string) (*FilterViewConditionSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewGetSpreadsheetSheetFilterViewConditionReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		ConditionId(conditionID).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterViewCondition.Get(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("获取筛选条件失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("获取筛选条件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil {
		return sheetConditionToLocal(resp.Data.Condition), nil
	}
	return &FilterViewConditionSummary{}, nil
}

// UpdateFilterViewCondition 更新筛选条件 (V3 API, PUT)。
// conditionID 由路径携带，body 只含 filter_type / compare_type / expected。
// PUT /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id
func UpdateFilterViewCondition(ctx context.Context, spreadsheetToken, sheetID, filterViewID, conditionID, filterType, compareType string, expected []string, userAccessToken ...string) (*FilterViewConditionSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	// 更新时 body 不带 condition_id（由 URL 路径携带）。
	req := larksheets.NewUpdateSpreadsheetSheetFilterViewConditionReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		ConditionId(conditionID).
		FilterViewCondition(sheetBuildConditionBuilder("", filterType, compareType, expected).Build()).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterViewCondition.Update(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("更新筛选条件失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("更新筛选条件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil {
		return sheetConditionToLocal(resp.Data.Condition), nil
	}
	return &FilterViewConditionSummary{}, nil
}

// DeleteFilterViewCondition 删除筛选条件 (V3 API)。
// DELETE /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id
func DeleteFilterViewCondition(ctx context.Context, spreadsheetToken, sheetID, filterViewID, conditionID string, userAccessToken ...string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewDeleteSpreadsheetSheetFilterViewConditionReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		ConditionId(conditionID).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterViewCondition.Delete(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return fmt.Errorf("删除筛选条件失败: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("删除筛选条件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// ListFilterViewConditions 列出筛选视图的所有筛选条件 (V3 API)。
// GET /open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/query
func ListFilterViewConditions(ctx context.Context, spreadsheetToken, sheetID, filterViewID string, userAccessToken ...string) ([]*FilterViewConditionSummary, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	uat := firstString(userAccessToken)

	req := larksheets.NewQuerySpreadsheetSheetFilterViewConditionReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		SheetId(sheetID).
		FilterViewId(filterViewID).
		Build()

	resp, err := cli.Sheets.SpreadsheetSheetFilterViewCondition.Query(ctx, req, UserTokenOption(uat)...)
	if err != nil {
		return nil, fmt.Errorf("查询筛选条件失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询筛选条件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	out := make([]*FilterViewConditionSummary, 0)
	if resp.Data != nil {
		for _, c := range resp.Data.Items {
			out = append(out, sheetConditionToLocal(c))
		}
	}
	return out, nil
}

// ==================== 下拉菜单：获取 / 更新 / 删除 (V2 API) ====================

// GetDropdown 获取指定区域的下拉菜单（数据验证）设置 (V2 API)。
// GET /open-apis/sheets/v2/spreadsheets/:token/dataValidation?range=:range&dataValidationType=list
func GetDropdown(ctx context.Context, spreadsheetToken, rangeStr string, userAccessToken ...string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	if !strings.Contains(rangeStr, "!") {
		return nil, fmt.Errorf("--range 必须包含 sheetId 前缀（例如 <sheetId>!A1:A100）")
	}
	uat := firstString(userAccessToken)

	params := url.Values{}
	params.Set("range", rangeStr)
	params.Set("dataValidationType", "list")
	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dataValidation?%s", spreadsheetToken, params.Encode())

	respBody, err := v2APICallWithToken(cli, ctx, "GET", path, nil, uat)
	if err != nil {
		return nil, fmt.Errorf("获取下拉菜单失败: %w", err)
	}
	var apiResp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("获取下拉菜单失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// UpdateDropdown 更新下拉菜单设置 (V2 API, PUT)。
// ranges 为多个范围（每个需带 sheetId 前缀）；options 为选项；multiple 多选；
// colors 长度需与 options 一致，highlight 启用上色（传 colors 时自动视为高亮）。
// PUT /open-apis/sheets/v2/spreadsheets/:token/dataValidation/:sheet_id
func UpdateDropdown(ctx context.Context, spreadsheetToken, sheetID string, ranges, options []string, multiple bool, colors []string, highlight bool, userAccessToken ...string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return fmt.Errorf("--ranges 至少需要一个范围")
	}
	if len(options) == 0 {
		return fmt.Errorf("下拉选项不能为空")
	}
	if colors != nil && len(colors) != len(options) {
		return fmt.Errorf("--colors 长度(%d)必须与选项数(%d)一致", len(colors), len(options))
	}
	uat := firstString(userAccessToken)

	condValues := make([]any, len(options))
	for i, s := range options {
		condValues[i] = s
	}
	opts := map[string]any{
		"multipleValues": multiple,
	}
	if colors != nil {
		colorVals := make([]any, len(colors))
		for i, c := range colors {
			colorVals[i] = c
		}
		opts["colors"] = colorVals
		opts["highlightValidData"] = true
	} else if highlight {
		opts["highlightValidData"] = true
	}

	rangeVals := make([]any, len(ranges))
	for i, r := range ranges {
		rangeVals[i] = r
	}

	reqBody := map[string]any{
		"ranges":             rangeVals,
		"dataValidationType": "list",
		"dataValidation": map[string]any{
			"conditionValues": condValues,
			"options":         opts,
		},
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dataValidation/%s", spreadsheetToken, sheetID)
	respBody, err := v2APICallWithToken(cli, ctx, "PUT", path, reqBody, uat)
	if err != nil {
		return fmt.Errorf("更新下拉菜单失败: %w", err)
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("更新下拉菜单失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return nil
}

// DeleteDropdown 删除指定范围的下拉菜单（数据验证）(V2 API, DELETE)。
// ranges 每个需带 sheetId 前缀，最多 100 个。
// DELETE /open-apis/sheets/v2/spreadsheets/:token/dataValidation
func DeleteDropdown(ctx context.Context, spreadsheetToken string, ranges []string, userAccessToken ...string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return fmt.Errorf("--ranges 至少需要一个范围")
	}
	uat := firstString(userAccessToken)

	dvRanges := make([]any, len(ranges))
	for i, r := range ranges {
		dvRanges[i] = map[string]any{"range": r}
	}
	reqBody := map[string]any{
		"dataValidationRanges": dvRanges,
	}

	path := fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dataValidation", spreadsheetToken)
	respBody, err := v2APICallWithToken(cli, ctx, "DELETE", path, reqBody, uat)
	if err != nil {
		return fmt.Errorf("删除下拉菜单失败: %w", err)
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("删除下拉菜单失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return nil
}
