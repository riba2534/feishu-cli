package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"

	"github.com/riba2534/feishu-cli/internal/config"
)

// UserContactIDInfo 用户联系信息（通过邮箱/手机号查询）
//
// 注意：OpenID / UserID / UnionID 分别是三种不同的 ID。batch_get_id 每次只按一种
// user_id_type 返回，旧版只按 open_id 查询却把结果写进了 user_id 字段（实为 open_id）；
// 现按类型分别查询并填入对应字段，拿不到（如应用缺 contact:user.employee_id:readonly）时留空。
type UserContactIDInfo struct {
	UserID  string `json:"user_id,omitempty"`
	OpenID  string `json:"open_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
	Mobile  string `json:"mobile,omitempty"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
}

// BatchGetUsersBasic 通过 open_id 批量获取用户基本资料（姓名/头像），用于补齐消息列表中
// 的发送者名字。底层调用 POST /open-apis/contact/v3/users/basic_batch，需要 User Token；
// 该端点权限更轻，目标用户即使不在 app 可见范围内也能返回 basic profile。
// 单次最多 50 个 open_id，超出会自动分批。失败时返回已解出的部分结果 + 错误。
func BatchGetUsersBasic(openIDs []string, userAccessToken string) (map[string]string, error) {
	if userAccessToken == "" {
		return nil, fmt.Errorf("批量查询用户基本资料需要 User Access Token")
	}

	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	nameMap := make(map[string]string)
	const batchSize = 10 // basic_batch API 单次最多 10 个 user_ids
	for i := 0; i < len(openIDs); i += batchSize {
		end := i + batchSize
		if end > len(openIDs) {
			end = len(openIDs)
		}
		batch := openIDs[i:end]

		bodyBytes, err := json.Marshal(map[string]any{"user_ids": batch})
		if err != nil {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: %w", err)
		}

		reqURL := fmt.Sprintf("%s/open-apis/contact/v3/users/basic_batch?user_id_type=open_id", baseURL)
		req, err := http.NewRequestWithContext(Context(), http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+userAccessToken)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		httpResp, err := rawHTTPClient().Do(req)
		if err != nil {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: %w", err)
		}
		body, readErr := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if readErr != nil {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: 读取响应失败: %w", readErr)
		}

		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Users []struct {
					UserID string `json:"user_id"`
					Name   string `json:"name"`
				} `json:"users"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: 解析响应失败: %w", err)
		}
		if resp.Code != 0 {
			return nameMap, fmt.Errorf("批量查询用户基本资料失败: code=%d, msg=%s", resp.Code, resp.Msg)
		}
		for _, u := range resp.Data.Users {
			if u.UserID != "" && u.Name != "" {
				nameMap[u.UserID] = u.Name
			}
		}
	}
	return nameMap, nil
}

// SearchUserItem 搜索用户接口返回的单个用户
type SearchUserItem struct {
	OpenID        string   `json:"open_id,omitempty"`
	UserID        string   `json:"user_id,omitempty"`
	Name          string   `json:"name,omitempty"`
	DepartmentIDs []string `json:"department_ids,omitempty"`
	AvatarURL     string   `json:"avatar_url,omitempty"`
}

// SearchUsersResult 搜索用户接口的返回
type SearchUsersResult struct {
	Users     []*SearchUserItem `json:"users"`
	PageToken string            `json:"page_token,omitempty"`
	HasMore   bool              `json:"has_more"`
}

// SearchUsers 通过关键词（姓名/邮箱/手机号）搜索用户，返回 open_id 等信息。
// 底层调用 GET /open-apis/search/v1/user，需要 User Token + scope contact:user:search。
// SDK 未封装该端点，直接走 raw HTTP；与 listMessagesWithUserToken 保持相同风格。
func SearchUsers(query string, pageSize int, pageToken, userAccessToken string) (*SearchUsersResult, error) {
	if userAccessToken == "" {
		return nil, fmt.Errorf("搜索用户需要 User Access Token")
	}
	if query == "" {
		return nil, fmt.Errorf("搜索用户必须提供 query")
	}

	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	params := url.Values{}
	params.Set("query", query)
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		params.Set("page_token", pageToken)
	}

	reqURL := fmt.Sprintf("%s/open-apis/search/v1/user?%s", baseURL, params.Encode())
	req, err := http.NewRequestWithContext(Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("搜索用户失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	httpResp, err := rawHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索用户失败: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("搜索用户失败: 读取响应失败: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Users []struct {
				OpenID        string   `json:"open_id"`
				UserID        string   `json:"user_id"`
				Name          string   `json:"name"`
				DepartmentIDs []string `json:"department_ids"`
				AvatarURL     string   `json:"avatar_url"`
			} `json:"users"`
			PageToken string `json:"page_token"`
			HasMore   bool   `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("搜索用户失败: 解析响应失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("搜索用户失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchUsersResult{
		PageToken: resp.Data.PageToken,
		HasMore:   resp.Data.HasMore,
	}
	for _, u := range resp.Data.Users {
		result.Users = append(result.Users, &SearchUserItem{
			OpenID:        u.OpenID,
			UserID:        u.UserID,
			Name:          u.Name,
			DepartmentIDs: u.DepartmentIDs,
			AvatarURL:     u.AvatarURL,
		})
	}
	return result, nil
}

// DepartmentInfo 部门信息
type DepartmentInfo struct {
	Name               string `json:"name"`
	DepartmentID       string `json:"department_id,omitempty"`
	OpenDepartmentID   string `json:"open_department_id,omitempty"`
	ParentDepartmentID string `json:"parent_department_id,omitempty"`
	LeaderUserID       string `json:"leader_user_id,omitempty"`
	ChatID             string `json:"chat_id,omitempty"`
	MemberCount        int    `json:"member_count,omitempty"`
}

// batchGetUserIDByType 调用 batch_get_id，按指定 user_id_type 返回（item.UserId 即该类型的 ID）。
func batchGetUserIDByType(emails, mobiles []string, userIDType string) ([]*larkcontact.UserContactInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkcontact.NewBatchGetIdUserReqBodyBuilder()
	if len(emails) > 0 {
		bodyBuilder.Emails(emails)
	}
	if len(mobiles) > 0 {
		bodyBuilder.Mobiles(mobiles)
	}

	req := larkcontact.NewBatchGetIdUserReqBuilder().
		UserIdType(userIDType).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Contact.User.BatchGetId(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("批量查询用户 ID 失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("批量查询用户 ID 失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil {
		return nil, nil
	}
	return resp.Data.UserList, nil
}

// contactLookupKey 以邮箱（忽略大小写）或手机号作为 batch_get_id 结果的对齐键。
func contactLookupKey(email, mobile string) string {
	if email != "" {
		return "e:" + strings.ToLower(strings.TrimSpace(email))
	}
	return "m:" + strings.TrimSpace(mobile)
}

// BatchGetUserID 通过邮箱或手机号批量获取用户 ID（App/Tenant 身份）。
//
// open_id 必查（失败即报错）；user_id、union_id 各再查一次，失败（多为权限不足）时
// 对应字段留空，不影响 open_id 结果。
func BatchGetUserID(emails, mobiles []string) ([]*UserContactIDInfo, error) {
	openList, err := batchGetUserIDByType(emails, mobiles, "open_id")
	if err != nil {
		return nil, err
	}

	var result []*UserContactIDInfo
	index := map[string]*UserContactIDInfo{}
	for _, item := range openList {
		info := &UserContactIDInfo{
			OpenID: StringVal(item.UserId),
			Mobile: StringVal(item.Mobile),
			Email:  StringVal(item.Email),
		}
		result = append(result, info)
		index[contactLookupKey(info.Email, info.Mobile)] = info
	}
	if len(result) == 0 {
		return result, nil
	}

	for _, idType := range []string{"user_id", "union_id"} {
		list, err := batchGetUserIDByType(emails, mobiles, idType)
		if err != nil {
			continue // best-effort：缺少对应权限时留空
		}
		for _, item := range list {
			info := index[contactLookupKey(StringVal(item.Email), StringVal(item.Mobile))]
			if info == nil {
				continue
			}
			switch idType {
			case "user_id":
				info.UserID = StringVal(item.UserId)
			case "union_id":
				info.UnionID = StringVal(item.UserId)
			}
		}
	}
	return result, nil
}

// ListUsers 列出部门下的用户
func ListUsers(departmentID, userIDType string, pageSize int, pageToken string) ([]*UserInfo, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	if userIDType == "" {
		userIDType = "open_id"
	}

	reqBuilder := larkcontact.NewListUserReqBuilder().
		DepartmentId(departmentID).
		UserIdType(userIDType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Contact.User.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取用户列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取用户列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var users []*UserInfo
	if resp.Data != nil && resp.Data.Items != nil {
		for _, user := range resp.Data.Items {
			if user == nil {
				continue
			}
			info := &UserInfo{
				UserID:       StringVal(user.UserId),
				OpenID:       StringVal(user.OpenId),
				UnionID:      StringVal(user.UnionId),
				Name:         StringVal(user.Name),
				EnName:       StringVal(user.EnName),
				Email:        StringVal(user.Email),
				Mobile:       StringVal(user.Mobile),
				EmployeeNo:   StringVal(user.EmployeeNo),
				EmployeeType: IntVal(user.EmployeeType),
				Gender:       IntVal(user.Gender),
				JobTitle:     StringVal(user.JobTitle),
			}
			if user.Status != nil && user.Status.IsFrozen != nil {
				if *user.Status.IsFrozen {
					info.Status = "frozen"
				} else {
					info.Status = "active"
				}
			}
			users = append(users, info)
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return users, nextPageToken, hasMore, nil
}

// GetDepartment 获取部门详情
func GetDepartment(departmentID, userIDType, departmentIDType string) (*DepartmentInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if userIDType == "" {
		userIDType = "open_id"
	}
	if departmentIDType == "" {
		departmentIDType = "open_department_id"
	}

	req := larkcontact.NewGetDepartmentReqBuilder().
		DepartmentId(departmentID).
		UserIdType(userIDType).
		DepartmentIdType(departmentIDType).
		Build()

	resp, err := client.Contact.Department.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取部门信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取部门信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Department == nil {
		return nil, fmt.Errorf("部门不存在")
	}

	return convertDepartment(resp.Data.Department), nil
}

// ListDepartments 列出子部门
func ListDepartments(parentDepartmentID, userIDType, departmentIDType string, pageSize int, pageToken string) ([]*DepartmentInfo, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	if userIDType == "" {
		userIDType = "open_id"
	}
	if departmentIDType == "" {
		departmentIDType = "open_department_id"
	}

	reqBuilder := larkcontact.NewChildrenDepartmentReqBuilder().
		DepartmentId(parentDepartmentID).
		UserIdType(userIDType).
		DepartmentIdType(departmentIDType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Contact.Department.Children(Context(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取子部门列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取子部门列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var depts []*DepartmentInfo
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			if item == nil {
				continue
			}
			depts = append(depts, convertDepartment(item))
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return depts, nextPageToken, hasMore, nil
}

// convertDepartment 转换 SDK Department 为 DepartmentInfo
func convertDepartment(dept *larkcontact.Department) *DepartmentInfo {
	if dept == nil {
		return nil
	}
	return &DepartmentInfo{
		Name:               StringVal(dept.Name),
		DepartmentID:       StringVal(dept.DepartmentId),
		OpenDepartmentID:   StringVal(dept.OpenDepartmentId),
		ParentDepartmentID: StringVal(dept.ParentDepartmentId),
		LeaderUserID:       StringVal(dept.LeaderUserId),
		ChatID:             StringVal(dept.ChatId),
		MemberCount:        IntVal(dept.MemberCount),
	}
}
