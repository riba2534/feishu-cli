package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// 官方当前 v4 用户态契约（open.feishu.cn 动态元数据）。
const (
	approvalDefinitionDetailPath  = "/open-apis/approval/v4/approvals/:approval_code/detail"
	approvalInstanceDetailPath    = "/open-apis/approval/v4/instances/detail"
	approvalInstanceInitiatePath  = "/open-apis/approval/v4/instances/initiate"
	approvalInstanceRecallPath    = "/open-apis/approval/v4/instances/recall"
	approvalInstanceAddCCPath     = "/open-apis/approval/v4/instances/add_cc"
	approvalInstanceInitiatedPath = "/open-apis/approval/v4/instances/initiated"
	approvalTaskListPath          = "/open-apis/approval/v4/tasks"
	approvalTaskPassPath          = "/open-apis/approval/v4/tasks/pass"
	approvalTaskRefusePath        = "/open-apis/approval/v4/tasks/refuse"
	approvalTaskForwardPath       = "/open-apis/approval/v4/tasks/forward"
)

// GetApprovalOptions represents optional filters for fetching an approval definition.
type GetApprovalOptions struct {
	Locale string
}

// ApprovalDefinition represents a simplified approval definition.
type ApprovalDefinition struct {
	ApprovalCode string          `json:"approval_code"`
	ApprovalName string          `json:"approval_name"`
	Form         any             `json:"form,omitempty"`
	NodeList     []*ApprovalNode `json:"node_list,omitempty"`
}

// ApprovalNode represents a simplified approval node.
type ApprovalNode struct {
	Name                string `json:"name,omitempty"`
	NodeID              string `json:"node_id,omitempty"`
	CustomNodeID        string `json:"custom_node_id,omitempty"`
	NodeType            string `json:"node_type,omitempty"`
	NeedApprover        bool   `json:"need_approver,omitempty"`
	ApproverChosenMulti bool   `json:"approver_chosen_multi,omitempty"`
	RequireSignature    bool   `json:"require_signature,omitempty"`
	ApproverChosenRange any    `json:"approver_chosen_range,omitempty"`
}

// ApprovalTaskQueryOptions represents options for querying approval tasks.
type ApprovalTaskQueryOptions struct {
	PageSize       int
	PageToken      string
	Topic          string
	Locale         string
	DefinitionCode string
	StartTimestamp string
	EndTimestamp   string
	UserIDType     string
}

// ApprovalTaskQueryResult represents a simplified approval task query result.
type ApprovalTaskQueryResult struct {
	Tasks     []*ApprovalTaskInfo `json:"tasks"`
	PageToken string              `json:"page_token,omitempty"`
	HasMore   bool                `json:"has_more"`
	Count     *int                `json:"count,omitempty"`
}

// ApprovalSummary is a form summary field from task/initiated list APIs.
type ApprovalSummary struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// ApprovalTaskInfo represents a simplified approval task.
type ApprovalTaskInfo struct {
	Topic               string            `json:"topic,omitempty"`
	UserID              string            `json:"user_id,omitempty"`
	Title               string            `json:"title,omitempty"`
	Status              string            `json:"status,omitempty"`
	DefinitionCode      string            `json:"definition_code,omitempty"`
	DefinitionName      string            `json:"definition_name,omitempty"`
	DefinitionGroupID   string            `json:"definition_group_id,omitempty"`
	DefinitionGroupName string            `json:"definition_group_name,omitempty"`
	TaskID              string            `json:"task_id,omitempty"`
	InstanceCode        string            `json:"instance_code,omitempty"`
	InstanceStatus      string            `json:"instance_status,omitempty"`
	InstanceExternalID  string            `json:"instance_external_id,omitempty"`
	TaskExternalID      string            `json:"task_external_id,omitempty"`
	Initiator           string            `json:"initiator,omitempty"`
	InitiatorName       string            `json:"initiator_name,omitempty"`
	Summaries           []ApprovalSummary `json:"summaries,omitempty"`
	SupportAPIOperate   bool              `json:"support_api_operate"`
	Link                string            `json:"link,omitempty"`
}

// GetApprovalInstanceOptions represents options for fetching one approval instance.
type GetApprovalInstanceOptions struct {
	InstanceCode string
	Locale       string
	UserIDType   string
}

// ListInitiatedApprovalInstancesOptions represents options for listing initiated instances.
type ListInitiatedApprovalInstancesOptions struct {
	PageSize       int
	PageToken      string
	Locale         string
	DefinitionCode string
	StartTimestamp string
	EndTimestamp   string
	UserIDType     string
}

// ApprovalInitiatedResult represents initiated approval instances for the current user.
type ApprovalInitiatedResult struct {
	Instances []*ApprovalInitiatedInstance `json:"instances"`
	PageToken string                       `json:"page_token,omitempty"`
	HasMore   bool                         `json:"has_more"`
	Count     *int                         `json:"count,omitempty"`
}

// ApprovalInitiatedInstance is one initiated approval instance.
type ApprovalInitiatedInstance struct {
	InstanceCode        string            `json:"instance_code,omitempty"`
	DefinitionCode      string            `json:"definition_code,omitempty"`
	DefinitionName      string            `json:"definition_name,omitempty"`
	DefinitionGroupID   string            `json:"definition_group_id,omitempty"`
	DefinitionGroupName string            `json:"definition_group_name,omitempty"`
	Initiator           string            `json:"initiator,omitempty"`
	InitiatorName       string            `json:"initiator_name,omitempty"`
	InstanceStatus      string            `json:"instance_status,omitempty"`
	InstanceExternalID  string            `json:"instance_external_id,omitempty"`
	Link                string            `json:"link,omitempty"`
	Summaries           []ApprovalSummary `json:"summaries,omitempty"`
}

// TransferApprovalTaskOptions represents options for transferring an approval task.
type TransferApprovalTaskOptions struct {
	InstanceCode   string // 必填：审批实例 code
	TaskID         string // 必填：审批任务 ID
	TransferUserID string // 必填：被转交用户 ID
	Comment        string // 可选：审批意见
	UserIDType     string // 可选：open_id / user_id / union_id，默认 open_id
}

type approvalTaskString string

func (s *approvalTaskString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = approvalTaskString(str)
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var number json.Number
	if err := decoder.Decode(&number); err == nil {
		*s = approvalTaskString(number.String())
		return nil
	}

	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		*s = approvalTaskString(strconv.FormatBool(boolean))
		return nil
	}

	return fmt.Errorf("不支持的审批任务字段类型: %s", string(data))
}

func (s approvalTaskString) String() string {
	return string(s)
}

type approvalTaskQueryAPIResp struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg"`
	Data *approvalTaskQueryAPIData `json:"data"`
}

type approvalTaskQueryAPIData struct {
	Tasks     []*approvalTaskAPIInfo `json:"tasks,omitempty"`
	PageToken string                 `json:"page_token,omitempty"`
	HasMore   bool                   `json:"has_more,omitempty"`
	Count     *int                   `json:"count,omitempty"`
}

type approvalTaskAPIInfo struct {
	Topic               approvalTaskString `json:"topic,omitempty"`
	UserID              approvalTaskString `json:"user_id,omitempty"`
	Title               approvalTaskString `json:"title,omitempty"`
	Status              approvalTaskString `json:"status,omitempty"`
	DefinitionCode      approvalTaskString `json:"definition_code,omitempty"`
	DefinitionName      approvalTaskString `json:"definition_name,omitempty"`
	DefinitionGroupID   approvalTaskString `json:"definition_group_id,omitempty"`
	DefinitionGroupName approvalTaskString `json:"definition_group_name,omitempty"`
	Initiator           approvalTaskString `json:"initiator,omitempty"`
	InitiatorName       approvalTaskString `json:"initiator_name,omitempty"`
	TaskID              approvalTaskString `json:"task_id,omitempty"`
	InstanceCode        approvalTaskString `json:"instance_code,omitempty"`
	InstanceStatus      approvalTaskString `json:"instance_status,omitempty"`
	InstanceExternalID  approvalTaskString `json:"instance_external_id,omitempty"`
	TaskExternalID      approvalTaskString `json:"task_external_id,omitempty"`
	Summaries           []ApprovalSummary  `json:"summaries,omitempty"`
	SupportAPIOperate   bool               `json:"support_api_operate,omitempty"`
	Link                approvalTaskString `json:"link,omitempty"`
}

type approvalInitiatedAPIResp struct {
	Code int                       `json:"code"`
	Msg  string                    `json:"msg"`
	Data *approvalInitiatedAPIData `json:"data"`
}

type approvalInitiatedAPIData struct {
	Instances []*approvalInitiatedAPIInfo `json:"instances,omitempty"`
	PageToken string                      `json:"page_token,omitempty"`
	HasMore   bool                        `json:"has_more,omitempty"`
	Count     *int                        `json:"count,omitempty"`
}

type approvalInitiatedAPIInfo struct {
	InstanceCode        approvalTaskString `json:"instance_code,omitempty"`
	DefinitionCode      approvalTaskString `json:"definition_code,omitempty"`
	DefinitionName      approvalTaskString `json:"definition_name,omitempty"`
	DefinitionGroupID   approvalTaskString `json:"definition_group_id,omitempty"`
	DefinitionGroupName approvalTaskString `json:"definition_group_name,omitempty"`
	Initiator           approvalTaskString `json:"initiator,omitempty"`
	InitiatorName       approvalTaskString `json:"initiator_name,omitempty"`
	InstanceStatus      approvalTaskString `json:"instance_status,omitempty"`
	InstanceExternalID  approvalTaskString `json:"instance_external_id,omitempty"`
	Link                approvalTaskString `json:"link,omitempty"`
	Summaries           []ApprovalSummary  `json:"summaries,omitempty"`
}

type approvalDefinitionAPIResp struct {
	Code int                        `json:"code"`
	Msg  string                     `json:"msg"`
	Data *approvalDefinitionAPIData `json:"data"`
}

type approvalDefinitionAPIData struct {
	ApprovalName string                       `json:"approval_name"`
	Form         string                       `json:"form"`
	NodeList     []*approvalDefinitionNodeAPI `json:"node_list"`
}

type approvalDefinitionNodeAPI struct {
	Name                string `json:"name"`
	NodeID              string `json:"node_id"`
	CustomNodeID        string `json:"custom_node_id"`
	NodeType            string `json:"node_type"`
	NeedApprover        bool   `json:"need_approver"`
	ApproverChosenMulti bool   `json:"approver_chosen_multi"`
	RequireSignature    bool   `json:"require_signature"`
	ApproverChosenRange any    `json:"approver_chosen_range"`
}

func setApprovalQuery(q larkcore.QueryParams, key, value string) {
	if strings.TrimSpace(value) != "" {
		q.Set(key, value)
	}
}

func doApprovalUserGet(apiPath string, pathParams, query map[string]string, userAccessToken, action string) ([]byte, error) {
	if strings.TrimSpace(userAccessToken) == "" {
		return nil, fmt.Errorf("%s需要 User Access Token", action)
	}

	c, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   apiPath,
		PathParams:                larkcore.PathParams{},
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}
	for k, v := range pathParams {
		req.PathParams.Set(k, v)
	}
	for k, v := range query {
		setApprovalQuery(req.QueryParams, k, v)
	}

	resp, err := c.Do(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	// 先按业务信封解析再看 HTTP 状态：审批业务错误常随 HTTP 400 下发；
	// HTTP 200 仍可能带业务错误码，Raw / --output raw-json 必须先检查再吐原始 body。
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}
	if _, err := parseGenericApprovalData(resp.RawBody, action); err != nil {
		return nil, err
	}
	return resp.RawBody, nil
}

func getApprovalDefinitionRawBody(approvalCode string, opts GetApprovalOptions, userAccessToken string) ([]byte, error) {
	if strings.TrimSpace(approvalCode) == "" {
		return nil, fmt.Errorf("approval_code 不能为空")
	}
	query := map[string]string{}
	if opts.Locale != "" {
		query["locale"] = opts.Locale
	}
	return doApprovalUserGet(approvalDefinitionDetailPath, map[string]string{
		"approval_code": approvalCode,
	}, query, userAccessToken, "获取审批定义")
}

// GetApprovalDefinitionRaw retrieves the raw approval definition response body from the API.
func GetApprovalDefinitionRaw(approvalCode string, opts GetApprovalOptions, userAccessToken string) ([]byte, error) {
	return getApprovalDefinitionRawBody(approvalCode, opts, userAccessToken)
}

// GetApprovalDefinition retrieves approval definition details by approval code.
func GetApprovalDefinition(approvalCode string, opts GetApprovalOptions, userAccessToken string) (*ApprovalDefinition, error) {
	body, err := getApprovalDefinitionRawBody(approvalCode, opts, userAccessToken)
	if err != nil {
		return nil, err
	}
	return parseApprovalDefinitionResponse(body, approvalCode)
}

func getApprovalInstanceRawBody(opts GetApprovalInstanceOptions, userAccessToken string) ([]byte, error) {
	instanceCode := strings.TrimSpace(opts.InstanceCode)
	if instanceCode == "" {
		return nil, fmt.Errorf("instance_code 不能为空")
	}

	query := map[string]string{
		"instance_code": instanceCode,
		"locale":        opts.Locale,
	}
	if opts.UserIDType != "" {
		userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
		if err != nil {
			return nil, err
		}
		query["user_id_type"] = userIDType
	}
	return doApprovalUserGet(approvalInstanceDetailPath, nil, query, userAccessToken, "获取审批实例详情")
}

// GetApprovalInstanceRaw retrieves the raw approval instance response body from the API.
func GetApprovalInstanceRaw(opts GetApprovalInstanceOptions, userAccessToken string) ([]byte, error) {
	return getApprovalInstanceRawBody(opts, userAccessToken)
}

// GetApprovalInstance retrieves one approval instance detail. The returned map is the API data object.
func GetApprovalInstance(opts GetApprovalInstanceOptions, userAccessToken string) (map[string]any, error) {
	body, err := getApprovalInstanceRawBody(opts, userAccessToken)
	if err != nil {
		return nil, err
	}
	data, err := parseGenericApprovalData(body, "获取审批实例详情")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析审批实例详情响应失败: %w", err)
	}
	return out, nil
}

func parseApprovalDefinitionResponse(body []byte, approvalCode string) (*ApprovalDefinition, error) {
	var apiResp approvalDefinitionAPIResp
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析审批定义响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("获取审批定义失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data == nil {
		return nil, fmt.Errorf("获取审批定义返回数据为空")
	}

	result := &ApprovalDefinition{
		ApprovalCode: approvalCode,
		ApprovalName: apiResp.Data.ApprovalName,
		Form:         parseEmbeddedJSON(apiResp.Data.Form),
	}
	if len(apiResp.Data.NodeList) > 0 {
		result.NodeList = make([]*ApprovalNode, 0, len(apiResp.Data.NodeList))
		for _, node := range apiResp.Data.NodeList {
			if node == nil {
				continue
			}
			result.NodeList = append(result.NodeList, &ApprovalNode{
				Name:                node.Name,
				NodeID:              node.NodeID,
				CustomNodeID:        node.CustomNodeID,
				NodeType:            node.NodeType,
				NeedApprover:        node.NeedApprover,
				ApproverChosenMulti: node.ApproverChosenMulti,
				RequireSignature:    node.RequireSignature,
				ApproverChosenRange: node.ApproverChosenRange,
			})
		}
	}
	return result, nil
}

func queryApprovalTasksRawBody(opts ApprovalTaskQueryOptions, userAccessToken string) ([]byte, error) {
	topic := strings.TrimSpace(opts.Topic)
	if topic == "" {
		return nil, fmt.Errorf("topic 不能为空")
	}

	query := map[string]string{
		"topic":           topic,
		"page_token":      opts.PageToken,
		"locale":          opts.Locale,
		"definition_code": opts.DefinitionCode,
		"start_timestamp": opts.StartTimestamp,
		"end_timestamp":   opts.EndTimestamp,
	}
	if opts.PageSize > 0 {
		query["page_size"] = strconv.Itoa(opts.PageSize)
	}
	if opts.UserIDType != "" {
		userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
		if err != nil {
			return nil, err
		}
		query["user_id_type"] = userIDType
	}
	return doApprovalUserGet(approvalTaskListPath, nil, query, userAccessToken, "查询审批任务")
}

// QueryApprovalTasksRaw retrieves the raw approval task response body from the API.
func QueryApprovalTasksRaw(opts ApprovalTaskQueryOptions, userAccessToken string) ([]byte, error) {
	return queryApprovalTasksRawBody(opts, userAccessToken)
}

// QueryApprovalTasks retrieves approval tasks for the current user.
func QueryApprovalTasks(opts ApprovalTaskQueryOptions, userAccessToken string) (*ApprovalTaskQueryResult, error) {
	body, err := queryApprovalTasksRawBody(opts, userAccessToken)
	if err != nil {
		return nil, err
	}
	return parseApprovalTaskQueryResponse(body)
}

func listInitiatedApprovalInstancesRawBody(opts ListInitiatedApprovalInstancesOptions, userAccessToken string) ([]byte, error) {
	query := map[string]string{
		"page_token":      opts.PageToken,
		"locale":          opts.Locale,
		"definition_code": opts.DefinitionCode,
		"start_timestamp": opts.StartTimestamp,
		"end_timestamp":   opts.EndTimestamp,
	}
	if opts.PageSize > 0 {
		query["page_size"] = strconv.Itoa(opts.PageSize)
	}
	if opts.UserIDType != "" {
		userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
		if err != nil {
			return nil, err
		}
		query["user_id_type"] = userIDType
	}
	return doApprovalUserGet(approvalInstanceInitiatedPath, nil, query, userAccessToken, "查询已发起审批实例")
}

// ListInitiatedApprovalInstancesRaw retrieves the raw initiated-instance response body.
func ListInitiatedApprovalInstancesRaw(opts ListInitiatedApprovalInstancesOptions, userAccessToken string) ([]byte, error) {
	return listInitiatedApprovalInstancesRawBody(opts, userAccessToken)
}

// ListInitiatedApprovalInstances lists approval instances initiated by the current user.
func ListInitiatedApprovalInstances(opts ListInitiatedApprovalInstancesOptions, userAccessToken string) (*ApprovalInitiatedResult, error) {
	body, err := listInitiatedApprovalInstancesRawBody(opts, userAccessToken)
	if err != nil {
		return nil, err
	}
	return parseApprovalInitiatedResponse(body)
}

func parseEmbeddedJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return raw
}

func parseApprovalTaskQueryResponse(body []byte) (*ApprovalTaskQueryResult, error) {
	var apiResp approvalTaskQueryAPIResp
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析审批任务响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询审批任务失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	result := &ApprovalTaskQueryResult{
		Tasks: make([]*ApprovalTaskInfo, 0),
	}
	if apiResp.Data == nil {
		return result, nil
	}
	result.PageToken = apiResp.Data.PageToken
	result.HasMore = apiResp.Data.HasMore
	result.Count = apiResp.Data.Count
	if len(apiResp.Data.Tasks) > 0 {
		result.Tasks = make([]*ApprovalTaskInfo, 0, len(apiResp.Data.Tasks))
		for _, task := range apiResp.Data.Tasks {
			if task == nil {
				continue
			}
			result.Tasks = append(result.Tasks, approvalTaskAPIToInfo(task))
		}
	}
	return result, nil
}

func approvalTaskAPIToInfo(task *approvalTaskAPIInfo) *ApprovalTaskInfo {
	return &ApprovalTaskInfo{
		Topic:               task.Topic.String(),
		UserID:              task.UserID.String(),
		Title:               task.Title.String(),
		Status:              task.Status.String(),
		DefinitionCode:      task.DefinitionCode.String(),
		DefinitionName:      task.DefinitionName.String(),
		DefinitionGroupID:   task.DefinitionGroupID.String(),
		DefinitionGroupName: task.DefinitionGroupName.String(),
		TaskID:              task.TaskID.String(),
		InstanceCode:        task.InstanceCode.String(),
		InstanceStatus:      task.InstanceStatus.String(),
		InstanceExternalID:  task.InstanceExternalID.String(),
		TaskExternalID:      task.TaskExternalID.String(),
		Initiator:           task.Initiator.String(),
		InitiatorName:       task.InitiatorName.String(),
		Summaries:           task.Summaries,
		SupportAPIOperate:   task.SupportAPIOperate,
		Link:                task.Link.String(),
	}
}

func parseApprovalInitiatedResponse(body []byte) (*ApprovalInitiatedResult, error) {
	var apiResp approvalInitiatedAPIResp
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析已发起审批实例响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询已发起审批实例失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	result := &ApprovalInitiatedResult{
		Instances: make([]*ApprovalInitiatedInstance, 0),
	}
	if apiResp.Data == nil {
		return result, nil
	}
	result.PageToken = apiResp.Data.PageToken
	result.HasMore = apiResp.Data.HasMore
	result.Count = apiResp.Data.Count
	if len(apiResp.Data.Instances) > 0 {
		result.Instances = make([]*ApprovalInitiatedInstance, 0, len(apiResp.Data.Instances))
		for _, item := range apiResp.Data.Instances {
			if item == nil {
				continue
			}
			result.Instances = append(result.Instances, &ApprovalInitiatedInstance{
				InstanceCode:        item.InstanceCode.String(),
				DefinitionCode:      item.DefinitionCode.String(),
				DefinitionName:      item.DefinitionName.String(),
				DefinitionGroupID:   item.DefinitionGroupID.String(),
				DefinitionGroupName: item.DefinitionGroupName.String(),
				Initiator:           item.Initiator.String(),
				InitiatorName:       item.InitiatorName.String(),
				InstanceStatus:      item.InstanceStatus.String(),
				InstanceExternalID:  item.InstanceExternalID.String(),
				Link:                item.Link.String(),
				Summaries:           item.Summaries,
			})
		}
	}
	return result, nil
}

// CreateApprovalInstanceOptions represents options for creating an approval instance.
// 创建审批实例参数（POST /open-apis/approval/v4/instances/initiate）
type CreateApprovalInstanceOptions struct {
	ApprovalCode     string          // 必填：审批定义 code
	Form             string          // 可选：表单数据 JSON 字符串
	NodeApproverList json.RawMessage // 可选：节点指定审批人，JSON 原文
	NodeCCList       json.RawMessage // 可选：节点指定抄送人，JSON 原文
	UUID             string          // 可选：幂等 uuid
}

// CreateApprovalInstanceResult 创建实例返回结果。
type CreateApprovalInstanceResult struct {
	InstanceCode string `json:"instance_code"`
	InstanceLink string `json:"instance_link,omitempty"`
}

// CancelApprovalInstanceOptions represents options for cancelling an approval instance.
type CancelApprovalInstanceOptions struct {
	InstanceCode string // 必填：审批实例 code
}

// CCApprovalInstanceOptions represents options for cc'ing an approval instance.
type CCApprovalInstanceOptions struct {
	InstanceCode string   // 必填：审批实例 code
	CCUserIDs    []string // 必填：被抄送用户 ID 列表
	Comment      string   // 可选：抄送备注
	UserIDType   string   // 可选：open_id / user_id / union_id
}

// ApprovalTaskActionOptions represents shared options for task approve/reject.
type ApprovalTaskActionOptions struct {
	InstanceCode string // 必填：审批实例 code
	TaskID       string // 必填：审批任务 ID
	Comment      string // 可选：审批意见
	Form         string // 可选：表单数据（通过任务时使用），JSON 字符串
}

type genericApprovalAPIResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func parseGenericApprovalData(body []byte, action string) (json.RawMessage, error) {
	var apiResp genericApprovalAPIResp
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// doApprovalPost 统一发起审批 POST 调用，支持透传 user_id_type 查询参数 + user/tenant token。
func doApprovalPost(apiPath string, body map[string]any, userIDType, userAccessToken, action string) (json.RawMessage, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}

	if userIDType != "" {
		sep := "?"
		if strings.Contains(apiPath, "?") {
			sep = "&"
		}
		apiPath = apiPath + sep + "user_id_type=" + userIDType
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := c.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}
	return parseGenericApprovalData(resp.RawBody, action)
}

func doApprovalUserPost(apiPath string, body map[string]any, userIDType, userAccessToken, action string) (json.RawMessage, error) {
	if strings.TrimSpace(userAccessToken) == "" {
		return nil, fmt.Errorf("%s需要 User Access Token", action)
	}
	return doApprovalPost(apiPath, body, userIDType, userAccessToken, action)
}

func normalizeApprovalWriteUserIDType(userIDType string) (string, error) {
	t := strings.TrimSpace(userIDType)
	if t == "" {
		return "open_id", nil
	}
	switch t {
	case "open_id", "user_id", "union_id":
		return t, nil
	default:
		return "", fmt.Errorf("user_id_type 不支持 %q，仅支持 open_id / user_id / union_id", userIDType)
	}
}

func normalizeApprovalNodeKVList(raw json.RawMessage, field string) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", field, err)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		key, _ := item["key"].(string)
		if strings.TrimSpace(key) == "" {
			if v, ok := item["custom_node_id"].(string); ok && strings.TrimSpace(v) != "" {
				key = v
			} else if v, ok := item["node_id"].(string); ok {
				key = v
			}
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s 每项必须包含 key（或 node_id / custom_node_id）", field)
		}
		out = append(out, map[string]any{
			"key":   key,
			"value": item["value"],
		})
	}
	return out, nil
}

func buildCreateApprovalInstanceBody(opts CreateApprovalInstanceOptions) (map[string]any, error) {
	if strings.TrimSpace(opts.ApprovalCode) == "" {
		return nil, fmt.Errorf("approval_code 不能为空")
	}

	body := map[string]any{
		"approval_code": opts.ApprovalCode,
	}
	if strings.TrimSpace(opts.Form) != "" {
		body["form"] = opts.Form
	}
	if strings.TrimSpace(opts.UUID) != "" {
		body["uuid"] = strings.TrimSpace(opts.UUID)
	}
	if len(opts.NodeApproverList) > 0 {
		v, err := normalizeApprovalNodeKVList(opts.NodeApproverList, "node_approver_list")
		if err != nil {
			return nil, err
		}
		if v != nil {
			body["node_approver_list"] = v
		}
	}
	if len(opts.NodeCCList) > 0 {
		v, err := normalizeApprovalNodeKVList(opts.NodeCCList, "node_cc_list")
		if err != nil {
			return nil, err
		}
		if v != nil {
			body["node_cc_list"] = v
		}
	}
	return body, nil
}

// CreateApprovalInstance 以当前 User Token 身份发起一条审批实例。
func CreateApprovalInstance(opts CreateApprovalInstanceOptions, userAccessToken string) (*CreateApprovalInstanceResult, error) {
	body, err := buildCreateApprovalInstanceBody(opts)
	if err != nil {
		return nil, err
	}
	data, err := doApprovalUserPost(approvalInstanceInitiatePath, body, "", userAccessToken, "创建审批实例")
	if err != nil {
		return nil, err
	}

	result := &CreateApprovalInstanceResult{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return nil, fmt.Errorf("解析创建审批实例响应失败: %w", err)
		}
	}
	return result, nil
}

// CancelApprovalInstance 撤回已发起的审批实例。
func CancelApprovalInstance(opts CancelApprovalInstanceOptions, userAccessToken string) error {
	body, err := buildCancelApprovalInstanceBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalInstanceRecallPath, body, "", userAccessToken, "取消审批实例")
	return err
}

// CCApprovalInstance 抄送审批实例给指定用户。
func CCApprovalInstance(opts CCApprovalInstanceOptions, userAccessToken string) error {
	body, userIDType, err := buildCCApprovalInstanceBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalInstanceAddCCPath, body, userIDType, userAccessToken, "抄送审批实例")
	return err
}

// ApproveApprovalTask 通过指定审批任务。
func ApproveApprovalTask(opts ApprovalTaskActionOptions, userAccessToken string) error {
	return runApprovalTaskAction(approvalTaskPassPath, opts, userAccessToken, "通过审批任务", true)
}

// RejectApprovalTask 拒绝指定审批任务。
func RejectApprovalTask(opts ApprovalTaskActionOptions, userAccessToken string) error {
	return runApprovalTaskAction(approvalTaskRefusePath, opts, userAccessToken, "拒绝审批任务", false)
}

// TransferApprovalTask 转交指定审批任务给另一位用户。
func TransferApprovalTask(opts TransferApprovalTaskOptions, userAccessToken string) error {
	body, userIDType, err := buildTransferApprovalTaskBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalTaskForwardPath, body, userIDType, userAccessToken, "转交审批任务")
	return err
}

func runApprovalTaskAction(apiPath string, opts ApprovalTaskActionOptions, userAccessToken, action string, includeForm bool) error {
	body, err := buildApprovalTaskActionBody(opts, includeForm)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(apiPath, body, "", userAccessToken, action)
	return err
}

func buildCancelApprovalInstanceBody(opts CancelApprovalInstanceOptions) (map[string]any, error) {
	instanceCode := strings.TrimSpace(opts.InstanceCode)
	if instanceCode == "" {
		return nil, fmt.Errorf("instance_code 不能为空")
	}
	return map[string]any{
		"instance_code": instanceCode,
	}, nil
}

func buildCCApprovalInstanceBody(opts CCApprovalInstanceOptions) (map[string]any, string, error) {
	instanceCode := strings.TrimSpace(opts.InstanceCode)
	if instanceCode == "" {
		return nil, "", fmt.Errorf("instance_code 不能为空")
	}
	if len(opts.CCUserIDs) == 0 {
		return nil, "", fmt.Errorf("cc_user_ids 不能为空")
	}
	userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
	if err != nil {
		return nil, "", err
	}

	body := map[string]any{
		"instance_code": instanceCode,
		"cc_user_ids":   opts.CCUserIDs,
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	return body, userIDType, nil
}

func buildApprovalTaskActionBody(opts ApprovalTaskActionOptions, includeForm bool) (map[string]any, error) {
	instanceCode := strings.TrimSpace(opts.InstanceCode)
	if instanceCode == "" {
		return nil, fmt.Errorf("instance_code 不能为空")
	}
	taskID := strings.TrimSpace(opts.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id 不能为空")
	}

	body := map[string]any{
		"instance_code": instanceCode,
		"task_id":       taskID,
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	if includeForm && opts.Form != "" {
		body["form"] = opts.Form
	}
	return body, nil
}

func buildTransferApprovalTaskBody(opts TransferApprovalTaskOptions) (map[string]any, string, error) {
	instanceCode := strings.TrimSpace(opts.InstanceCode)
	if instanceCode == "" {
		return nil, "", fmt.Errorf("instance_code 不能为空")
	}
	taskID := strings.TrimSpace(opts.TaskID)
	if taskID == "" {
		return nil, "", fmt.Errorf("task_id 不能为空")
	}
	transferUserID := strings.TrimSpace(opts.TransferUserID)
	if transferUserID == "" {
		return nil, "", fmt.Errorf("transfer_user_id 不能为空")
	}
	userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
	if err != nil {
		return nil, "", err
	}

	body := map[string]any{
		"instance_code":    instanceCode,
		"task_id":          taskID,
		"transfer_user_id": transferUserID,
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	return body, userIDType, nil
}
