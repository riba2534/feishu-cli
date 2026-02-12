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

	info := &UserInfo{
		UserID:       StringVal(user.UserId),
		OpenID:       StringVal(user.OpenId),
		UnionID:      StringVal(user.UnionId),
		Name:         StringVal(user.Name),
		EnName:       StringVal(user.EnName),
		Nickname:     StringVal(user.Nickname),
		Email:        StringVal(user.Email),
		Mobile:       StringVal(user.Mobile),
		EmployeeNo:   StringVal(user.EmployeeNo),
		EmployeeType: IntVal(user.EmployeeType),
		Gender:       IntVal(user.Gender),
		City:         StringVal(user.City),
		Country:      StringVal(user.Country),
		WorkStation:  StringVal(user.WorkStation),
		JoinTime:     IntVal(user.JoinTime),
		IsTenantMgr:  BoolVal(user.IsTenantManager),
		JobTitle:     StringVal(user.JobTitle),
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

	return info, nil
}

// batchUserLimit 飞书 Batch API 单次最多查询 50 个用户
const batchUserLimit = 50

// BatchGetUserInfo 批量获取用户信息
// 使用飞书 /open-apis/contact/v3/users/batch API，每次最多 50 个，自动分批
func BatchGetUserInfo(userIDs []string, userIDType string) ([]*UserInfo, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	c, err := GetClient()
	if err != nil {
		return nil, err
	}

	if userIDType == "" {
		userIDType = "open_id"
	}

	var result []*UserInfo

	// 分批查询，每批最多 50 个
	for i := 0; i < len(userIDs); i += batchUserLimit {
		end := i + batchUserLimit
		if end > len(userIDs) {
			end = len(userIDs)
		}
		batch := userIDs[i:end]

		req := larkcontact.NewBatchUserReqBuilder().
			UserIds(batch).
			UserIdType(userIDType).
			Build()

		resp, err := c.Contact.User.Batch(Context(), req)
		if err != nil {
			return result, fmt.Errorf("批量获取用户信息失败: %w", err)
		}
		if !resp.Success() {
			return result, fmt.Errorf("批量获取用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
		}

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
					Nickname:     StringVal(user.Nickname),
					Email:        StringVal(user.Email),
					Mobile:       StringVal(user.Mobile),
					EmployeeNo:   StringVal(user.EmployeeNo),
					EmployeeType: IntVal(user.EmployeeType),
					Gender:       IntVal(user.Gender),
					City:         StringVal(user.City),
					Country:      StringVal(user.Country),
					WorkStation:  StringVal(user.WorkStation),
					JoinTime:     IntVal(user.JoinTime),
					IsTenantMgr:  BoolVal(user.IsTenantManager),
					JobTitle:     StringVal(user.JobTitle),
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
				result = append(result, info)
			}
		}
	}

	return result, nil
}
