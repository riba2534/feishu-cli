package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkokr "github.com/larksuite/oapi-sdk-go/v3/service/okr/v1"
)

// --------- OKR 业务结构（输出层）---------

// OKROwner OKR 所有者，与飞书 OpenAPI 字段对齐
type OKROwner struct {
	OwnerType string `json:"owner_type"`
	UserID    string `json:"user_id,omitempty"`
}

// OKRCycle OKR 周期（v2 接口实体）
type OKRCycle struct {
	ID            string   `json:"id"`
	CreateTime    string   `json:"create_time,omitempty"`
	UpdateTime    string   `json:"update_time,omitempty"`
	TenantCycleID string   `json:"tenant_cycle_id,omitempty"`
	Owner         OKROwner `json:"owner"`
	StartTime     string   `json:"start_time,omitempty"`
	EndTime       string   `json:"end_time,omitempty"`
	CycleStatus   string   `json:"cycle_status,omitempty"`
	Score         *float64 `json:"score,omitempty"`
}

// OKRKeyResult 关键结果
type OKRKeyResult struct {
	ID          string   `json:"id"`
	CreateTime  string   `json:"create_time,omitempty"`
	UpdateTime  string   `json:"update_time,omitempty"`
	Owner       OKROwner `json:"owner"`
	ObjectiveID string   `json:"objective_id"`
	Position    *int32   `json:"position,omitempty"`
	Content     string   `json:"content,omitempty"`
	Score       *float64 `json:"score,omitempty"`
	Weight      *float64 `json:"weight,omitempty"`
	Deadline    string   `json:"deadline,omitempty"`
}

// OKRObjective 目标
type OKRObjective struct {
	ID         string         `json:"id"`
	CreateTime string         `json:"create_time,omitempty"`
	UpdateTime string         `json:"update_time,omitempty"`
	Owner      OKROwner       `json:"owner"`
	CycleID    string         `json:"cycle_id"`
	Position   *int32         `json:"position,omitempty"`
	Content    string         `json:"content,omitempty"`
	Notes      string         `json:"notes,omitempty"`
	Score      *float64       `json:"score,omitempty"`
	Weight     *float64       `json:"weight,omitempty"`
	Deadline   string         `json:"deadline,omitempty"`
	CategoryID string         `json:"category_id,omitempty"`
	KeyResults []OKRKeyResult `json:"key_results,omitempty"`
}

// OKRProgressRate 进度率
type OKRProgressRate struct {
	Percent *float64 `json:"percent,omitempty"`
	Status  string   `json:"status,omitempty"`
}

// OKRProgress OKR 进展记录
type OKRProgress struct {
	ProgressID   string           `json:"progress_id"`
	ModifyTime   string           `json:"modify_time,omitempty"`
	CreateTime   string           `json:"create_time,omitempty"`
	Content      string           `json:"content,omitempty"`
	ProgressRate *OKRProgressRate `json:"progress_rate,omitempty"`
}

// --------- 内部 JSON 反序列化结构（接近 OpenAPI 原始字段）---------

type okrRawOwner struct {
	OwnerType string  `json:"owner_type"`
	UserID    *string `json:"user_id,omitempty"`
}

func (o *okrRawOwner) toOwner() OKROwner {
	if o == nil {
		return OKROwner{}
	}
	out := OKROwner{OwnerType: o.OwnerType}
	if o.UserID != nil {
		out.UserID = *o.UserID
	}
	return out
}

type okrCycleStatus int32

const (
	okrCycleStatusNormal  okrCycleStatus = 1
	okrCycleStatusInvalid okrCycleStatus = 2
	okrCycleStatusHidden  okrCycleStatus = 3
)

func (s okrCycleStatus) String() string {
	switch s {
	case okrCycleStatusNormal:
		return "normal"
	case okrCycleStatusInvalid:
		return "invalid"
	case okrCycleStatusHidden:
		return "hidden"
	}
	return ""
}

type okrRawCycle struct {
	ID            string          `json:"id"`
	CreateTime    string          `json:"create_time,omitempty"`
	UpdateTime    string          `json:"update_time,omitempty"`
	TenantCycleID string          `json:"tenant_cycle_id,omitempty"`
	Owner         okrRawOwner     `json:"owner"`
	StartTime     string          `json:"start_time,omitempty"`
	EndTime       string          `json:"end_time,omitempty"`
	CycleStatus   *okrCycleStatus `json:"cycle_status,omitempty"`
	Score         *float64        `json:"score,omitempty"`
}

func (c *okrRawCycle) toCycle() *OKRCycle {
	if c == nil {
		return nil
	}
	cycle := &OKRCycle{
		ID:            c.ID,
		CreateTime:    formatOKRTimestamp(c.CreateTime),
		UpdateTime:    formatOKRTimestamp(c.UpdateTime),
		TenantCycleID: c.TenantCycleID,
		Owner:         c.Owner.toOwner(),
		StartTime:     formatOKRTimestamp(c.StartTime),
		EndTime:       formatOKRTimestamp(c.EndTime),
		Score:         c.Score,
	}
	if c.CycleStatus != nil {
		cycle.CycleStatus = c.CycleStatus.String()
	}
	return cycle
}

type okrRawObjective struct {
	ID         string          `json:"id"`
	CreateTime string          `json:"create_time,omitempty"`
	UpdateTime string          `json:"update_time,omitempty"`
	Owner      okrRawOwner     `json:"owner"`
	CycleID    string          `json:"cycle_id"`
	Position   *int32          `json:"position,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	Notes      json.RawMessage `json:"notes,omitempty"`
	Score      *float64        `json:"score,omitempty"`
	Weight     *float64        `json:"weight,omitempty"`
	Deadline   *string         `json:"deadline,omitempty"`
	CategoryID *string         `json:"category_id,omitempty"`
}

func (o *okrRawObjective) toObjective() *OKRObjective {
	if o == nil {
		return nil
	}
	out := &OKRObjective{
		ID:         o.ID,
		CreateTime: formatOKRTimestamp(o.CreateTime),
		UpdateTime: formatOKRTimestamp(o.UpdateTime),
		Owner:      o.Owner.toOwner(),
		CycleID:    o.CycleID,
		Position:   o.Position,
		Content:    rawJSONString(o.Content),
		Notes:      rawJSONString(o.Notes),
		Score:      o.Score,
		Weight:     o.Weight,
	}
	if o.Deadline != nil {
		out.Deadline = formatOKRTimestamp(*o.Deadline)
	}
	if o.CategoryID != nil {
		out.CategoryID = *o.CategoryID
	}
	return out
}

type okrRawKeyResult struct {
	ID          string          `json:"id"`
	CreateTime  string          `json:"create_time,omitempty"`
	UpdateTime  string          `json:"update_time,omitempty"`
	Owner       okrRawOwner     `json:"owner"`
	ObjectiveID string          `json:"objective_id"`
	Position    *int32          `json:"position,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Score       *float64        `json:"score,omitempty"`
	Weight      *float64        `json:"weight,omitempty"`
	Deadline    *string         `json:"deadline,omitempty"`
}

func (k *okrRawKeyResult) toKeyResult() *OKRKeyResult {
	if k == nil {
		return nil
	}
	out := &OKRKeyResult{
		ID:          k.ID,
		CreateTime:  formatOKRTimestamp(k.CreateTime),
		UpdateTime:  formatOKRTimestamp(k.UpdateTime),
		Owner:       k.Owner.toOwner(),
		ObjectiveID: k.ObjectiveID,
		Position:    k.Position,
		Content:     rawJSONString(k.Content),
		Score:       k.Score,
		Weight:      k.Weight,
	}
	if k.Deadline != nil {
		out.Deadline = formatOKRTimestamp(*k.Deadline)
	}
	return out
}

// rawJSONString 将 json.RawMessage 转为字符串（保留 JSON 形态）；空值或 null 返回空字符串。
func rawJSONString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
}

// formatOKRTimestamp 把毫秒字符串格式化为 YYYY-MM-DD HH:MM:SS（UTC+8 当地时间）；解析失败返回原值。
func formatOKRTimestamp(ts string) string {
	if ts == "" {
		return ""
	}
	ms, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ts
	}
	// 用 time.UnixMilli + Local 时区，本地展示更直观
	return timeUnixMilliLocalString(ms)
}

// --------- OKR 周期列表（v2，分页拉取）---------

// ListOKRCyclesOptions 列出周期的选项
type ListOKRCyclesOptions struct {
	UserID     string
	UserIDType string // open_id / union_id / user_id
}

// ListOKRCycles 拉取指定用户的所有周期，自动分页（page_size=100）。
// userAccessToken 必填（OKR 接口默认要求 User Token）。
func ListOKRCycles(opts ListOKRCyclesOptions, userAccessToken string) ([]*OKRCycle, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	tokenType, reqOpts := resolveTokenOpts(userAccessToken)

	all := make([]*OKRCycle, 0)
	pageToken := ""
	for page := 0; ; page++ {
		query := larkcore.QueryParams{}
		query.Set("user_id", opts.UserID)
		query.Set("user_id_type", opts.UserIDType)
		query.Set("page_size", "100")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		req := &larkcore.ApiReq{
			HttpMethod:                http.MethodGet,
			ApiPath:                   "/open-apis/okr/v2/cycles",
			QueryParams:               query,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
		}

		resp, err := client.Do(Context(), req, reqOpts...)
		if err != nil {
			return nil, fmt.Errorf("查询 OKR 周期失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("查询 OKR 周期失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Items     []*okrRawCycle `json:"items"`
				HasMore   bool           `json:"has_more"`
				PageToken string         `json:"page_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return nil, fmt.Errorf("解析 OKR 周期响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("查询 OKR 周期失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		for _, item := range apiResp.Data.Items {
			if c := item.toCycle(); c != nil {
				all = append(all, c)
			}
		}

		if !apiResp.Data.HasMore || apiResp.Data.PageToken == "" {
			break
		}
		pageToken = apiResp.Data.PageToken

		// 防御性：上限 200 页（2 万条），避免异常响应造成死循环
		if page >= 200 {
			break
		}
	}

	return all, nil
}

// --------- 周期详情：拉目标 + 每个目标的关键结果 ---------

// GetOKRCycleDetail 拉取一个周期下所有目标及其关键结果，自动分页。
func GetOKRCycleDetail(cycleID string, userAccessToken string) ([]*OKRObjective, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, reqOpts := resolveTokenOpts(userAccessToken)

	rawObjectives, err := listOKRCycleObjectives(client, tokenType, reqOpts, cycleID)
	if err != nil {
		return nil, err
	}

	out := make([]*OKRObjective, 0, len(rawObjectives))
	for _, ro := range rawObjectives {
		obj := ro.toObjective()
		if obj == nil {
			continue
		}
		krs, err := listOKRObjectiveKeyResults(client, tokenType, reqOpts, obj.ID)
		if err != nil {
			return nil, fmt.Errorf("查询目标 %s 的关键结果失败: %w", obj.ID, err)
		}
		obj.KeyResults = krs
		out = append(out, obj)
	}
	return out, nil
}

func listOKRCycleObjectives(client okrHTTPDoer, tokenType larkcore.AccessTokenType, reqOpts []larkcore.RequestOptionFunc, cycleID string) ([]*okrRawObjective, error) {
	all := make([]*okrRawObjective, 0)
	pageToken := ""
	for page := 0; ; page++ {
		query := larkcore.QueryParams{}
		query.Set("page_size", "100")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		req := &larkcore.ApiReq{
			HttpMethod:                http.MethodGet,
			ApiPath:                   fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID),
			QueryParams:               query,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
		}

		resp, err := client.Do(Context(), req, reqOpts...)
		if err != nil {
			return nil, fmt.Errorf("查询周期目标失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("查询周期目标失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Items     []*okrRawObjective `json:"items"`
				HasMore   bool               `json:"has_more"`
				PageToken string             `json:"page_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return nil, fmt.Errorf("解析周期目标响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("查询周期目标失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		all = append(all, apiResp.Data.Items...)
		if !apiResp.Data.HasMore || apiResp.Data.PageToken == "" {
			break
		}
		pageToken = apiResp.Data.PageToken
		if page >= 200 {
			break
		}
	}
	return all, nil
}

func listOKRObjectiveKeyResults(client okrHTTPDoer, tokenType larkcore.AccessTokenType, reqOpts []larkcore.RequestOptionFunc, objectiveID string) ([]OKRKeyResult, error) {
	all := make([]OKRKeyResult, 0)
	pageToken := ""
	for page := 0; ; page++ {
		query := larkcore.QueryParams{}
		query.Set("page_size", "100")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		req := &larkcore.ApiReq{
			HttpMethod:                http.MethodGet,
			ApiPath:                   fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", objectiveID),
			QueryParams:               query,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
		}

		resp, err := client.Do(Context(), req, reqOpts...)
		if err != nil {
			return nil, fmt.Errorf("查询目标关键结果失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("查询目标关键结果失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Items     []*okrRawKeyResult `json:"items"`
				HasMore   bool               `json:"has_more"`
				PageToken string             `json:"page_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return nil, fmt.Errorf("解析目标关键结果响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("查询目标关键结果失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		for _, item := range apiResp.Data.Items {
			if kr := item.toKeyResult(); kr != nil {
				all = append(all, *kr)
			}
		}

		if !apiResp.Data.HasMore || apiResp.Data.PageToken == "" {
			break
		}
		pageToken = apiResp.Data.PageToken
		if page >= 200 {
			break
		}
	}
	return all, nil
}

// --------- OKR 进展（v1，走 SDK；List 走 v2 HTTP）---------

// OKRProgressTargetType OKR 进展挂载的实体类型
type OKRProgressTargetType int

const (
	// OKRTargetObjective 目标 Objective
	OKRTargetObjective OKRProgressTargetType = 2
	// OKRTargetKeyResult 关键结果 Key Result
	OKRTargetKeyResult OKRProgressTargetType = 3
)

// ParseOKRTargetType 把 objective / key_result 文本转为枚举
func ParseOKRTargetType(s string) (OKRProgressTargetType, bool) {
	switch s {
	case "objective":
		return OKRTargetObjective, true
	case "key_result":
		return OKRTargetKeyResult, true
	}
	return 0, false
}

// OKRProgressStatus 进展状态
type OKRProgressStatus int

const (
	OKRProgressStatusNormal  OKRProgressStatus = 0
	OKRProgressStatusOverdue OKRProgressStatus = 1
	OKRProgressStatusDone    OKRProgressStatus = 2
)

// ParseOKRProgressStatus 把 normal / overdue / done 转为枚举
func ParseOKRProgressStatus(s string) (OKRProgressStatus, bool) {
	switch s {
	case "normal", "0":
		return OKRProgressStatusNormal, true
	case "overdue", "1":
		return OKRProgressStatusOverdue, true
	case "done", "2":
		return OKRProgressStatusDone, true
	}
	return 0, false
}

func (s OKRProgressStatus) String() string {
	switch s {
	case OKRProgressStatusNormal:
		return "normal"
	case OKRProgressStatusOverdue:
		return "overdue"
	case OKRProgressStatusDone:
		return "done"
	}
	return ""
}

// CreateOKRProgressOptions 创建进展记录的选项
type CreateOKRProgressOptions struct {
	// ContentJSON 是 ContentBlock 富文本 JSON 字符串（v1 OKR 接口格式）。
	// 可以传 v1 形态（type/textRun/...）也可以传 v2 形态（block_element_type/...），
	// 内部会原样下发；推荐外部直接构造 v1 形态以避免歧义。
	ContentJSON  string
	TargetID     string
	TargetType   OKRProgressTargetType
	SourceTitle  string
	SourceURL    string
	UserIDType   string
	ProgressRate *OKRProgressRateInput
}

// OKRProgressRateInput 进度率输入；Percent 必填，Status 可空
type OKRProgressRateInput struct {
	Percent float64
	Status  *OKRProgressStatus
}

// CreateOKRProgress 创建一条 OKR 进展记录。
// 走 v3 SDK 的 ProgressRecord.Create，内容字段使用 SDK 的 ContentBlock 结构；
// 但 SDK 的 ContentBlock JSON tag 用的是 v1 字段名（type/textRun/elements/...），
// 与 lark-cli 的 v1 形态一致，因此外部传入的 JSON 串以 v1 格式为准。
func CreateOKRProgress(opts CreateOKRProgressOptions, userAccessToken string) (*OKRProgress, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	var contentBlock *larkokr.ContentBlock
	if strings.TrimSpace(opts.ContentJSON) != "" {
		contentBlock = &larkokr.ContentBlock{}
		if err := json.Unmarshal([]byte(opts.ContentJSON), contentBlock); err != nil {
			return nil, fmt.Errorf("解析 ContentBlock JSON 失败: %w", err)
		}
	}

	if opts.SourceTitle == "" {
		opts.SourceTitle = "created by feishu-cli"
	}
	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	bodyBuilder := larkokr.NewCreateProgressRecordReqBodyBuilder().
		SourceTitle(opts.SourceTitle).
		TargetId(opts.TargetID).
		TargetType(int(opts.TargetType))
	if opts.SourceURL != "" {
		bodyBuilder = bodyBuilder.SourceUrl(opts.SourceURL)
	}
	if contentBlock != nil {
		bodyBuilder = bodyBuilder.Content(contentBlock)
	}
	if opts.ProgressRate != nil {
		rateBuilder := larkokr.NewProgressRateNewBuilder().Percent(opts.ProgressRate.Percent)
		if opts.ProgressRate.Status != nil {
			rateBuilder = rateBuilder.Status(int(*opts.ProgressRate.Status))
		}
		bodyBuilder = bodyBuilder.ProgressRate(rateBuilder.Build())
	}

	req := larkokr.NewCreateProgressRecordReqBuilder().
		UserIdType(opts.UserIDType).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Okr.ProgressRecord.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("创建 OKR 进展失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("创建 OKR 进展失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("创建 OKR 进展失败: 接口未返回数据")
	}
	return progressRecordFieldsToOut(resp.Data.ProgressId, resp.Data.ModifyTime, resp.Data.Content, resp.Data.ProgressRate), nil
}

// UpdateOKRProgressOptions 更新进展记录的选项
type UpdateOKRProgressOptions struct {
	ProgressID   string
	ContentJSON  string
	UserIDType   string
	ProgressRate *OKRProgressRateInput
}

// UpdateOKRProgress 更新一条 OKR 进展记录
func UpdateOKRProgress(opts UpdateOKRProgressOptions, userAccessToken string) (*OKRProgress, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	var contentBlock *larkokr.ContentBlock
	if strings.TrimSpace(opts.ContentJSON) != "" {
		contentBlock = &larkokr.ContentBlock{}
		if err := json.Unmarshal([]byte(opts.ContentJSON), contentBlock); err != nil {
			return nil, fmt.Errorf("解析 ContentBlock JSON 失败: %w", err)
		}
	}

	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	bodyBuilder := larkokr.NewUpdateProgressRecordReqBodyBuilder()
	if contentBlock != nil {
		bodyBuilder = bodyBuilder.Content(contentBlock)
	}
	if opts.ProgressRate != nil {
		rateBuilder := larkokr.NewProgressRateNewBuilder().Percent(opts.ProgressRate.Percent)
		if opts.ProgressRate.Status != nil {
			rateBuilder = rateBuilder.Status(int(*opts.ProgressRate.Status))
		}
		bodyBuilder = bodyBuilder.ProgressRate(rateBuilder.Build())
	}

	req := larkokr.NewUpdateProgressRecordReqBuilder().
		ProgressId(opts.ProgressID).
		UserIdType(opts.UserIDType).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Okr.ProgressRecord.Update(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("更新 OKR 进展失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("更新 OKR 进展失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("更新 OKR 进展失败: 接口未返回数据")
	}
	return progressRecordFieldsToOut(resp.Data.ProgressId, resp.Data.ModifyTime, resp.Data.Content, resp.Data.ProgressRate), nil
}

// GetOKRProgress 根据 ID 拉取一条进展记录
func GetOKRProgress(progressID string, userIDType string, userAccessToken string) (*OKRProgress, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}
	if userIDType == "" {
		userIDType = "open_id"
	}

	req := larkokr.NewGetProgressRecordReqBuilder().
		ProgressId(progressID).
		UserIdType(userIDType).
		Build()

	resp, err := client.Okr.ProgressRecord.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("查询 OKR 进展失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询 OKR 进展失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("查询 OKR 进展失败: 接口未返回数据")
	}
	return progressRecordFieldsToOut(resp.Data.ProgressId, resp.Data.ModifyTime, resp.Data.Content, resp.Data.ProgressRate), nil
}

// DeleteOKRProgress 删除一条进展记录
func DeleteOKRProgress(progressID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkokr.NewDeleteProgressRecordReqBuilder().
		ProgressId(progressID).
		Build()

	resp, err := client.Okr.ProgressRecord.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("删除 OKR 进展失败: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("删除 OKR 进展失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// ListOKRProgressesOptions 列出 Objective/KeyResult 的进展记录
type ListOKRProgressesOptions struct {
	TargetID         string
	TargetType       OKRProgressTargetType
	UserIDType       string
	DepartmentIDType string
}

// ListOKRProgresses 拉取一个 Objective 或 KeyResult 下的所有进展记录（v2 接口，自动分页）。
func ListOKRProgresses(opts ListOKRProgressesOptions, userAccessToken string) ([]*OKRProgress, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}
	if opts.DepartmentIDType == "" {
		opts.DepartmentIDType = "open_department_id"
	}

	tokenType, reqOpts := resolveTokenOpts(userAccessToken)

	var apiPath string
	switch opts.TargetType {
	case OKRTargetObjective:
		apiPath = fmt.Sprintf("/open-apis/okr/v2/objectives/%s/progresses", opts.TargetID)
	case OKRTargetKeyResult:
		apiPath = fmt.Sprintf("/open-apis/okr/v2/key_results/%s/progresses", opts.TargetID)
	default:
		return nil, fmt.Errorf("不支持的 target-type: %d", opts.TargetType)
	}

	all := make([]*OKRProgress, 0)
	pageToken := ""
	for page := 0; ; page++ {
		query := larkcore.QueryParams{}
		query.Set("user_id_type", opts.UserIDType)
		query.Set("department_id_type", opts.DepartmentIDType)
		query.Set("page_size", "100")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		req := &larkcore.ApiReq{
			HttpMethod:                http.MethodGet,
			ApiPath:                   apiPath,
			QueryParams:               query,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
		}

		resp, err := client.Do(Context(), req, reqOpts...)
		if err != nil {
			return nil, fmt.Errorf("查询 OKR 进展列表失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("查询 OKR 进展列表失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Items []struct {
					ID           string          `json:"id"`
					CreateTime   string          `json:"create_time,omitempty"`
					UpdateTime   string          `json:"update_time,omitempty"`
					Owner        okrRawOwner     `json:"owner"`
					EntityType   *int32          `json:"entity_type,omitempty"`
					EntityID     string          `json:"entity_id,omitempty"`
					Content      json.RawMessage `json:"content,omitempty"`
					ProgressRate *struct {
						ProgressPercent *float64 `json:"progress_percent,omitempty"`
						ProgressStatus  *int     `json:"progress_status,omitempty"`
					} `json:"progress_rate,omitempty"`
				} `json:"items"`
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return nil, fmt.Errorf("解析 OKR 进展列表响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, fmt.Errorf("查询 OKR 进展列表失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		for _, item := range apiResp.Data.Items {
			p := &OKRProgress{
				ProgressID: item.ID,
				ModifyTime: formatOKRTimestamp(item.UpdateTime),
				CreateTime: formatOKRTimestamp(item.CreateTime),
				Content:    rawJSONString(item.Content),
			}
			if item.ProgressRate != nil {
				p.ProgressRate = &OKRProgressRate{
					Percent: item.ProgressRate.ProgressPercent,
				}
				if item.ProgressRate.ProgressStatus != nil {
					p.ProgressRate.Status = OKRProgressStatus(*item.ProgressRate.ProgressStatus).String()
				}
			}
			all = append(all, p)
		}

		if !apiResp.Data.HasMore || apiResp.Data.PageToken == "" {
			break
		}
		pageToken = apiResp.Data.PageToken
		if page >= 200 {
			break
		}
	}
	return all, nil
}

// --------- OKR 图片上传（multipart）---------

// OKRImageUploadResult 上传图片返回
type OKRImageUploadResult struct {
	FileToken string `json:"file_token"`
	URL       string `json:"url,omitempty"`
	FileName  string `json:"file_name"`
	Size      int64  `json:"size"`
}

// UploadOKRImage 上传一张图片到 OKR Progress 富文本图床。
// 通过 SDK 的 Image.Upload 走 multipart/form-data。
func UploadOKRImage(filePath string, targetID string, targetType OKRProgressTargetType, userAccessToken string) (*OKRImageUploadResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("获取图片信息失败: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("--file 不能是目录: %s", filePath)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()

	fileName := filepath.Base(filePath)

	req := larkokr.NewUploadImageReqBuilder().
		Body(larkokr.NewUploadImageReqBodyBuilder().
			Data(f).
			TargetId(targetID).
			TargetType(int(targetType)).
			Build()).
		Build()

	resp, err := client.Okr.Image.Upload(ContextWithTimeout(downloadTimeout), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("上传 OKR 图片失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("上传 OKR 图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("上传 OKR 图片失败: 接口未返回数据")
	}

	out := &OKRImageUploadResult{
		FileToken: StringVal(resp.Data.FileToken),
		URL:       StringVal(resp.Data.Url),
		FileName:  fileName,
		Size:      info.Size(),
	}
	if out.FileToken == "" {
		return nil, fmt.Errorf("上传 OKR 图片失败: 接口未返回 file_token")
	}
	return out, nil
}

// --------- 内部辅助 ---------

// okrHTTPDoer 仅在内部用于测试时 mock client.Do
type okrHTTPDoer interface {
	Do(ctx context.Context, req *larkcore.ApiReq, options ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error)
}

// timeUnixMilliLocalString 把毫秒时间戳格式化为本地时区 "2006-01-02 15:04:05" 字符串
func timeUnixMilliLocalString(ms int64) string {
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

// progressRecordFieldsToOut 把 SDK 中形态相同的 Progress 响应（ProgressRecord /
// CreateProgressRecordRespData / UpdateProgressRecordRespData / GetProgressRecordRespData）
// 拆出 4 个核心字段后归一化为对外 OKRProgress。
//
// 这些 SDK 类型 schema 完全一致但走的是不同 Go 结构体，没法在调用侧共享一个 *ProgressRecord。
// 改成传字段而非整个对象，避免每加一个响应类型就要写一个 to* helper。
func progressRecordFieldsToOut(progressID, modifyTime *string, content *larkokr.ContentBlock, rate *larkokr.ProgressRateNew) *OKRProgress {
	out := &OKRProgress{
		ProgressID: StringVal(progressID),
		ModifyTime: formatOKRTimestamp(StringVal(modifyTime)),
	}
	if rate != nil {
		r := &OKRProgressRate{Percent: rate.Percent}
		if rate.Status != nil {
			r.Status = OKRProgressStatus(*rate.Status).String()
		}
		out.ProgressRate = r
	}
	if content != nil {
		if data, err := json.Marshal(content); err == nil {
			out.Content = string(data)
		}
	}
	return out
}
