package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ==================== Bitable 数据结构 ====================

// BitableApp 多维表格基本信息
type BitableApp struct {
	AppToken string `json:"app_token"`
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Revision int    `json:"revision,omitempty"`
}

// BitableTable 数据表信息
type BitableTable struct {
	TableID  string `json:"table_id"`
	Name     string `json:"name"`
	Revision int    `json:"revision,omitempty"`
}

// BitableField 字段信息
type BitableField struct {
	FieldID     string              `json:"field_id"`
	FieldName   string              `json:"field_name"`
	Type        int                 `json:"type"`
	UIType      string              `json:"ui_type,omitempty"`
	IsPrimary   bool                `json:"is_primary,omitempty"`
	Description *BitableFieldDesc   `json:"description,omitempty"`
	Property    json.RawMessage     `json:"property,omitempty"`
}

// BitableFieldDesc 字段描述，兼容字符串和对象两种 API 返回格式
type BitableFieldDesc struct {
	Text string
}

func (d *BitableFieldDesc) UnmarshalJSON(data []byte) error {
	// 尝试对象格式 {"text":"..."}
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		d.Text = obj.Text
		return nil
	}
	// 回退到纯字符串格式
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		d.Text = s
		return nil
	}
	return nil
}

func (d BitableFieldDesc) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Text string `json:"text"`
	}{Text: d.Text})
}

// BitableRecord 记录信息
type BitableRecord struct {
	RecordID string         `json:"record_id"`
	Fields   map[string]any `json:"fields"`
}

// BitableView 视图信息
type BitableView struct {
	ViewID   string `json:"view_id"`
	ViewName string `json:"view_name"`
	ViewType string `json:"view_type"`
}

// BitableSearchOptions 记录搜索选项
type BitableSearchOptions struct {
	PageSize  int              `json:"page_size,omitempty"`
	PageToken string           `json:"page_token,omitempty"`
	Filter    *BitableFilter   `json:"filter,omitempty"`
	Sort      []BitableSortItem `json:"sort,omitempty"`
	FieldNames []string        `json:"field_names,omitempty"`
}

// BitableFilter 过滤条件
type BitableFilter struct {
	Conjunction string              `json:"conjunction"` // and / or
	Conditions  []BitableCondition  `json:"conditions"`
}

// BitableCondition 过滤条件项
type BitableCondition struct {
	FieldName string `json:"field_name"`
	Operator  string `json:"operator"` // is, isNot, contains, doesNotContain, isEmpty, isNotEmpty, isGreater, isLess, etc.
	Value     []any  `json:"value,omitempty"`
}

// BitableSortItem 排序项
type BitableSortItem struct {
	FieldName string `json:"field_name"`
	Desc      bool   `json:"desc,omitempty"`
}

const bitableBase = "/open-apis/bitable/v1"

// ==================== 多维表格（App）操作 ====================

// CreateBitableApp 创建多维表格
func CreateBitableApp(name string, folderToken string, userAccessToken string) (*BitableApp, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"name": name,
	}
	if folderToken != "" {
		reqBody["folder_token"] = folderToken
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := bitableBase + "/apps"
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建多维表格失败: %w", err)
	}

	return parseBitableResponse[BitableApp](resp, "创建多维表格")
}

// GetBitableApp 获取多维表格元数据
func GetBitableApp(appToken string, userAccessToken string) (*BitableApp, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s", bitableBase, appToken)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取多维表格信息失败: %w", err)
	}

	return parseBitableResponse[BitableApp](resp, "获取多维表格信息")
}

// ==================== 数据表（Table）操作 ====================

// ListBitableTables 列出数据表
func ListBitableTables(appToken string, userAccessToken string) ([]BitableTable, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables", bitableBase, appToken)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("列出数据表失败: %w", err)
	}

	return parseBitableListResponse[BitableTable](resp, "列出数据表", "items")
}

// CreateBitableTable 创建数据表
func CreateBitableTable(appToken string, name string, userAccessToken string) (*BitableTable, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"table": map[string]any{
			"name": name,
		},
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables", bitableBase, appToken)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建数据表失败: %w", err)
	}

	return parseBitableResponse[BitableTable](resp, "创建数据表")
}

// DeleteBitableTable 删除数据表
func DeleteBitableTable(appToken string, tableID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s", bitableBase, appToken, tableID)
	resp, err := client.Delete(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("删除数据表失败: %w", err)
	}

	return checkBitableError(resp, "删除数据表")
}

// RenameBitableTable 重命名数据表
func RenameBitableTable(appToken string, tableID string, name string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"name": name,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s", bitableBase, appToken, tableID)
	resp, err := client.Patch(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("重命名数据表失败: %w", err)
	}

	return checkBitableError(resp, "重命名数据表")
}

// ==================== 字段（Field）操作 ====================

// ListBitableFields 列出字段
func ListBitableFields(appToken string, tableID string, userAccessToken string) ([]BitableField, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/fields", bitableBase, appToken, tableID)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("列出字段失败: %w", err)
	}

	return parseBitableListResponse[BitableField](resp, "列出字段", "items")
}

// CreateBitableField 创建字段
func CreateBitableField(appToken string, tableID string, fieldDef map[string]any, userAccessToken string) (*BitableField, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/fields", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, fieldDef, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建字段失败: %w", err)
	}

	return parseBitableFieldResponse(resp, "创建字段")
}

// UpdateBitableField 更新字段（注意：单选字段必须带完整 property）
func UpdateBitableField(appToken string, tableID string, fieldID string, fieldDef map[string]any, userAccessToken string) (*BitableField, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/fields/%s", bitableBase, appToken, tableID, fieldID)
	resp, err := client.Put(Context(), apiPath, fieldDef, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("更新字段失败: %w", err)
	}

	return parseBitableFieldResponse(resp, "更新字段")
}

// DeleteBitableField 删除字段
func DeleteBitableField(appToken string, tableID string, fieldID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/fields/%s", bitableBase, appToken, tableID, fieldID)
	resp, err := client.Delete(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("删除字段失败: %w", err)
	}

	return checkBitableError(resp, "删除字段")
}

// ==================== 记录（Record）操作 ====================

// CreateBitableRecord 创建单条记录
func CreateBitableRecord(appToken string, tableID string, fields map[string]any, userAccessToken string) (*BitableRecord, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"fields": fields,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建记录失败: %w", err)
	}

	return parseBitableRecordResponse(resp, "创建记录")
}

// BatchCreateBitableRecords 批量创建记录（最多 500 条）
func BatchCreateBitableRecords(appToken string, tableID string, records []map[string]any, userAccessToken string) ([]BitableRecord, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构造请求体
	items := make([]map[string]any, len(records))
	for i, r := range records {
		items[i] = map[string]any{"fields": r}
	}
	reqBody := map[string]any{
		"records": items,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/batch_create", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("批量创建记录失败: %w", err)
	}

	return parseBitableRecordsResponse(resp, "批量创建记录")
}

// UpdateBitableRecord 更新单条记录
func UpdateBitableRecord(appToken string, tableID string, recordID string, fields map[string]any, userAccessToken string) (*BitableRecord, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"fields": fields,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/%s", bitableBase, appToken, tableID, recordID)
	resp, err := client.Put(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("更新记录失败: %w", err)
	}

	return parseBitableRecordResponse(resp, "更新记录")
}

// BatchUpdateBitableRecords 批量更新记录（最多 500 条）
func BatchUpdateBitableRecords(appToken string, tableID string, records []map[string]any, userAccessToken string) ([]BitableRecord, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"records": records,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/batch_update", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("批量更新记录失败: %w", err)
	}

	return parseBitableRecordsResponse(resp, "批量更新记录")
}

// BatchDeleteBitableRecords 批量删除记录（最多 500 条）
func BatchDeleteBitableRecords(appToken string, tableID string, recordIDs []string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"records": recordIDs,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/batch_delete", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("批量删除记录失败: %w", err)
	}

	return checkBitableError(resp, "批量删除记录")
}

// SearchBitableRecords 搜索记录
func SearchBitableRecords(appToken string, tableID string, opts BitableSearchOptions, userAccessToken string) ([]BitableRecord, string, int, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", 0, err
	}

	reqBody := map[string]any{}
	if opts.PageSize > 0 {
		reqBody["page_size"] = opts.PageSize
	}
	if opts.PageToken != "" {
		reqBody["page_token"] = opts.PageToken
	}
	if opts.Filter != nil {
		reqBody["filter"] = opts.Filter
	}
	if len(opts.Sort) > 0 {
		reqBody["sort"] = opts.Sort
	}
	if len(opts.FieldNames) > 0 {
		reqBody["field_names"] = opts.FieldNames
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/search", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, reqOpts...)
	if err != nil {
		return nil, "", 0, fmt.Errorf("搜索记录失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", 0, fmt.Errorf("搜索记录失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items     []BitableRecord `json:"items"`
			PageToken string          `json:"page_token"`
			HasMore   bool            `json:"has_more"`
			Total     int             `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, "", 0, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, "", 0, fmt.Errorf("搜索记录失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	nextPageToken := ""
	if apiResp.Data.HasMore {
		nextPageToken = apiResp.Data.PageToken
	}

	return apiResp.Data.Items, nextPageToken, apiResp.Data.Total, nil
}

// GetBitableRecord 获取单条记录
func GetBitableRecord(appToken string, tableID string, recordID string, userAccessToken string) (*BitableRecord, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/records/%s", bitableBase, appToken, tableID, recordID)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取记录失败: %w", err)
	}

	return parseBitableRecordResponse(resp, "获取记录")
}

// ==================== 视图（View）操作 ====================

// ListBitableViews 列出视图
func ListBitableViews(appToken string, tableID string, pageSize int, pageToken string, userAccessToken string) ([]BitableView, string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	params := url.Values{}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		params.Set("page_token", pageToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/views", bitableBase, appToken, tableID)
	if len(params) > 0 {
		apiPath += "?" + params.Encode()
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("列出视图失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("列出视图失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items     []BitableView `json:"items"`
			PageToken string        `json:"page_token"`
			HasMore   bool          `json:"has_more"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, "", fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, "", fmt.Errorf("列出视图失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	nextPageToken := ""
	if apiResp.Data.HasMore {
		nextPageToken = apiResp.Data.PageToken
	}

	return apiResp.Data.Items, nextPageToken, nil
}

// CreateBitableView 创建视图
func CreateBitableView(appToken string, tableID string, viewName string, viewType string, userAccessToken string) (*BitableView, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"view_name": viewName,
		"view_type": viewType,
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/views", bitableBase, appToken, tableID)
	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建视图失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建视图失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			View BitableView `json:"view"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建视图失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return &apiResp.Data.View, nil
}

// DeleteBitableView 删除视图
func DeleteBitableView(appToken string, tableID string, viewID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken)
	}

	apiPath := fmt.Sprintf("%s/apps/%s/tables/%s/views/%s", bitableBase, appToken, tableID, viewID)
	resp, err := client.Delete(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("删除视图失败: %w", err)
	}

	return checkBitableError(resp, "删除视图")
}

// ==================== 内部辅助函数 ====================

// checkBitableError 检查 Bitable API 响应错误
func checkBitableError(resp *larkcore.ApiResp, action string) error {
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	return nil
}

// parseBitableResponse 解析 Bitable API 响应（data 直接是对象）
func parseBitableResponse[T any](resp *larkcore.ApiResp, action string) (*T, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			App   *T `json:"app"`
			Table *T `json:"table"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	// 尝试不同的 data 字段名
	if apiResp.Data.App != nil {
		return apiResp.Data.App, nil
	}
	if apiResp.Data.Table != nil {
		return apiResp.Data.Table, nil
	}

	// 直接尝试从 data 解析
	var directResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data T      `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &directResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &directResp.Data, nil
}

// parseBitableListResponse 解析 Bitable API 列表响应
func parseBitableListResponse[T any](resp *larkcore.ApiResp, action string, key string) ([]T, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	// 从 data 中提取 items
	var dataMap map[string]json.RawMessage
	if err := json.Unmarshal(apiResp.Data, &dataMap); err != nil {
		return nil, fmt.Errorf("解析响应数据失败: %w", err)
	}

	itemsRaw, ok := dataMap[key]
	if !ok {
		return []T{}, nil
	}

	var items []T
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return nil, fmt.Errorf("解析列表数据失败: %w", err)
	}

	return items, nil
}

// parseBitableFieldResponse 解析字段响应
func parseBitableFieldResponse(resp *larkcore.ApiResp, action string) (*BitableField, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Field BitableField `json:"field"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	return &apiResp.Data.Field, nil
}

// parseBitableRecordResponse 解析单条记录响应
func parseBitableRecordResponse(resp *larkcore.ApiResp, action string) (*BitableRecord, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Record BitableRecord `json:"record"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	return &apiResp.Data.Record, nil
}

// parseBitableRecordsResponse 解析多条记录响应
func parseBitableRecordsResponse(resp *larkcore.ApiResp, action string) ([]BitableRecord, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Records []BitableRecord `json:"records"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.Records, nil
}
