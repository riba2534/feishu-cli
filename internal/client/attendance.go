package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// AttendanceUserTask 单个用户某天的打卡任务（聚合上下班两次打卡）
type AttendanceUserTask struct {
	ResultID     string                  `json:"result_id"`
	UserID       string                  `json:"user_id"`
	EmployeeName string                  `json:"employee_name,omitempty"`
	Day          int                     `json:"day"` // yyyyMMdd
	GroupID      string                  `json:"group_id,omitempty"`
	ShiftID      string                  `json:"shift_id,omitempty"`
	Records      []*AttendanceTaskRecord `json:"records,omitempty"`
}

// AttendanceTaskRecord 单条上下班打卡结果
type AttendanceTaskRecord struct {
	CheckInRecordID          string `json:"check_in_record_id,omitempty"`
	CheckOutRecordID         string `json:"check_out_record_id,omitempty"`
	CheckInResult            string `json:"check_in_result,omitempty"`
	CheckOutResult           string `json:"check_out_result,omitempty"`
	CheckInResultSupplement  string `json:"check_in_result_supplement,omitempty"`
	CheckOutResultSupplement string `json:"check_out_result_supplement,omitempty"`
	CheckInShiftTime         string `json:"check_in_shift_time,omitempty"`
	CheckOutShiftTime        string `json:"check_out_shift_time,omitempty"`
	TaskShiftType            int    `json:"task_shift_type,omitempty"`
}

// AttendanceQueryUserTaskResult 打卡记录查询结果
type AttendanceQueryUserTaskResult struct {
	UserTaskResults     []*AttendanceUserTask `json:"user_task_results"`
	InvalidUserIDs      []string              `json:"invalid_user_ids,omitempty"`
	UnauthorizedUserIDs []string              `json:"unauthorized_user_ids,omitempty"`
}

// AttendanceUserStats 单用户统计数据
type AttendanceUserStats struct {
	Name   string                     `json:"name"`
	UserID string                     `json:"user_id"`
	Datas  []*AttendanceUserStatsCell `json:"datas,omitempty"`
}

// AttendanceUserStatsCell 统计字段单元
type AttendanceUserStatsCell struct {
	Code  string `json:"code,omitempty"`
	Title string `json:"title,omitempty"`
	Value string `json:"value,omitempty"`
}

// AttendanceQueryUserStatsResult 统计查询结果
type AttendanceQueryUserStatsResult struct {
	UserDatas       []*AttendanceUserStats `json:"user_datas"`
	InvalidUserList []string               `json:"invalid_user_list,omitempty"`
}

// ParseAttendanceDate 接受 2006-01-02 / 20060102 两种格式，返回 yyyyMMdd 整数。
func ParseAttendanceDate(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("日期为空")
	}
	if strings.Contains(s, "-") {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return 0, fmt.Errorf("解析日期失败 %q: %w（期望 YYYY-MM-DD 或 YYYYMMDD）", s, err)
		}
		n, _ := strconv.Atoi(t.Format("20060102"))
		return n, nil
	}
	// 纯数字 yyyyMMdd
	if len(s) != 8 {
		return 0, fmt.Errorf("日期 %q 不是 YYYYMMDD 8 位数字", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("解析日期失败 %q: %w", s, err)
	}
	if _, err := time.Parse("20060102", s); err != nil {
		return 0, fmt.Errorf("日期 %q 无效: %w", s, err)
	}
	return n, nil
}

// QueryAttendanceUserTasks 查询用户考勤打卡记录
//
// 对应 OpenAPI: POST /open-apis/attendance/v1/user_tasks/query
// 权限要求: attendance:task:readonly 或 attendance:task
// 支持 User Access Token 和 Tenant Access Token
//
// employeeType 取值：employee_id（默认）/ employee_no
// 当 employeeType 为 employee_no 且 userIDs 为空时走官方本人自查路径
// userIDs 长度 ≤ 50，dateFrom/dateTo 为 yyyyMMdd
func QueryAttendanceUserTasks(
	employeeType string,
	userIDs []string,
	dateFrom int,
	dateTo int,
	needOvertime bool,
	ignoreInvalidUsers bool,
	includeTerminatedUser bool,
	userAccessToken ...string,
) (*AttendanceQueryUserTaskResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	if employeeType == "" {
		employeeType = "employee_id"
	}
	if len(userIDs) == 0 {
		if employeeType != "employee_no" {
			return nil, fmt.Errorf("employee_type 为 %s 时 user_ids 不能为空（查询本人请使用 employee_no 且留空 user_ids）", employeeType)
		}
	}
	if dateFrom == 0 || dateTo == 0 {
		return nil, fmt.Errorf("check_date_from / check_date_to 必填")
	}

	q := url.Values{}
	q.Set("employee_type", employeeType)
	q.Set("ignore_invalid_users", fmt.Sprintf("%v", ignoreInvalidUsers))
	q.Set("include_terminated_user", fmt.Sprintf("%v", includeTerminatedUser))

	apiPath := "/open-apis/attendance/v1/user_tasks/query?" + q.Encode()

	sendUserIDs := userIDs
	if sendUserIDs == nil {
		sendUserIDs = []string{}
	}

	body := map[string]any{
		"user_ids":             sendUserIDs,
		"check_date_from":      dateFrom,
		"check_date_to":        dateTo,
		"need_overtime_result": needOvertime,
	}

	uat := firstString(userAccessToken)
	tokenType, opts := resolveTokenOpts(uat)

	resp, err := cli.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询考勤打卡记录失败: %w", err)
	}
	// 先解析业务码：考勤业务错误（含缺 scope）随 HTTP 400 下发
	if err := CheckAPIResponse("查询考勤打卡记录", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserTaskResults []struct {
				ResultID     string `json:"result_id"`
				UserID       string `json:"user_id"`
				EmployeeName string `json:"employee_name"`
				Day          int    `json:"day"`
				GroupID      string `json:"group_id"`
				ShiftID      string `json:"shift_id"`
				Records      []struct {
					CheckInRecordID          string `json:"check_in_record_id"`
					CheckOutRecordID         string `json:"check_out_record_id"`
					CheckInResult            string `json:"check_in_result"`
					CheckOutResult           string `json:"check_out_result"`
					CheckInResultSupplement  string `json:"check_in_result_supplement"`
					CheckOutResultSupplement string `json:"check_out_result_supplement"`
					CheckInShiftTime         string `json:"check_in_shift_time"`
					CheckOutShiftTime        string `json:"check_out_shift_time"`
					TaskShiftType            int    `json:"task_shift_type"`
				} `json:"records"`
			} `json:"user_task_results"`
			InvalidUserIDs      []string `json:"invalid_user_ids"`
			UnauthorizedUserIDs []string `json:"unauthorized_user_ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析考勤打卡记录响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询考勤打卡记录失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	out := &AttendanceQueryUserTaskResult{
		InvalidUserIDs:      apiResp.Data.InvalidUserIDs,
		UnauthorizedUserIDs: apiResp.Data.UnauthorizedUserIDs,
	}
	for _, t := range apiResp.Data.UserTaskResults {
		task := &AttendanceUserTask{
			ResultID:     t.ResultID,
			UserID:       t.UserID,
			EmployeeName: t.EmployeeName,
			Day:          t.Day,
			GroupID:      t.GroupID,
			ShiftID:      t.ShiftID,
		}
		for _, r := range t.Records {
			task.Records = append(task.Records, &AttendanceTaskRecord{
				CheckInRecordID:          r.CheckInRecordID,
				CheckOutRecordID:         r.CheckOutRecordID,
				CheckInResult:            r.CheckInResult,
				CheckOutResult:           r.CheckOutResult,
				CheckInResultSupplement:  r.CheckInResultSupplement,
				CheckOutResultSupplement: r.CheckOutResultSupplement,
				CheckInShiftTime:         r.CheckInShiftTime,
				CheckOutShiftTime:        r.CheckOutShiftTime,
				TaskShiftType:            r.TaskShiftType,
			})
		}
		out.UserTaskResults = append(out.UserTaskResults, task)
	}
	return out, nil
}

// QueryAttendanceUserStats 查询用户考勤统计数据（legacy 路径）
//
// 对应 OpenAPI: POST /open-apis/attendance/v1/user_stats_datas/query
// 权限要求: attendance:task:readonly（Tenant Token）
//
// employeeType 取值：employee_id（默认）/ employee_no
// statsType: daily（日度）/ month（月度）
// userID 是发起人的用户 ID（同 查询统计设置 中的 user_id）
// startDate/endDate 间隔不超过 31 天
func QueryAttendanceUserStats(
	employeeType string,
	statsType string,
	startDate int,
	endDate int,
	userIDs []string,
	currentUserID string,
	locale string,
	needHistory bool,
	currentGroupOnly bool,
) (*AttendanceQueryUserStatsResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	if employeeType == "" {
		employeeType = "employee_id"
	}
	if statsType == "" {
		statsType = "daily"
	}
	if startDate == 0 || endDate == 0 {
		return nil, fmt.Errorf("start_date / end_date 必填")
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("user_ids 不能为空")
	}

	q := url.Values{}
	q.Set("employee_type", employeeType)

	apiPath := "/open-apis/attendance/v1/user_stats_datas/query?" + q.Encode()

	body := map[string]any{
		"stats_type":         statsType,
		"start_date":         startDate,
		"end_date":           endDate,
		"user_ids":           userIDs,
		"need_history":       needHistory,
		"current_group_only": currentGroupOnly,
	}
	if locale != "" {
		body["locale"] = locale
	}
	if currentUserID != "" {
		body["user_id"] = currentUserID
	}

	resp, err := cli.Post(Context(), apiPath, body, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, fmt.Errorf("查询考勤统计失败: %w", err)
	}
	// 先解析业务码：考勤业务错误（含缺 scope）随 HTTP 400 下发
	if err := CheckAPIResponse("查询考勤统计", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserDatas []struct {
				Name   string `json:"name"`
				UserID string `json:"user_id"`
				Datas  []struct {
					Code  string `json:"code"`
					Title string `json:"title"`
					Value string `json:"value"`
				} `json:"datas"`
			} `json:"user_datas"`
			InvalidUserList []string `json:"invalid_user_list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析考勤统计响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询考勤统计失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	out := &AttendanceQueryUserStatsResult{
		InvalidUserList: apiResp.Data.InvalidUserList,
	}
	for _, u := range apiResp.Data.UserDatas {
		us := &AttendanceUserStats{
			Name:   u.Name,
			UserID: u.UserID,
		}
		for _, c := range u.Datas {
			us.Datas = append(us.Datas, &AttendanceUserStatsCell{
				Code:  c.Code,
				Title: c.Title,
				Value: c.Value,
			})
		}
		out.UserDatas = append(out.UserDatas, us)
	}
	return out, nil
}

// FormatAttendanceDate 把 yyyyMMdd 整数格式化成 YYYY-MM-DD 字符串，便于人类阅读
func FormatAttendanceDate(d int) string {
	if d <= 0 {
		return ""
	}
	s := strconv.Itoa(d)
	if len(s) != 8 {
		return s
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:8]
}
