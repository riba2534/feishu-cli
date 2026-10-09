package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ListMeetingsByNo 按会议号获取关联的会议列表（仅支持查询近 90 天内的会议）
// API: GET /open-apis/vc/v1/meetings/list_by_no?meeting_no=&start_time=&end_time=
// start/end 为 Unix 秒时间戳。返回 data 原始 JSON（含 meeting_briefs[]，每项的 id 即 meeting_id）。
//
// 权限：User/Tenant Token 均可，需 vc:meeting:readonly 或 vc:meeting.meetingid:read。
// 一个会议号可能对应多场会议（周期性会议每次实例的 meeting_id 不同），故返回列表。
func ListMeetingsByNo(meetingNo string, startSec, endSec int64, userAccessToken string) (json.RawMessage, error) {
	params := url.Values{}
	params.Set("meeting_no", meetingNo)
	params.Set("start_time", strconv.FormatInt(startSec, 10))
	params.Set("end_time", strconv.FormatInt(endSec, 10))
	apiPath := fmt.Sprintf("%s/meetings/list_by_no?%s", vcBase, params.Encode())

	return vcCallAPI("按会议号查询会议", http.MethodGet, apiPath, nil, userAccessToken)
}
