package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// errTaskSearchPaginationLimit 表示搜索接口翻页已达服务端上限（offset 最大 150，错误码 1474004）。
// 自动翻页时遇到它应优雅停止，返回已收集的结果并提示用户缩小搜索范围。
var errTaskSearchPaginationLimit = errors.New("task search pagination limit reached")

// taskSearchPaginationLimitCode 服务端翻页上限错误码
const taskSearchPaginationLimitCode = 1474004

// TaskSearchOptions 任务搜索条件
type TaskSearchOptions struct {
	Query       string   // 关键词（服务端全文检索，匹配任务标题等）
	CreatorIDs  []string // 创建人 open_id 列表
	AssigneeIDs []string // 负责人 open_id 列表
	FollowerIDs []string // 关注人 open_id 列表
	Completed   *bool    // 完成状态过滤：true 已完成 / false 未完成 / nil 不限
	DueStart    string   // 截止时间下界（RFC3339），对应 filter.due_time.start_time
	DueEnd      string   // 截止时间上界（RFC3339），对应 filter.due_time.end_time
	PageSize    int      // 每页数量，默认 50
	PageToken   string   // 起始分页标记
	PageAll     bool     // 是否自动翻页拉取全部结果
	Enrich      bool     // 是否逐条拉取任务详情（false 时只返回 GUID + 链接，零额外往返）
}

// SearchTasksResult 任务搜索结果
type SearchTasksResult struct {
	Tasks     []*TaskInfo `json:"tasks"`
	PageToken string      `json:"page_token,omitempty"`
	HasMore   bool        `json:"has_more"`
	// Truncated 表示自动翻页时已达服务端翻页上限（offset 最多 150），结果被截断
	Truncated bool `json:"truncated,omitempty"`
}

// searchTaskHit 搜索接口返回的单条命中（仅含 GUID 与 app_link，详情需二次拉取）
type searchTaskHit struct {
	id      string
	appLink string
}

// taskSearchMaxPages 自动翻页时的最大页数保护，避免异常情况下无限拉取
const taskSearchMaxPages = 50

// taskSearchMaxPageSize 搜索接口单页数量上限（服务端限制，超出返回 1474003）
const taskSearchMaxPageSize = 30

// taskSearchEnrichWorkers 详情拉取的并发度（搜索接口只返回 GUID，需逐条 GetTask 补详情）
const taskSearchEnrichWorkers = 5

// SearchTasks 按条件搜索任务
//
// 底层调用飞书任务搜索接口 POST /open-apis/task/v2/tasks/search（SDK 未封装，走通用 HTTP）。
// 创建人 / 负责人 / 关注人 / 完成状态 / 截止时间等条件在服务端过滤，Query 为服务端关键词检索。
// 搜索接口只返回任务 GUID 与链接，因此对每条命中再调用 GetTask 拉取详情以展示标题、截止时间等。
func SearchTasks(opts TaskSearchOptions, userAccessToken string) (*SearchTasksResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	filter := map[string]interface{}{}
	if len(opts.CreatorIDs) > 0 {
		filter["creator_ids"] = opts.CreatorIDs
	}
	if len(opts.AssigneeIDs) > 0 {
		filter["assignee_ids"] = opts.AssigneeIDs
	}
	if len(opts.FollowerIDs) > 0 {
		filter["follower_ids"] = opts.FollowerIDs
	}
	if opts.Completed != nil {
		filter["is_completed"] = *opts.Completed
	}
	if opts.DueStart != "" || opts.DueEnd != "" {
		due := map[string]interface{}{}
		if opts.DueStart != "" {
			due["start_time"] = opts.DueStart
		}
		if opts.DueEnd != "" {
			due["end_time"] = opts.DueEnd
		}
		filter["due_time"] = due
	}

	// 搜索接口 page_size 上限为 30（超出返回 1474003），默认 20
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > taskSearchMaxPageSize {
		pageSize = taskSearchMaxPageSize
	}

	// query 与 filter 走请求体；page_size / page_token 是搜索接口的 query 参数
	body := map[string]interface{}{
		"query": opts.Query,
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	result := &SearchTasksResult{Tasks: make([]*TaskInfo, 0)}
	pageToken := opts.PageToken

	// 先按页收集所有命中的 GUID，保持接口返回顺序
	var allHits []searchTaskHit
	for page := 0; page < taskSearchMaxPages; page++ {
		hits, nextToken, hasMore, err := searchTasksPage(cli, pageSize, pageToken, body, userAccessToken)
		if err != nil {
			// 翻页越过服务端上限：优雅停止，返回已收集的结果并标记截断
			if errors.Is(err, errTaskSearchPaginationLimit) {
				result.HasMore = true
				result.Truncated = true
				result.PageToken = ""
				break
			}
			return nil, err
		}
		for _, hit := range hits {
			if hit.id != "" {
				allHits = append(allHits, hit)
			}
		}

		result.PageToken = nextToken
		result.HasMore = hasMore

		if !opts.PageAll || !hasMore || nextToken == "" {
			break
		}
		pageToken = nextToken
	}

	if opts.Enrich {
		result.Tasks = enrichTaskHits(allHits, userAccessToken)
	} else {
		// 免富化：只回 GUID + 链接，零额外 API 往返（只取 ID 的脚本场景）
		result.Tasks = make([]*TaskInfo, len(allHits))
		for i, hit := range allHits {
			result.Tasks[i] = &TaskInfo{Guid: hit.id, OriginHref: hit.appLink}
		}
	}
	return result, nil
}

// enrichTaskHits 对搜索命中的 GUID 并发拉取任务详情，结果保持输入顺序。
// 详情拉取失败时降级为仅返回 GUID 与链接，不中断整个搜索。
func enrichTaskHits(hits []searchTaskHit, userAccessToken string) []*TaskInfo {
	tasks := make([]*TaskInfo, len(hits))
	if len(hits) == 0 {
		return tasks
	}

	workers := taskSearchEnrichWorkers
	if len(hits) < workers {
		workers = len(hits)
	}

	indexCh := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indexCh {
				hit := hits[i]
				info, err := GetTask(hit.id, userAccessToken)
				if err != nil || info == nil {
					// 任务被删除/不可见（含服务端返回 success 但 task 为空的边界）：
					// 降级为最小信息，绝不把 nil 存进结果（文本输出会解引用）
					tasks[i] = &TaskInfo{Guid: hit.id, OriginHref: hit.appLink}
					continue
				}
				tasks[i] = info
			}
		}()
	}
	for i := range hits {
		indexCh <- i
	}
	close(indexCh)
	wg.Wait()

	return tasks
}

// searchTasksPage 拉取搜索接口的一页结果。page_size / page_token 作为 query 参数拼到 URL。
func searchTasksPage(cli *lark.Client, pageSize int, pageToken string, body map[string]interface{}, userAccessToken string) ([]searchTaskHit, string, bool, error) {
	tokenType, reqOpts := resolveTokenOpts(userAccessToken)

	apiPath := fmt.Sprintf("/open-apis/task/v2/tasks/search?page_size=%d", pageSize)
	if pageToken != "" {
		apiPath += "&page_token=" + url.QueryEscape(pageToken)
	}

	resp, err := cli.Post(Context(), apiPath, body, tokenType, reqOpts...)
	if err != nil {
		return nil, "", false, fmt.Errorf("搜索任务失败: %w", err)
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				ID       string `json:"id"`
				MetaData struct {
					AppLink string `json:"app_link"`
				} `json:"meta_data"`
			} `json:"items"`
			PageToken string `json:"page_token"`
			HasMore   bool   `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		if resp.StatusCode != 200 {
			return nil, "", false, fmt.Errorf("搜索任务失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}
		return nil, "", false, fmt.Errorf("解析搜索响应失败: %w", err)
	}
	if apiResp.Code == taskSearchPaginationLimitCode {
		return nil, "", false, errTaskSearchPaginationLimit
	}
	if apiResp.Code != 0 {
		return nil, "", false, fmt.Errorf("搜索任务失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	hits := make([]searchTaskHit, 0, len(apiResp.Data.Items))
	for _, item := range apiResp.Data.Items {
		hits = append(hits, searchTaskHit{id: item.ID, appLink: item.MetaData.AppLink})
	}
	return hits, apiResp.Data.PageToken, apiResp.Data.HasMore, nil
}
