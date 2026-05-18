package client

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	larkattendance "github.com/larksuite/oapi-sdk-go/v3/service/attendance/v1"
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
// 权限要求: tenant_access_token（应用需获得 attendance:task:readonly 权限）
//
// 注：larksuite/oapi-sdk-go v3.5.3 中该接口 SupportedAccessTokenTypes 仅含 Tenant，
// SDK 在 validateTokenType 会拒绝 user_access_token，故本函数走默认 tenant token。
//
// employeeType 取值：employee_id（默认）/ open_id / user_id / employee_no
// userIDs 长度 ≤ 50，dateFrom/dateTo 为 yyyyMMdd
func QueryAttendanceUserTasks(
	employeeType string,
	userIDs []string,
	dateFrom int,
	dateTo int,
	needOvertime bool,
	ignoreInvalidUsers bool,
	includeTerminatedUser bool,
) (*AttendanceQueryUserTaskResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	if employeeType == "" {
		employeeType = "employee_id"
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("user_ids 不能为空")
	}
	if dateFrom == 0 || dateTo == 0 {
		return nil, fmt.Errorf("check_date_from / check_date_to 必填")
	}

	body := larkattendance.NewQueryUserTaskReqBodyBuilder().
		UserIds(userIDs).
		CheckDateFrom(dateFrom).
		CheckDateTo(dateTo).
		NeedOvertimeResult(needOvertime).
		Build()

	reqBuilder := larkattendance.NewQueryUserTaskReqBuilder().
		EmployeeType(employeeType).
		IgnoreInvalidUsers(ignoreInvalidUsers).
		IncludeTerminatedUser(includeTerminatedUser).
		Body(body)

	resp, err := cli.Attendance.UserTask.Query(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("查询考勤打卡记录失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询考勤打卡记录失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	out := &AttendanceQueryUserTaskResult{}
	if resp.Data == nil {
		return out, nil
	}
	out.InvalidUserIDs = resp.Data.InvalidUserIds
	out.UnauthorizedUserIDs = resp.Data.UnauthorizedUserIds
	for _, t := range resp.Data.UserTaskResults {
		if t == nil {
			continue
		}
		task := &AttendanceUserTask{
			ResultID:     StringVal(t.ResultId),
			UserID:       StringVal(t.UserId),
			EmployeeName: StringVal(t.EmployeeName),
			Day:          IntVal(t.Day),
			GroupID:      StringVal(t.GroupId),
			ShiftID:      StringVal(t.ShiftId),
		}
		for _, r := range t.Records {
			if r == nil {
				continue
			}
			task.Records = append(task.Records, &AttendanceTaskRecord{
				CheckInRecordID:          StringVal(r.CheckInRecordId),
				CheckOutRecordID:         StringVal(r.CheckOutRecordId),
				CheckInResult:            StringVal(r.CheckInResult),
				CheckOutResult:           StringVal(r.CheckOutResult),
				CheckInResultSupplement:  StringVal(r.CheckInResultSupplement),
				CheckOutResultSupplement: StringVal(r.CheckOutResultSupplement),
				CheckInShiftTime:         StringVal(r.CheckInShiftTime),
				CheckOutShiftTime:        StringVal(r.CheckOutShiftTime),
				TaskShiftType:            IntVal(r.TaskShiftType),
			})
		}
		out.UserTaskResults = append(out.UserTaskResults, task)
	}
	return out, nil
}

// QueryAttendanceUserStats 查询用户考勤统计数据
//
// 对应 OpenAPI: POST /open-apis/attendance/v1/user_stats_datas/query
// 权限要求: tenant_access_token（应用需获得 attendance:task:readonly 权限）
//
// 注：larksuite/oapi-sdk-go v3.5.3 中该接口 SupportedAccessTokenTypes 仅含 Tenant，
// SDK 在 validateTokenType 会拒绝 user_access_token，故本函数走默认 tenant token。
//
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

	bodyBuilder := larkattendance.NewQueryUserStatsDataReqBodyBuilder().
		StatsType(statsType).
		StartDate(startDate).
		EndDate(endDate).
		UserIds(userIDs).
		NeedHistory(needHistory).
		CurrentGroupOnly(currentGroupOnly)

	if locale != "" {
		bodyBuilder.Locale(locale)
	}
	if currentUserID != "" {
		bodyBuilder.UserId(currentUserID)
	}

	reqBuilder := larkattendance.NewQueryUserStatsDataReqBuilder().
		EmployeeType(employeeType).
		Body(bodyBuilder.Build())

	resp, err := cli.Attendance.UserStatsData.Query(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("查询考勤统计失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询考勤统计失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	out := &AttendanceQueryUserStatsResult{}
	if resp.Data == nil {
		return out, nil
	}
	out.InvalidUserList = resp.Data.InvalidUserList
	for _, u := range resp.Data.UserDatas {
		if u == nil {
			continue
		}
		us := &AttendanceUserStats{
			Name:   StringVal(u.Name),
			UserID: StringVal(u.UserId),
		}
		for _, c := range u.Datas {
			if c == nil {
				continue
			}
			us.Datas = append(us.Datas, &AttendanceUserStatsCell{
				Code:  StringVal(c.Code),
				Title: StringVal(c.Title),
				Value: StringVal(c.Value),
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
