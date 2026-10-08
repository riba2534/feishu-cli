package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	larkwiki "github.com/larksuite/oapi-sdk-go/v3/service/wiki/v2"
)

// WikiNode 知识库节点信息
type WikiNode struct {
	SpaceID         string `json:"space_id"`
	NodeToken       string `json:"node_token"`
	ObjToken        string `json:"obj_token"`
	ObjType         string `json:"obj_type"`
	ParentNodeToken string `json:"parent_node_token,omitempty"`
	NodeType        string `json:"node_type"`
	OriginNodeToken string `json:"origin_node_token,omitempty"`
	OriginSpaceID   string `json:"origin_space_id,omitempty"`
	Title           string `json:"title"`
	HasChild        bool   `json:"has_child"`
	Creator         string `json:"creator,omitempty"`
	Owner           string `json:"owner,omitempty"`
	NodeCreateTime  string `json:"node_create_time,omitempty"`
	ObjCreateTime   string `json:"obj_create_time,omitempty"`
	ObjEditTime     string `json:"obj_edit_time,omitempty"`
}

// WikiSpace 知识空间信息
type WikiSpace struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SpaceType   string `json:"space_type,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

// GetWikiNode 获取知识库节点信息。
//
// Deprecated: 请直接使用 ResolveWikiNode。保留此包装是为了让尚未迁移的调用点
// 同样走 node_by_token（旧 get_node 对 obj_token 一律报 131005）。
func GetWikiNode(token string, userAccessToken string) (*WikiNode, error) {
	return ResolveWikiNode(token, userAccessToken)
}

// GetWikiNodeWithOptions 获取知识库节点信息。
//
// Deprecated: node_by_token 由服务端自动识别 token 类型，objType 已无需传入并被忽略；
// 请直接使用 ResolveWikiNode。
func GetWikiNodeWithOptions(token, objType, userAccessToken string) (*WikiNode, error) {
	_ = objType
	return ResolveWikiNode(token, userAccessToken)
}

// ListWikiSpaces 获取知识空间列表
func ListWikiSpaces(pageSize int, pageToken string, userAccessToken string) ([]*WikiSpace, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkwiki.NewListSpaceReqBuilder()
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Wiki.Space.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取知识空间列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取知识空间列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var spaces []*WikiSpace
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			spaces = append(spaces, &WikiSpace{
				SpaceID:     StringVal(item.SpaceId),
				Name:        StringVal(item.Name),
				Description: StringVal(item.Description),
				SpaceType:   StringVal(item.SpaceType),
				Visibility:  StringVal(item.Visibility),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return spaces, nextPageToken, hasMore, nil
}

// ListWikiNodes 获取知识空间下的节点列表
func ListWikiNodes(spaceID string, parentNodeToken string, pageSize int, pageToken string, userAccessToken string) ([]*WikiNode, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkwiki.NewListSpaceNodeReqBuilder().
		SpaceId(spaceID)

	if parentNodeToken != "" {
		reqBuilder.ParentNodeToken(parentNodeToken)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Wiki.SpaceNode.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取节点列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取节点列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var nodes []*WikiNode
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			nodes = append(nodes, &WikiNode{
				SpaceID:         StringVal(item.SpaceId),
				NodeToken:       StringVal(item.NodeToken),
				ObjToken:        StringVal(item.ObjToken),
				ObjType:         StringVal(item.ObjType),
				ParentNodeToken: StringVal(item.ParentNodeToken),
				NodeType:        StringVal(item.NodeType),
				Title:           StringVal(item.Title),
				HasChild:        BoolVal(item.HasChild),
				Creator:         StringVal(item.Creator),
				Owner:           StringVal(item.Owner),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return nodes, nextPageToken, hasMore, nil
}

// CreateWikiNodeResult 创建节点的结果
type CreateWikiNodeResult struct {
	SpaceID         string `json:"space_id"`
	NodeToken       string `json:"node_token"`
	ObjToken        string `json:"obj_token"`
	ObjType         string `json:"obj_type"`
	NodeType        string `json:"node_type,omitempty"`
	OriginNodeToken string `json:"origin_node_token,omitempty"`
}

// CreateWikiNode 在知识空间中创建节点
func CreateWikiNode(spaceID, title, parentNode, objType, nodeType, originNodeToken string, userAccessToken string) (*CreateWikiNodeResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	nodeBuilder := larkwiki.NewNodeBuilder().
		Title(title)

	if objType == "" {
		objType = larkwiki.ObjTypeObjTypeDocx
	}
	nodeBuilder.ObjType(objType)

	if nodeType == "" {
		nodeType = larkwiki.NodeTypeNodeTypeEntity
	}
	nodeBuilder.NodeType(nodeType)

	if parentNode != "" {
		nodeBuilder.ParentNodeToken(parentNode)
	}

	if originNodeToken != "" {
		nodeBuilder.OriginNodeToken(originNodeToken)
	}

	req := larkwiki.NewCreateSpaceNodeReqBuilder().
		SpaceId(spaceID).
		Node(nodeBuilder.Build()).
		Build()

	resp, err := client.Wiki.SpaceNode.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("创建知识库节点失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建知识库节点失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Node == nil {
		return nil, fmt.Errorf("创建节点成功但未返回节点信息")
	}

	node := resp.Data.Node
	return &CreateWikiNodeResult{
		SpaceID:         StringVal(node.SpaceId),
		NodeToken:       StringVal(node.NodeToken),
		ObjToken:        StringVal(node.ObjToken),
		ObjType:         StringVal(node.ObjType),
		NodeType:        StringVal(node.NodeType),
		OriginNodeToken: StringVal(node.OriginNodeToken),
	}, nil
}

// UpdateWikiNode 更新知识库节点标题
func UpdateWikiNode(spaceID, nodeToken, title string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	body := larkwiki.NewUpdateTitleSpaceNodeReqBodyBuilder().
		Title(title).
		Build()

	req := larkwiki.NewUpdateTitleSpaceNodeReqBuilder().
		SpaceId(spaceID).
		NodeToken(nodeToken).
		Body(body).
		Build()

	resp, err := client.Wiki.SpaceNode.UpdateTitle(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("更新知识库节点标题失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新知识库节点标题失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// WikiSpaceDetail 知识空间详细信息
type WikiSpaceDetail struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SpaceType   string `json:"space_type,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	OpenSharing string `json:"open_sharing,omitempty"`
}

// WikiSpaceMember 知识空间成员
type WikiSpaceMember struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
	MemberRole string `json:"member_role"`
	Type       string `json:"type,omitempty"`
}

// GetWikiSpace 获取知识空间详情
func GetWikiSpace(spaceID string, userAccessToken string) (*WikiSpaceDetail, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkwiki.NewGetSpaceReqBuilder().
		SpaceId(spaceID).
		Build()

	resp, err := client.Wiki.Space.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取知识空间详情失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取知识空间详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Space == nil {
		return nil, fmt.Errorf("知识空间不存在")
	}

	space := resp.Data.Space
	return &WikiSpaceDetail{
		SpaceID:     StringVal(space.SpaceId),
		Name:        StringVal(space.Name),
		Description: StringVal(space.Description),
		SpaceType:   StringVal(space.SpaceType),
		Visibility:  StringVal(space.Visibility),
		OpenSharing: StringVal(space.OpenSharing),
	}, nil
}

// AddWikiSpaceMember 添加知识空间成员
func AddWikiSpaceMember(spaceID, memberType, memberID, memberRole string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	member := larkwiki.NewMemberBuilder().
		MemberType(memberType).
		MemberId(memberID).
		MemberRole(memberRole).
		Build()

	req := larkwiki.NewCreateSpaceMemberReqBuilder().
		SpaceId(spaceID).
		Member(member).
		NeedNotification(true).
		Build()

	resp, err := client.Wiki.SpaceMember.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("添加知识空间成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加知识空间成员失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ListWikiSpaceMembers 列出知识空间成员
func ListWikiSpaceMembers(spaceID string, pageSize int, pageToken string, userAccessToken string) ([]*WikiSpaceMember, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkwiki.NewListSpaceMemberReqBuilder().
		SpaceId(spaceID)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Wiki.SpaceMember.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取知识空间成员列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取知识空间成员列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var members []*WikiSpaceMember
	if resp.Data != nil && resp.Data.Members != nil {
		for _, item := range resp.Data.Members {
			members = append(members, &WikiSpaceMember{
				MemberType: StringVal(item.MemberType),
				MemberID:   StringVal(item.MemberId),
				MemberRole: StringVal(item.MemberRole),
				Type:       StringVal(item.Type),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return members, nextPageToken, hasMore, nil
}

// RemoveWikiSpaceMember 移除知识空间成员
func RemoveWikiSpaceMember(spaceID, memberType, memberID, memberRole string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	member := larkwiki.NewMemberBuilder().
		MemberType(memberType).
		MemberId(memberID).
		MemberRole(memberRole).
		Build()

	req := larkwiki.NewDeleteSpaceMemberReqBuilder().
		SpaceId(spaceID).
		MemberId(memberID).
		Member(member).
		Build()

	resp, err := client.Wiki.SpaceMember.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("移除知识空间成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("移除知识空间成员失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// MoveWikiNodeResult 移动节点的结果
type MoveWikiNodeResult struct {
	NodeToken string `json:"node_token"`
}

// MoveWikiNode 移动知识库节点
func MoveWikiNode(spaceID, nodeToken, targetSpaceID, targetParent string, userAccessToken string) (*MoveWikiNodeResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkwiki.NewMoveSpaceNodeReqBodyBuilder().
		TargetSpaceId(targetSpaceID)

	if targetParent != "" {
		bodyBuilder.TargetParentToken(targetParent)
	}

	req := larkwiki.NewMoveSpaceNodeReqBuilder().
		SpaceId(spaceID).
		NodeToken(nodeToken).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Wiki.SpaceNode.Move(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("移动知识库节点失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("移动知识库节点失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &MoveWikiNodeResult{}
	if resp.Data != nil && resp.Data.Node != nil {
		result.NodeToken = StringVal(resp.Data.Node.NodeToken)
	}

	return result, nil
}

// MoveDocsToWikiResult 移动云空间文档至知识空间的结果
type MoveDocsToWikiResult struct {
	WikiToken string `json:"wiki_token,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Applied   bool   `json:"applied"`
}

// MoveDocsToWiki 将云空间（我的空间/共享空间）已有文档挂载到知识空间节点树下。
//
// 移动后文档从云空间相关入口消失，权限默认继承父页面。如果文档较大会返回 task_id
// 异步执行；如果调用方无权限但 apply=true，会提交迁入申请（applied=true）。
//
// objType 可选：docx / doc / sheet / mindnote / bitable / file
func MoveDocsToWiki(spaceID, objType, objToken, parentWikiToken string, apply bool, userAccessToken string) (*MoveDocsToWikiResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkwiki.NewMoveDocsToWikiSpaceNodeReqBodyBuilder().
		ObjType(objType).
		ObjToken(objToken).
		Apply(apply)

	if parentWikiToken != "" {
		bodyBuilder.ParentWikiToken(parentWikiToken)
	}

	req := larkwiki.NewMoveDocsToWikiSpaceNodeReqBuilder().
		SpaceId(spaceID).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Wiki.SpaceNode.MoveDocsToWiki(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("移动云空间文档至知识空间失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("移动云空间文档至知识空间失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &MoveDocsToWikiResult{}
	if resp.Data != nil {
		result.WikiToken = StringVal(resp.Data.WikiToken)
		result.TaskID = StringVal(resp.Data.TaskId)
		result.Applied = BoolVal(resp.Data.Applied)
	}
	return result, nil
}

// WikiDeleteSpaceTaskStatus 表示 delete_space 异步任务的状态。
type WikiDeleteSpaceTaskStatus struct {
	TaskID    string
	Status    string // success / failure / processing 等
	StatusMsg string
}

// Ready 表示任务已成功完成。
func (s WikiDeleteSpaceTaskStatus) Ready() bool {
	return strings.EqualFold(strings.TrimSpace(s.Status), "success")
}

// Failed 表示任务以失败状态结束。
func (s WikiDeleteSpaceTaskStatus) Failed() bool {
	st := strings.ToLower(strings.TrimSpace(s.Status))
	return st == "failure" || st == "failed"
}

// DeleteWikiSpace 提交知识空间删除请求。返回 task_id 为空表示同步删除完成；
// 非空表示后端转为异步任务，需要后续轮询 GetWikiDeleteSpaceTask。
func DeleteWikiSpace(spaceID, userAccessToken string) (string, error) {
	c, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s", url.PathEscape(spaceID))
	resp, err := c.Delete(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("删除知识空间失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("删除知识空间失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return "", fmt.Errorf("删除知识空间响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("删除知识空间失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return parsed.Data.TaskID, nil
}

// GetWikiDeleteSpaceTask 查询 delete_space 异步任务的当前状态。
func GetWikiDeleteSpaceTask(taskID, userAccessToken string) (*WikiDeleteSpaceTaskStatus, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/tasks/%s?task_type=delete_space", url.PathEscape(taskID))
	resp, err := c.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询 delete_space 任务失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 delete_space 任务失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Task struct {
				TaskID            string `json:"task_id"`
				DeleteSpaceResult struct {
					Status    string `json:"status"`
					StatusMsg string `json:"status_msg"`
				} `json:"delete_space_result"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return nil, fmt.Errorf("查询 delete_space 任务响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("查询 delete_space 任务失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return &WikiDeleteSpaceTaskStatus{
		TaskID:    parsed.Data.Task.TaskID,
		Status:    parsed.Data.Task.DeleteSpaceResult.Status,
		StatusMsg: parsed.Data.Task.DeleteSpaceResult.StatusMsg,
	}, nil
}

// WikiDeleteNodeTaskStatus 表示 delete_node 异步任务的状态。
type WikiDeleteNodeTaskStatus struct {
	TaskID    string
	Status    string // success / failure / processing 等
	StatusMsg string
}

// Ready 表示任务已成功完成。
func (s WikiDeleteNodeTaskStatus) Ready() bool {
	return strings.EqualFold(strings.TrimSpace(s.Status), "success")
}

// Failed 表示任务以失败状态结束。
func (s WikiDeleteNodeTaskStatus) Failed() bool {
	st := strings.ToLower(strings.TrimSpace(s.Status))
	return st == "failure" || st == "failed"
}

// DeleteWikiNode 提交删除知识库节点请求。
// OpenAPI: DELETE /open-apis/wiki/v2/spaces/{space_id}/nodes/{node_token}
// body: {"obj_type": objType, "include_children": includeChildren}
// 返回 task_id 为空表示同步删除完成；非空表示转为异步任务。
func DeleteWikiNode(spaceID, nodeToken, objType string, includeChildren bool, userAccessToken string) (string, error) {
	c, err := GetClient()
	if err != nil {
		return "", err
	}
	if objType == "" {
		objType = "wiki"
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes/%s", url.PathEscape(spaceID), url.PathEscape(nodeToken))
	body := map[string]any{
		"obj_type":         objType,
		"include_children": includeChildren,
	}
	resp, err := c.Delete(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("删除知识库节点失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("删除知识库节点失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return "", fmt.Errorf("删除知识库节点响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("删除知识库节点失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return parsed.Data.TaskID, nil
}

// GetWikiDeleteNodeTask 查询 delete_node 异步任务的当前状态。
// OpenAPI: GET /open-apis/wiki/v2/tasks/{task_id}?task_type=delete_node
// 响应数据在 task.simple_task_result 下。
func GetWikiDeleteNodeTask(taskID, userAccessToken string) (*WikiDeleteNodeTaskStatus, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/tasks/%s?task_type=delete_node", url.PathEscape(taskID))
	resp, err := c.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询 delete_node 任务失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 delete_node 任务失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Task struct {
				TaskID           string `json:"task_id"`
				SimpleTaskResult struct {
					Status    string `json:"status"`
					StatusMsg string `json:"status_msg"`
				} `json:"simple_task_result"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return nil, fmt.Errorf("查询 delete_node 任务响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("查询 delete_node 任务失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}

	taskIDOut := parsed.Data.Task.TaskID
	if taskIDOut == "" {
		taskIDOut = taskID
	}
	status := parsed.Data.Task.SimpleTaskResult.Status
	statusMsg := parsed.Data.Task.SimpleTaskResult.StatusMsg
	if status == "" {
		status = "processing"
	}
	if statusMsg == "" {
		statusMsg = status
	}
	return &WikiDeleteNodeTaskStatus{
		TaskID:    taskIDOut,
		Status:    status,
		StatusMsg: statusMsg,
	}, nil
}
