package client

import (
	"fmt"
	"strings"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
)

// UserContactIDInfo 用户联系信息（通过邮箱/手机号查询）
type UserContactIDInfo struct {
	UserID string `json:"user_id,omitempty"`
	Mobile string `json:"mobile,omitempty"`
	Email  string `json:"email,omitempty"`
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

// BatchGetUserID 通过邮箱或手机号批量获取用户 ID
func BatchGetUserID(emails, mobiles []string) ([]*UserContactIDInfo, error) {
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
		UserIdType("open_id").
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Contact.User.BatchGetId(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("批量查询用户 ID 失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("批量查询用户 ID 失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var result []*UserContactIDInfo
	if resp.Data != nil && resp.Data.UserList != nil {
		for _, item := range resp.Data.UserList {
			result = append(result, &UserContactIDInfo{
				UserID: StringVal(item.UserId),
				Mobile: StringVal(item.Mobile),
				Email:  StringVal(item.Email),
			})
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

// SearchUsersByName 递归遍历所有部门，按姓名模糊搜索用户
func SearchUsersByName(name string) ([]*UserInfo, error) {
	var matched []*UserInfo
	nameLower := strings.ToLower(name)

	var searchDept func(deptID string) error
	searchDept = func(deptID string) error {
		// 列出当前部门的所有用户（自动分页）
		pageToken := ""
		for {
			users, nextToken, hasMore, err := ListUsers(deptID, "open_id", 50, pageToken)
			if err != nil {
				return err
			}
			for _, u := range users {
				if strings.Contains(strings.ToLower(u.Name), nameLower) ||
					strings.Contains(strings.ToLower(u.EnName), nameLower) ||
					strings.Contains(strings.ToLower(u.Nickname), nameLower) {
					matched = append(matched, u)
				}
			}
			if !hasMore {
				break
			}
			pageToken = nextToken
		}

		// 递归子部门（自动分页）
		pageToken = ""
		for {
			depts, nextToken, hasMore, err := ListDepartments(deptID, "open_id", "open_department_id", 50, pageToken)
			if err != nil {
				return err
			}
			for _, d := range depts {
				if err := searchDept(d.OpenDepartmentID); err != nil {
					return err
				}
			}
			if !hasMore {
				break
			}
			pageToken = nextToken
		}

		return nil
	}

	if err := searchDept("0"); err != nil {
		return nil, err
	}

	return matched, nil
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
