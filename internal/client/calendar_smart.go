package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
)

// ============================================================================
// 智能时段建议 / 会议室查找
//
// 这两个能力（freebusy/suggestion 和 freebusy/room_find）在 SDK v3.5.3 中未
// 暴露专用方法，统一通过 client.Post 直调 OpenAPI。RSVP 走 SDK Reply 接口，
// 但额外补一个 ReplyEventToPrimary 方便不知道 calendar_id 时直接答复邀请。
// ============================================================================

const (
	// suggestionAPI 智能时段建议
	suggestionAPI = "/open-apis/calendar/v4/freebusy/suggestion"
	// roomFindAPI 会议室查找
	roomFindAPI = "/open-apis/calendar/v4/freebusy/room_find"
)

// SuggestionEventTime 单个时段（用于排除时段或返回的推荐时段）
type SuggestionEventTime struct {
	EventStartTime  string `json:"event_start_time,omitempty"`
	EventEndTime    string `json:"event_end_time,omitempty"`
	RecommendReason string `json:"recommend_reason,omitempty"`
}

// SuggestionRequest 智能时段建议入参
type SuggestionRequest struct {
	SearchStartTime    string                 `json:"search_start_time,omitempty"`
	SearchEndTime      string                 `json:"search_end_time,omitempty"`
	Timezone           string                 `json:"timezone,omitempty"`
	EventRrule         string                 `json:"event_rrule,omitempty"`
	DurationMinutes    int                    `json:"duration_minutes,omitempty"`
	AttendeeUserIDs    []string               `json:"attendee_user_ids,omitempty"`
	AttendeeChatIDs    []string               `json:"attendee_chat_ids,omitempty"`
	ExcludedEventTimes []*SuggestionEventTime `json:"excluded_event_times,omitempty"`
}

// SuggestionResult 智能时段建议返回
type SuggestionResult struct {
	Suggestions      []*SuggestionEventTime `json:"suggestions,omitempty"`
	AiActionGuidance string                 `json:"ai_action_guidance,omitempty"`
}

// SuggestFreebusy 调用 freebusy/suggestion 接口推荐可用时段
func SuggestFreebusy(req *SuggestionRequest, userAccessToken string) (*SuggestionResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), suggestionAPI, req, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询智能时段建议失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询智能时段建议失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int               `json:"code"`
		Msg  string            `json:"msg"`
		Data *SuggestionResult `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析智能时段建议响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询智能时段建议失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	if apiResp.Data == nil {
		return &SuggestionResult{}, nil
	}
	return apiResp.Data, nil
}

// RoomFindRequest 会议室查找单时段入参
type RoomFindRequest struct {
	City            string   `json:"city,omitempty"`
	Building        string   `json:"building,omitempty"`
	Floor           string   `json:"floor,omitempty"`
	RoomName        string   `json:"room_name,omitempty"`
	MinCapacity     int      `json:"min_capacity,omitempty"`
	MaxCapacity     int      `json:"max_capacity,omitempty"`
	EventStartTime  string   `json:"event_start_time,omitempty"`
	EventEndTime    string   `json:"event_end_time,omitempty"`
	AttendeeUserIDs []string `json:"attendee_user_ids,omitempty"`
	AttendeeChatIDs []string `json:"attendee_chat_ids,omitempty"`
	EventRrule      string   `json:"event_rrule,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
}

// RoomSuggestion 单个会议室候选
type RoomSuggestion struct {
	RoomID           string `json:"room_id,omitempty"`
	RoomName         string `json:"room_name,omitempty"`
	Capacity         int    `json:"capacity,omitempty"`
	ReserveUntilTime string `json:"reserve_until_time,omitempty"`
}

// RoomFindSlot 单个待查时段
type RoomFindSlot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// RoomFindTimeSlotResult 时段查询结果（含候选会议室）
type RoomFindTimeSlotResult struct {
	Start        string            `json:"start"`
	End          string            `json:"end"`
	MeetingRooms []*RoomSuggestion `json:"meeting_rooms,omitempty"`
}

// RoomFindResult 会议室查找返回
type RoomFindResult struct {
	TimeSlots []*RoomFindTimeSlotResult `json:"time_slots"`
}

// FindMeetingRoom 调用 freebusy/room_find 查询单时段可用会议室
// 自动处理 429 限流和 5xx 服务端临时错误：最多重试 3 次，full jitter 退避。
func FindMeetingRoom(req *RoomFindRequest, userAccessToken string) ([]*RoomSuggestion, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)

	retryCfg := RetryConfig{
		MaxRetries:       3,
		MaxTotalAttempts: 8,
		RetryOnRateLimit: true,
	}

	result := DoWithRetry(func() ([]*RoomSuggestion, http.Header, error) {
		resp, err := cli.Post(Context(), roomFindAPI, req, tokenType, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("查询可用会议室失败: %w", err)
		}
		headers := resp.Header
		if resp.StatusCode != http.StatusOK {
			return nil, headers, fmt.Errorf("查询可用会议室失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				AvailableRooms []*RoomSuggestion `json:"available_rooms"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return nil, headers, fmt.Errorf("解析会议室查找响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return nil, headers, fmt.Errorf("查询可用会议室失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		return apiResp.Data.AvailableRooms, headers, nil
	}, retryCfg)

	return result.Value, result.Err
}

// FindMeetingRoomBatch 并发查询多个时段的可用会议室，最多 workers 个并发
// 返回的 TimeSlots 按 Start 排序。任一时段查询失败立即返回第一个错误。
func FindMeetingRoomBatch(baseReq *RoomFindRequest, slots []RoomFindSlot, workers int, userAccessToken string) (*RoomFindResult, error) {
	if workers <= 0 {
		workers = 1
	}
	if len(slots) == 0 {
		return &RoomFindResult{}, nil
	}

	out := &RoomFindResult{
		TimeSlots: make([]*RoomFindTimeSlotResult, 0, len(slots)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, workers)

	for _, slot := range slots {
		wg.Add(1)
		sem <- struct{}{}
		go func(slot RoomFindSlot) {
			defer wg.Done()
			defer func() { <-sem }()

			req := *baseReq
			req.EventStartTime = slot.Start
			req.EventEndTime = slot.End
			rooms, err := FindMeetingRoom(&req, userAccessToken)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out.TimeSlots = append(out.TimeSlots, &RoomFindTimeSlotResult{
				Start:        slot.Start,
				End:          slot.End,
				MeetingRooms: rooms,
			})
		}(slot)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	sort.Slice(out.TimeSlots, func(i, j int) bool {
		return out.TimeSlots[i].Start < out.TimeSlots[j].Start
	})
	return out, nil
}

// SplitAttendeeIDs 把逗号分隔的混合 ID 列表按 ou_ / oc_ 前缀切分到两个 slice
// 返回 (userIDs, chatIDs, error)。ID 前后空白会被去掉；未识别前缀（如
// omm_/room_/app_ 会议室或资源 token）跳过并打 warn 到 stderr，不阻塞批量操作；
// 仅在完全无效（如显式给了非 ou_/oc_ 的"裸字符串"）时仍打 warn，不再返回 error。
// 保留 error 返回值是为了向后兼容签名，目前总是返回 nil。
func SplitAttendeeIDs(raw string) ([]string, []string, error) {
	var userIDs, chatIDs []string
	seenUsers := map[string]bool{}
	seenChats := map[string]bool{}

	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		switch {
		case strings.HasPrefix(id, "ou_"):
			if !seenUsers[id] {
				userIDs = append(userIDs, id)
				seenUsers[id] = true
			}
		case strings.HasPrefix(id, "oc_"):
			if !seenChats[id] {
				chatIDs = append(chatIDs, id)
				seenChats[id] = true
			}
		default:
			// 未知前缀（如 omm_/room_/app_ 会议室或第三方资源 token）跳过并打 warn 到 stderr，
			// 不阻塞批量操作；suggestion/room_find API 当前仅接收 user/chat ID。
			fmt.Fprintf(os.Stderr, "[calendar] 警告: 跳过未识别前缀的参与者 ID: %q（仅支持 ou_/oc_）\n", id)
			continue
		}
	}
	return userIDs, chatIDs, nil
}
