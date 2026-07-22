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
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("meeting_no", meetingNo)
	params.Set("start_time", strconv.FormatInt(startSec, 10))
	params.Set("end_time", strconv.FormatInt(endSec, 10))
	apiPath := fmt.Sprintf("%s/meetings/list_by_no?%s", vcBase, params.Encode())

	tokenType, opts := resolveTokenOpts(userAccessToken)

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("按会议号查询会议失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("按会议号查询会议失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("按会议号查询会议失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}
