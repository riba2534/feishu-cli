package client

import (
	"fmt"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
)

// UserInfo contains user information
type UserInfo struct {
	UserID       string `json:"user_id,omitempty"`
	OpenID       string `json:"open_id,omitempty"`
	UnionID      string `json:"union_id,omitempty"`
	Name         string `json:"name,omitempty"`
	EnName       string `json:"en_name,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	Email        string `json:"email,omitempty"`
	Mobile       string `json:"mobile,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	Status       string `json:"status,omitempty"`
	EmployeeNo   string `json:"employee_no,omitempty"`
	EmployeeType int    `json:"employee_type,omitempty"`
	Gender       int    `json:"gender,omitempty"`
	City         string `json:"city,omitempty"`
	Country      string `json:"country,omitempty"`
	WorkStation  string `json:"work_station,omitempty"`
	JoinTime     int    `json:"join_time,omitempty"`
	IsTenantMgr  bool   `json:"is_tenant_manager,omitempty"`
	JobTitle     string `json:"job_title,omitempty"`
}

// GetUserInfoOptions contains options for getting user info
type GetUserInfoOptions struct {
	UserIDType       string // open_id, union_id, user_id
	DepartmentIDType string // department_id, open_department_id
}

// GetUserInfo retrieves user information by user ID
func GetUserInfo(userID string, opts GetUserInfoOptions) (*UserInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// Default user ID type
	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	reqBuilder := larkcontact.NewGetUserReqBuilder().
		UserId(userID).
		UserIdType(opts.UserIDType)

	if opts.DepartmentIDType != "" {
		reqBuilder.DepartmentIdType(opts.DepartmentIDType)
	}

	resp, err := client.Contact.User.Get(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	user := resp.Data.User
	if user == nil {
		return nil, fmt.Errorf("用户不存在")
	}

	info := &UserInfo{}

	if user.UserId != nil {
		info.UserID = *user.UserId
	}
	if user.OpenId != nil {
		info.OpenID = *user.OpenId
	}
	if user.UnionId != nil {
		info.UnionID = *user.UnionId
	}
	if user.Name != nil {
		info.Name = *user.Name
	}
	if user.EnName != nil {
		info.EnName = *user.EnName
	}
	if user.Nickname != nil {
		info.Nickname = *user.Nickname
	}
	if user.Email != nil {
		info.Email = *user.Email
	}
	if user.Mobile != nil {
		info.Mobile = *user.Mobile
	}
	if user.Avatar != nil && user.Avatar.AvatarOrigin != nil {
		info.Avatar = *user.Avatar.AvatarOrigin
	}
	if user.Status != nil && user.Status.IsFrozen != nil {
		if *user.Status.IsFrozen {
			info.Status = "frozen"
		} else {
			info.Status = "active"
		}
	}
	if user.EmployeeNo != nil {
		info.EmployeeNo = *user.EmployeeNo
	}
	if user.EmployeeType != nil {
		info.EmployeeType = *user.EmployeeType
	}
	if user.Gender != nil {
		info.Gender = *user.Gender
	}
	if user.City != nil {
		info.City = *user.City
	}
	if user.Country != nil {
		info.Country = *user.Country
	}
	if user.WorkStation != nil {
		info.WorkStation = *user.WorkStation
	}
	if user.JoinTime != nil {
		info.JoinTime = *user.JoinTime
	}
	if user.IsTenantManager != nil {
		info.IsTenantMgr = *user.IsTenantManager
	}
	if user.JobTitle != nil {
		info.JobTitle = *user.JobTitle
	}

	return info, nil
}
