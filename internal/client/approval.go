package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkapproval "github.com/larksuite/oapi-sdk-go/v3/service/approval/v4"
)

// GetApprovalOptions represents optional filters for fetching an approval definition.
type GetApprovalOptions struct {
	Locale      string
	WithAdminID bool
	UserIDType  string
	WithOption  bool
	UserID      string
}

// ApprovalDefinition represents a simplified approval definition.
type ApprovalDefinition struct {
	ApprovalCode       string            `json:"approval_code"`
	ApprovalName       string            `json:"approval_name"`
	Status             string            `json:"status"`
	Form               any               `json:"form,omitempty"`
	NodeList           []*ApprovalNode   `json:"node_list,omitempty"`
	Viewers            []*ApprovalViewer `json:"viewers,omitempty"`
	ApprovalAdminIDs   []string          `json:"approval_admin_ids,omitempty"`
	FormWidgetRelation any               `json:"form_widget_relation,omitempty"`
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
}

// ApprovalViewer represents a simplified approval viewer.
type ApprovalViewer struct {
	Type   string `json:"type,omitempty"`
	ID     string `json:"id,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// ApprovalTaskQueryOptions represents options for querying approval tasks.
type ApprovalTaskQueryOptions struct {
	PageSize   int
	PageToken  string
	UserID     string
	Topic      string
	UserIDType string
}

// ApprovalTaskQueryResult represents a simplified approval task query result.
type ApprovalTaskQueryResult struct {
	Tasks     []*ApprovalTaskInfo `json:"tasks"`
	PageToken string              `json:"page_token,omitempty"`
	HasMore   bool                `json:"has_more"`
	Count     *ApprovalTaskCount  `json:"count,omitempty"`
}

// ApprovalTaskCount represents summary count information returned on the first page.
type ApprovalTaskCount struct {
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

// ApprovalTaskInfo represents a simplified approval task.
type ApprovalTaskInfo struct {
	Topic               string   `json:"topic,omitempty"`
	UserID              string   `json:"user_id,omitempty"`
	Title               string   `json:"title,omitempty"`
	HelpdeskURL         string   `json:"helpdesk_url,omitempty"`
	MobileURL           string   `json:"mobile_url,omitempty"`
	PCURL               string   `json:"pc_url,omitempty"`
	ProcessExternalID   string   `json:"process_external_id,omitempty"`
	TaskExternalID      string   `json:"task_external_id,omitempty"`
	Status              string   `json:"status,omitempty"`
	ProcessStatus       string   `json:"process_status,omitempty"`
	DefinitionCode      string   `json:"definition_code,omitempty"`
	DefinitionName      string   `json:"definition_name,omitempty"`
	DefinitionID        string   `json:"definition_id,omitempty"`
	DefinitionGroupID   string   `json:"definition_group_id,omitempty"`
	DefinitionGroupName string   `json:"definition_group_name,omitempty"`
	Initiators          []string `json:"initiators,omitempty"`
	InitiatorNames      []string `json:"initiator_names,omitempty"`
	TaskID              string   `json:"task_id,omitempty"`
	ProcessID           string   `json:"process_id,omitempty"`
	ProcessCode         string   `json:"process_code,omitempty"`
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
	Count     *ApprovalTaskCount     `json:"count,omitempty"`
}

type approvalTaskAPIInfo struct {
	Topic               approvalTaskString `json:"topic,omitempty"`
	UserID              approvalTaskString `json:"user_id,omitempty"`
	Title               approvalTaskString `json:"title,omitempty"`
	Urls                *approvalTaskURLs  `json:"urls,omitempty"`
	ProcessExternalID   approvalTaskString `json:"process_external_id,omitempty"`
	TaskExternalID      approvalTaskString `json:"task_external_id,omitempty"`
	Status              approvalTaskString `json:"status,omitempty"`
	ProcessStatus       approvalTaskString `json:"process_status,omitempty"`
	DefinitionCode      approvalTaskString `json:"definition_code,omitempty"`
	DefinitionName      approvalTaskString `json:"definition_name,omitempty"`
	DefinitionID        approvalTaskString `json:"definition_id,omitempty"`
	DefinitionGroupID   approvalTaskString `json:"definition_group_id,omitempty"`
	DefinitionGroupName approvalTaskString `json:"definition_group_name,omitempty"`
	Initiators          []string           `json:"initiators,omitempty"`
	InitiatorNames      []string           `json:"initiator_names,omitempty"`
	TaskID              approvalTaskString `json:"task_id,omitempty"`
	ProcessID           approvalTaskString `json:"process_id,omitempty"`
	ProcessCode         approvalTaskString `json:"process_code,omitempty"`
}

type approvalTaskURLs struct {
	Helpdesk approvalTaskString `json:"helpdesk,omitempty"`
	Mobile   approvalTaskString `json:"mobile,omitempty"`
	Pc       approvalTaskString `json:"pc,omitempty"`
}

type approvalGetAPIResp struct {
	Code int                               `json:"code"`
	Msg  string                            `json:"msg"`
	Data *larkapproval.GetApprovalRespData `json:"data"`
}

func getApprovalDefinitionRawBody(approvalCode string, opts GetApprovalOptions) ([]byte, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/approval/v4/approvals/:approval_code",
		PathParams:                larkcore.PathParams{},
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	}
	req.PathParams.Set("approval_code", approvalCode)
	if opts.Locale != "" {
		req.QueryParams.Set("locale", opts.Locale)
	}
	if opts.WithAdminID {
		req.QueryParams.Set("with_admin_id", "true")
	}
	if opts.UserIDType != "" {
		req.QueryParams.Set("user_id_type", opts.UserIDType)
	}
	if opts.WithOption {
		req.QueryParams.Set("with_option", "true")
	}
	if opts.UserID != "" {
		req.QueryParams.Set("user_id", opts.UserID)
	}

	resp, err := client.Do(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取审批定义失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取审批定义失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	return resp.RawBody, nil
}

// GetApprovalDefinitionRaw retrieves the raw approval definition response body from the API.
func GetApprovalDefinitionRaw(approvalCode string, opts GetApprovalOptions) ([]byte, error) {
	return getApprovalDefinitionRawBody(approvalCode, opts)
}

// GetApprovalDefinition retrieves approval definition details by approval code.
func GetApprovalDefinition(approvalCode string, opts GetApprovalOptions) (*ApprovalDefinition, error) {
	body, err := getApprovalDefinitionRawBody(approvalCode, opts)
	if err != nil {
		return nil, err
	}

	return parseApprovalDefinitionResponse(body, approvalCode)
}

func parseApprovalDefinitionResponse(body []byte, approvalCode string) (*ApprovalDefinition, error) {
	var apiResp approvalGetAPIResp
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
		ApprovalCode:       approvalCode,
		ApprovalName:       StringVal(apiResp.Data.ApprovalName),
		Status:             StringVal(apiResp.Data.Status),
		Form:               parseEmbeddedJSON(StringVal(apiResp.Data.Form)),
		ApprovalAdminIDs:   apiResp.Data.ApprovalAdminIds,
		FormWidgetRelation: parseEmbeddedJSON(StringVal(apiResp.Data.FormWidgetRelation)),
	}

	if len(apiResp.Data.NodeList) > 0 {
		result.NodeList = make([]*ApprovalNode, 0, len(apiResp.Data.NodeList))
		for _, node := range apiResp.Data.NodeList {
			if node == nil {
				continue
			}
			result.NodeList = append(result.NodeList, &ApprovalNode{
				Name:                StringVal(node.Name),
				NodeID:              StringVal(node.NodeId),
				CustomNodeID:        StringVal(node.CustomNodeId),
				NodeType:            StringVal(node.NodeType),
				NeedApprover:        BoolVal(node.NeedApprover),
				ApproverChosenMulti: BoolVal(node.ApproverChosenMulti),
				RequireSignature:    BoolVal(node.RequireSignature),
			})
		}
	}

	if len(apiResp.Data.Viewers) > 0 {
		result.Viewers = make([]*ApprovalViewer, 0, len(apiResp.Data.Viewers))
		for _, viewer := range apiResp.Data.Viewers {
			if viewer == nil {
				continue
			}
			result.Viewers = append(result.Viewers, &ApprovalViewer{
				Type:   StringVal(viewer.Type),
				ID:     StringVal(viewer.Id),
				UserID: StringVal(viewer.UserId),
			})
		}
	}

	return result, nil
}

func queryApprovalTasksRawBody(opts ApprovalTaskQueryOptions, userAccessToken string) ([]byte, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/approval/v4/tasks/query",
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant, larkcore.AccessTokenTypeUser},
	}
	req.QueryParams.Set("user_id", opts.UserID)
	req.QueryParams.Set("topic", opts.Topic)

	if opts.PageSize > 0 {
		req.QueryParams.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		req.QueryParams.Set("page_token", opts.PageToken)
	}
	if opts.UserIDType != "" {
		req.QueryParams.Set("user_id_type", opts.UserIDType)
	}

	resp, err := client.Do(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("查询审批任务失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询审批任务失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	return resp.RawBody, nil
}

// QueryApprovalTasksRaw retrieves the raw approval task response body from the API.
func QueryApprovalTasksRaw(opts ApprovalTaskQueryOptions, userAccessToken string) ([]byte, error) {
	return queryApprovalTasksRawBody(opts, userAccessToken)
}

// QueryApprovalTasks retrieves approval tasks for a user.
func QueryApprovalTasks(opts ApprovalTaskQueryOptions, userAccessToken string) (*ApprovalTaskQueryResult, error) {
	body, err := queryApprovalTasksRawBody(opts, userAccessToken)
	if err != nil {
		return nil, err
	}

	return parseApprovalTaskQueryResponse(body)
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
	info := &ApprovalTaskInfo{
		Topic:               task.Topic.String(),
		UserID:              task.UserID.String(),
		Title:               task.Title.String(),
		ProcessExternalID:   task.ProcessExternalID.String(),
		TaskExternalID:      task.TaskExternalID.String(),
		Status:              task.Status.String(),
		ProcessStatus:       task.ProcessStatus.String(),
		DefinitionCode:      task.DefinitionCode.String(),
		DefinitionName:      task.DefinitionName.String(),
		DefinitionID:        task.DefinitionID.String(),
		DefinitionGroupID:   task.DefinitionGroupID.String(),
		DefinitionGroupName: task.DefinitionGroupName.String(),
		Initiators:          task.Initiators,
		InitiatorNames:      task.InitiatorNames,
		TaskID:              task.TaskID.String(),
		ProcessID:           task.ProcessID.String(),
		ProcessCode:         task.ProcessCode.String(),
	}

	if task.Urls != nil {
		info.HelpdeskURL = task.Urls.Helpdesk.String()
		info.MobileURL = task.Urls.Mobile.String()
		info.PCURL = task.Urls.Pc.String()
	}

	return info
}

// CreateApprovalInstanceOptions represents options for creating an approval instance.
// 创建审批实例参数（POST /open-apis/approval/v4/instances）
type CreateApprovalInstanceOptions struct {
	ApprovalCode string // 必填：审批定义 code
	UserID       string // 必填：发起人 ID（open_id/user_id/union_id）
	Form         string // 必填：表单数据 JSON 字符串
	UserIDType   string // 可选：open_id / user_id / union_id，默认 open_id
	DepartmentID string // 可选：发起人部门
	OpenChatID   string // 可选：发送结果到的群聊
	NodeApproverUserIDList json.RawMessage // 可选：节点指定审批人，JSON 原文
	NodeCCUserIDList       json.RawMessage // 可选：节点指定抄送人，JSON 原文
}

// CreateApprovalInstanceResult 创建实例返回结果，仅暴露常用字段。
type CreateApprovalInstanceResult struct {
	InstanceCode string `json:"instance_code"`
}

// CancelApprovalInstanceOptions represents options for cancelling an approval instance.
// 取消审批实例参数（POST /open-apis/approval/v4/instances/cancel）
type CancelApprovalInstanceOptions struct {
	ApprovalCode string // 必填：审批定义 code
	InstanceCode string // 必填：审批实例 code
	UserID       string // 必填：执行操作的用户 ID
	UserIDType   string // 可选：open_id / user_id / union_id
}

// CCApprovalInstanceOptions represents options for cc'ing an approval instance.
// 抄送审批实例参数（POST /open-apis/approval/v4/instances/cc）
type CCApprovalInstanceOptions struct {
	ApprovalCode string   // 必填：审批定义 code
	InstanceCode string   // 必填：审批实例 code
	UserID       string   // 必填：执行抄送的用户 ID
	CCUserIDs    []string // 必填：被抄送用户 ID 列表
	Comment      string   // 可选：抄送备注
	UserIDType   string   // 可选：open_id / user_id / union_id
}

// ApprovalTaskActionOptions represents shared options for task approve/reject.
// 通过/拒绝审批任务参数（POST /open-apis/approval/v4/tasks/{approve,reject}）
type ApprovalTaskActionOptions struct {
	ApprovalCode string // 必填：审批定义 code
	InstanceCode string // 必填：审批实例 code
	TaskID       string // 必填：审批任务 ID
	UserID       string // 必填：操作人 ID
	Comment      string // 可选：审批意见
	Form         string // 可选：表单数据（更新表单时使用），JSON 字符串
	UserIDType   string // 可选：open_id / user_id / union_id
}

// genericApprovalAPIResp 解析审批写接口的通用响应（多数返回 data 为空或仅含 instance_code）。
type genericApprovalAPIResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, resp.StatusCode, string(resp.RawBody))
	}

	var apiResp genericApprovalAPIResp
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s失败: code=%d, msg=%s", action, apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// CreateApprovalInstance 创建一条审批实例，返回 instance_code。
func CreateApprovalInstance(opts CreateApprovalInstanceOptions, userAccessToken string) (*CreateApprovalInstanceResult, error) {
	if strings.TrimSpace(opts.ApprovalCode) == "" {
		return nil, fmt.Errorf("approval_code 不能为空")
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return nil, fmt.Errorf("user_id 不能为空")
	}
	if strings.TrimSpace(opts.Form) == "" {
		return nil, fmt.Errorf("form 不能为空")
	}

	body := map[string]any{
		"approval_code": opts.ApprovalCode,
		"user_id":       opts.UserID,
		"form":          opts.Form,
	}
	if opts.DepartmentID != "" {
		body["department_id"] = opts.DepartmentID
	}
	if opts.OpenChatID != "" {
		body["open_chat_id"] = opts.OpenChatID
	}
	if len(opts.NodeApproverUserIDList) > 0 {
		var v any
		if err := json.Unmarshal(opts.NodeApproverUserIDList, &v); err != nil {
			return nil, fmt.Errorf("解析 node_approver_user_id_list 失败: %w", err)
		}
		body["node_approver_user_id_list"] = v
	}
	if len(opts.NodeCCUserIDList) > 0 {
		var v any
		if err := json.Unmarshal(opts.NodeCCUserIDList, &v); err != nil {
			return nil, fmt.Errorf("解析 node_cc_user_id_list 失败: %w", err)
		}
		body["node_cc_user_id_list"] = v
	}

	data, err := doApprovalPost("/open-apis/approval/v4/instances", body, opts.UserIDType, userAccessToken, "创建审批实例")
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

// CancelApprovalInstance 取消（撤回）已发起的审批实例。
func CancelApprovalInstance(opts CancelApprovalInstanceOptions, userAccessToken string) error {
	if strings.TrimSpace(opts.ApprovalCode) == "" {
		return fmt.Errorf("approval_code 不能为空")
	}
	if strings.TrimSpace(opts.InstanceCode) == "" {
		return fmt.Errorf("instance_code 不能为空")
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return fmt.Errorf("user_id 不能为空")
	}

	body := map[string]any{
		"approval_code": opts.ApprovalCode,
		"instance_code": opts.InstanceCode,
		"user_id":       opts.UserID,
	}
	_, err := doApprovalPost("/open-apis/approval/v4/instances/cancel", body, opts.UserIDType, userAccessToken, "取消审批实例")
	return err
}

// CCApprovalInstance 抄送审批实例给指定用户。
func CCApprovalInstance(opts CCApprovalInstanceOptions, userAccessToken string) error {
	if strings.TrimSpace(opts.ApprovalCode) == "" {
		return fmt.Errorf("approval_code 不能为空")
	}
	if strings.TrimSpace(opts.InstanceCode) == "" {
		return fmt.Errorf("instance_code 不能为空")
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return fmt.Errorf("user_id 不能为空")
	}
	if len(opts.CCUserIDs) == 0 {
		return fmt.Errorf("cc_user_ids 不能为空")
	}

	body := map[string]any{
		"approval_code":  opts.ApprovalCode,
		"instance_code":  opts.InstanceCode,
		"user_id":        opts.UserID,
		"cc_user_ids":    opts.CCUserIDs,
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	_, err := doApprovalPost("/open-apis/approval/v4/instances/cc", body, opts.UserIDType, userAccessToken, "抄送审批实例")
	return err
}

// ApproveApprovalTask 通过指定审批任务。
func ApproveApprovalTask(opts ApprovalTaskActionOptions, userAccessToken string) error {
	return runApprovalTaskAction("/open-apis/approval/v4/tasks/approve", opts, userAccessToken, "通过审批任务")
}

// RejectApprovalTask 拒绝指定审批任务。
func RejectApprovalTask(opts ApprovalTaskActionOptions, userAccessToken string) error {
	return runApprovalTaskAction("/open-apis/approval/v4/tasks/reject", opts, userAccessToken, "拒绝审批任务")
}

func runApprovalTaskAction(apiPath string, opts ApprovalTaskActionOptions, userAccessToken, action string) error {
	if strings.TrimSpace(opts.ApprovalCode) == "" {
		return fmt.Errorf("approval_code 不能为空")
	}
	if strings.TrimSpace(opts.InstanceCode) == "" {
		return fmt.Errorf("instance_code 不能为空")
	}
	if strings.TrimSpace(opts.TaskID) == "" {
		return fmt.Errorf("task_id 不能为空")
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return fmt.Errorf("user_id 不能为空")
	}

	body := map[string]any{
		"approval_code": opts.ApprovalCode,
		"instance_code": opts.InstanceCode,
		"task_id":       opts.TaskID,
		"user_id":       opts.UserID,
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	if opts.Form != "" {
		body["form"] = opts.Form
	}
	_, err := doApprovalPost(apiPath, body, opts.UserIDType, userAccessToken, action)
	return err
}
