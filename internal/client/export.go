package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
)

// CreateExportTask 创建导出任务，返回任务 ticket
func CreateExportTask(docToken, docType, fileExtension, userAccessToken string) (string, error) {
	return CreateExportTaskWithSubId(docToken, docType, fileExtension, "", userAccessToken)
}

// CreateExportTaskWithSubId 创建导出任务（支持子表 ID），返回任务 ticket
// subId 用于将电子表格/多维表格导出为 CSV 时指定工作表/数据表 ID，为空时忽略
func CreateExportTaskWithSubId(docToken, docType, fileExtension, subId, userAccessToken string) (string, error) {
	return CreateExportTaskEx(docToken, docType, fileExtension, subId, false, userAccessToken)
}

// CreateExportTaskEx 创建导出任务，支持 bitable→base 的 only_schema。
// SDK v3.5.3 的 ExportTask 没有 OnlySchema 字段，走 raw JSON 以对齐官方契约。
func CreateExportTaskEx(docToken, docType, fileExtension, subId string, onlySchema bool, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	body := map[string]any{
		"token":          docToken,
		"type":           docType,
		"file_extension": fileExtension,
	}
	if strings.TrimSpace(subId) != "" {
		body["sub_id"] = subId
	}
	if onlySchema {
		body["only_schema"] = true
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), "/open-apis/drive/v1/export_tasks", body, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("创建导出任务失败: %w", err)
	}
	// 业务错误常随 HTTP 400 下发：先解析飞书信封里的 code，再看 HTTP 状态
	if err := CheckAPIResponse("创建导出任务", resp); err != nil {
		return "", err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析导出任务响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("创建导出任务失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data.Ticket == "" {
		return "", fmt.Errorf("创建导出任务成功但未返回 ticket")
	}
	return apiResp.Data.Ticket, nil
}

// GetExportTask 查询导出任务状态，返回 jobStatus、fileToken、error
// jobStatus: 0=成功, 1=初始化, 2=处理中
func GetExportTask(ticket, docToken, userAccessToken string) (int, string, error) {
	client, err := GetClient()
	if err != nil {
		return -1, "", err
	}

	req := larkdrive.NewGetExportTaskReqBuilder().
		Ticket(ticket).
		Token(docToken).
		Build()

	resp, err := client.Drive.ExportTask.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return -1, "", fmt.Errorf("查询导出任务失败: %w", err)
	}

	if !resp.Success() {
		return -1, "", fmt.Errorf("查询导出任务失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Result == nil {
		return -1, "", fmt.Errorf("查询导出任务未返回结果")
	}

	result := resp.Data.Result
	jobStatus := IntVal(result.JobStatus)
	fileToken := StringVal(result.FileToken)

	// jobStatus 0=成功, 1=初始化, 2=处理中, 其他=失败
	if jobStatus != 0 && jobStatus != 1 && jobStatus != 2 {
		errMsg := "未知错误"
		if result.JobErrorMsg != nil && *result.JobErrorMsg != "" {
			errMsg = *result.JobErrorMsg
		}
		return jobStatus, "", fmt.Errorf("导出任务失败: %s", errMsg)
	}

	return jobStatus, fileToken, nil
}

// DownloadExportFile 下载导出任务生成的文件。
// 流式下载（不把整个文件读进内存）+ 分片重试 + 空闲超时 + 原子写；业务错误（含 HTTP 200 + JSON）不落盘。
func DownloadExportFile(fileToken, outputPath, userAccessToken string) error {
	if err := validatePath(outputPath); err != nil {
		return err
	}
	bearer, err := driveDownloadBearer(userAccessToken)
	if err != nil {
		return err
	}
	_, _, err = downloadStreamToFile(downloadStreamSpec{
		Action: "下载导出文件",
		URL:    buildOpenAPIURL("/open-apis/drive/v1/export_tasks/file/" + url.PathEscape(fileToken) + "/download"),
		Bearer: bearer,
	}, outputPath, 0)
	return err
}

// WaitExportTask 轮询等待导出任务完成，返回导出文件的 fileToken
func WaitExportTask(ticket, docToken, userAccessToken string, maxRetries int) (string, error) {
	for i := 0; i < maxRetries; i++ {
		jobStatus, fileToken, err := GetExportTask(ticket, docToken, userAccessToken)
		if err != nil {
			return "", err
		}

		if jobStatus == 0 {
			return fileToken, nil
		}

		// jobStatus 1=初始化, 2=处理中
		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("导出任务超时，已等待 %d 秒", maxRetries)
}

// CreateImportTask 创建导入任务，返回任务 ticket（App Token）
func CreateImportTask(fileToken, fileType, fileName, targetType, folderToken string) (string, error) {
	return CreateImportTaskWithToken(fileToken, fileType, fileName, targetType, folderToken, "")
}

// CreateImportTaskWithToken 创建导入任务，支持 User Access Token 覆盖。
// 官方协议始终携带 point（mount_type=1）；省略 folderToken 时 mount_key 为空字符串，表示调用方根目录。
func CreateImportTaskWithToken(fileToken, fileType, fileName, targetType, folderToken, userAccessToken string) (string, error) {
	return CreateImportTaskEx(fileToken, fileType, fileName, targetType, folderToken, "", userAccessToken)
}

// CreateImportTaskEx 创建导入任务，支持 bitable --target-token（写入已有多维表格）。
func CreateImportTaskEx(fileToken, fileType, fileName, targetType, folderToken, targetToken, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	taskBuilder := larkdrive.NewImportTaskBuilder().
		FileExtension(fileType).
		FileToken(fileToken).
		Type(targetType).
		Point(larkdrive.NewImportTaskMountPointBuilder().
			MountType(1).
			MountKey(folderToken).
			Build())

	if fileName != "" {
		taskBuilder.FileName(fileName)
	}
	if targetType == "bitable" && strings.TrimSpace(targetToken) != "" {
		taskBuilder.Token(targetToken)
	}

	req := larkdrive.NewCreateImportTaskReqBuilder().
		ImportTask(taskBuilder.Build()).
		Build()

	resp, err := cli.Drive.ImportTask.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("创建导入任务失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("创建导入任务失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Ticket == nil {
		return "", fmt.Errorf("创建导入任务成功但未返回 ticket")
	}

	return *resp.Data.Ticket, nil
}

// GetImportTask 查询导入任务状态，返回 jobStatus、docToken、url、error
// jobStatus: 0=成功, 1=初始化, 2=处理中
func GetImportTask(ticket, userAccessToken string) (int, string, string, error) {
	client, err := GetClient()
	if err != nil {
		return -1, "", "", err
	}

	req := larkdrive.NewGetImportTaskReqBuilder().
		Ticket(ticket).
		Build()

	resp, err := client.Drive.ImportTask.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return -1, "", "", fmt.Errorf("查询导入任务失败: %w", err)
	}

	if !resp.Success() {
		return -1, "", "", fmt.Errorf("查询导入任务失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Result == nil {
		return -1, "", "", fmt.Errorf("查询导入任务未返回结果")
	}

	result := resp.Data.Result
	jobStatus := IntVal(result.JobStatus)
	docToken := StringVal(result.Token)
	url := StringVal(result.Url)

	// jobStatus 0=成功, 1=初始化, 2=处理中, 其他=失败
	if jobStatus != 0 && jobStatus != 1 && jobStatus != 2 {
		errMsg := "未知错误"
		if result.JobErrorMsg != nil && *result.JobErrorMsg != "" {
			errMsg = *result.JobErrorMsg
		}
		return jobStatus, "", "", fmt.Errorf("导入任务失败: %s", errMsg)
	}

	return jobStatus, docToken, url, nil
}

// WaitImportTask 轮询等待导入任务完成，返回文档 token 和 url
func WaitImportTask(ticket string, maxRetries int, userAccessToken string) (string, string, error) {
	for i := 0; i < maxRetries; i++ {
		jobStatus, docToken, url, err := GetImportTask(ticket, userAccessToken)
		if err != nil {
			return "", "", err
		}

		if jobStatus == 0 {
			return docToken, url, nil
		}

		time.Sleep(1 * time.Second)
	}

	return "", "", fmt.Errorf("导入任务超时，已等待 %d 秒", maxRetries)
}

// ==================== 扩展：有界轮询 + Markdown 快捷路径 + Resume 模式 ====================

// Drive 导出 / 导入 轮询参数（变量便于测试缩短间隔）
var (
	DriveExportMaxAttempts  = 10
	DriveExportPollInterval = 5 * time.Second
	DriveImportMaxAttempts  = 30
	DriveImportPollInterval = 2 * time.Second
	DriveMoveMaxAttempts    = 30
	DriveMovePollInterval   = 2 * time.Second
)

// DriveExportStatus 导出任务状态（归一化）
type DriveExportStatus struct {
	Ticket        string `json:"ticket"`
	FileExtension string `json:"file_extension"`
	DocType       string `json:"doc_type"`
	FileName      string `json:"file_name"`
	FileToken     string `json:"file_token"`
	JobErrorMsg   string `json:"job_error_msg"`
	FileSize      int64  `json:"file_size"`
	JobStatus     int    `json:"job_status"`
}

// Ready 任务已完成且有 file_token
func (s *DriveExportStatus) Ready() bool {
	return s != nil && s.FileToken != "" && s.JobStatus == 0
}

// Pending 任务进行中
func (s *DriveExportStatus) Pending() bool {
	if s == nil {
		return false
	}
	return s.JobStatus == 1 || s.JobStatus == 2 || (s.JobStatus == 0 && s.FileToken == "")
}

// Failed 任务失败
func (s *DriveExportStatus) Failed() bool {
	return s != nil && !s.Ready() && !s.Pending() && s.JobStatus != 0
}

// StatusLabel 返回人类可读的状态标签
func (s *DriveExportStatus) StatusLabel() string {
	if s == nil {
		return "unknown"
	}
	switch s.JobStatus {
	case 0:
		if s.FileToken != "" {
			return "success"
		}
		return "pending"
	case 1:
		return "new"
	case 2:
		return "processing"
	case 3:
		return "internal_error"
	case 107:
		return "export_size_limit"
	case 108:
		return "timeout"
	case 109:
		return "export_block_not_permitted"
	case 110:
		return "no_permission"
	case 111:
		return "docs_deleted"
	case 122:
		return "export_denied_on_copying"
	case 123:
		return "docs_not_exist"
	case 6000:
		return "export_images_exceed_limit"
	default:
		return fmt.Sprintf("status_%d", s.JobStatus)
	}
}

// GetDriveExportStatus 查询导出任务当前状态（归一化）
func GetDriveExportStatus(ticket, docToken, userAccessToken string) (*DriveExportStatus, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetExportTaskReqBuilder().
		Ticket(ticket).
		Token(docToken).
		Build()

	resp, err := client.Drive.ExportTask.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("查询导出任务失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询导出任务失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.Result == nil {
		return &DriveExportStatus{Ticket: ticket}, nil
	}
	r := resp.Data.Result
	return &DriveExportStatus{
		Ticket:        ticket,
		FileExtension: StringVal(r.FileExtension),
		DocType:       StringVal(r.Type),
		FileName:      StringVal(r.FileName),
		FileToken:     StringVal(r.FileToken),
		JobErrorMsg:   StringVal(r.JobErrorMsg),
		FileSize:      int64(IntVal(r.FileSize)),
		JobStatus:     IntVal(r.JobStatus),
	}, nil
}

// FetchDocMetaURL 批量查询文档元数据，返回可访问 URL（with_url=true）。
func FetchDocMetaURL(docToken, docType, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"request_docs": []map[string]any{
			{"doc_token": docToken, "doc_type": docType},
		},
		"with_url": true,
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), "/open-apis/drive/v1/metas/batch_query", body, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("查询文档 URL 失败: %w", err)
	}
	if err := CheckAPIResponse("查询文档 URL", resp); err != nil {
		return "", err
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Metas []struct {
				URL string `json:"url"`
			} `json:"metas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析文档元数据响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("查询文档 URL 失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if len(apiResp.Data.Metas) == 0 {
		return "", nil
	}
	return apiResp.Data.Metas[0].URL, nil
}

// FetchDocMetaTitle 批量查询文档元数据，返回标题
// API: POST /open-apis/drive/v1/metas/batch_query
func FetchDocMetaTitle(docToken, docType, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	body := map[string]any{
		"request_docs": []map[string]any{
			{"doc_token": docToken, "doc_type": docType},
		},
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Post(Context(), "/open-apis/drive/v1/metas/batch_query", body, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("查询文档元数据失败: %w", err)
	}
	if err := CheckAPIResponse("查询文档元数据", resp); err != nil {
		return "", err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Metas []struct {
				DocToken string `json:"doc_token"`
				Title    string `json:"title"`
				DocType  string `json:"doc_type"`
			} `json:"metas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析文档元数据响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("查询文档元数据失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if len(apiResp.Data.Metas) == 0 {
		return "", nil
	}
	return apiResp.Data.Metas[0].Title, nil
}

// FetchDocxMarkdownContent 通过 V2 docs_ai fetch 获取 docx 的 Markdown 文本。
// 官方协议：POST /open-apis/docs_ai/v1/documents/{token}/fetch  body={"format":"markdown"}
// 响应取 data.document.content；不启用 extra_param。
func FetchDocxMarkdownContent(docToken, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}

	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", url.PathEscape(docToken))
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, map[string]any{"format": "markdown"}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("获取文档 Markdown 内容失败: %w", err)
	}
	if err := CheckAPIResponse("获取文档 Markdown 内容", resp); err != nil {
		return "", err
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析文档 Markdown 响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("获取文档 Markdown 内容失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return parseDocsAIMarkdownContent(apiResp.Data)
}

func parseDocsAIMarkdownContent(data json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(data)) == 0 || string(bytes.TrimSpace(data)) == "null" {
		return "", fmt.Errorf("docs_ai fetch 响应缺少 document 对象")
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("解析文档 Markdown 响应失败: %w", err)
	}
	doc, ok := payload["document"].(map[string]any)
	if !ok || doc == nil {
		return "", fmt.Errorf("docs_ai fetch 响应缺少 document 对象")
	}
	content, ok := doc["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("docs_ai fetch 响应缺少 document.content")
	}
	return content, nil
}

// WaitDriveExportWithBound 有界轮询导出任务
// 返回: (status, timedOut, err)
// - status.Ready() == true 时成功
// - timedOut == true 表示超出轮询窗口仍未完成（调用方决定是否 resume）
// - err != nil 表示终态失败
func WaitDriveExportWithBound(ticket, docToken, userAccessToken string) (*DriveExportStatus, bool, error) {
	var last *DriveExportStatus
	var lastErr error
	for attempt := 1; attempt <= DriveExportMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := pollSleep(DriveExportPollInterval); err != nil {
				return last, false, err
			}
		}

		status, err := GetDriveExportStatus(ticket, docToken, userAccessToken)
		if err != nil {
			// 限流立即停止（继续轮询只会加剧限流）；其他查询错误视为瞬时，继续轮询
			if IsRateLimitError(err) {
				return last, false, &DrivePollError{Err: err, RateLimited: true}
			}
			lastErr = err
			continue
		}
		last = status

		if status.Ready() {
			return status, false, nil
		}
		if status.Failed() {
			msg := status.JobErrorMsg
			if msg == "" {
				msg = status.StatusLabel()
			}
			return status, false, fmt.Errorf("导出任务失败: %s (ticket=%s)", msg, ticket)
		}
	}
	if last == nil && lastErr != nil {
		return nil, false, &DrivePollError{Err: lastErr, AllFailed: true}
	}
	return last, true, nil
}

// DriveImportStatus 导入任务状态
type DriveImportStatus struct {
	Ticket      string `json:"ticket"`
	JobStatus   int    `json:"job_status"`
	JobErrorMsg string `json:"job_error_msg"`
	DocToken    string `json:"doc_token"`
	DocURL      string `json:"doc_url"`
	Type        string `json:"type"`
}

// Ready 任务已完成
func (s *DriveImportStatus) Ready() bool {
	return s != nil && s.JobStatus == 0 && s.DocToken != ""
}

// Pending 任务进行中
func (s *DriveImportStatus) Pending() bool {
	if s == nil {
		return false
	}
	return s.JobStatus == 1 || s.JobStatus == 2 || (s.JobStatus == 0 && s.DocToken == "")
}

// Failed 任务失败
func (s *DriveImportStatus) Failed() bool {
	return s != nil && !s.Ready() && !s.Pending() && s.JobStatus != 0
}

// GetDriveImportStatus 查询导入任务状态（归一化）
func GetDriveImportStatus(ticket, userAccessToken string) (*DriveImportStatus, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetImportTaskReqBuilder().
		Ticket(ticket).
		Build()

	resp, err := client.Drive.ImportTask.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("查询导入任务失败: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("查询导入任务失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.Result == nil {
		return &DriveImportStatus{Ticket: ticket}, nil
	}
	r := resp.Data.Result
	return &DriveImportStatus{
		Ticket:      ticket,
		JobStatus:   IntVal(r.JobStatus),
		JobErrorMsg: StringVal(r.JobErrorMsg),
		DocToken:    StringVal(r.Token),
		DocURL:      StringVal(r.Url),
		Type:        StringVal(r.Type),
	}, nil
}

// WaitDriveImportWithBound 有界轮询导入任务
func WaitDriveImportWithBound(ticket, userAccessToken string) (*DriveImportStatus, bool, error) {
	var last *DriveImportStatus
	var lastErr error
	for attempt := 1; attempt <= DriveImportMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := pollSleep(DriveImportPollInterval); err != nil {
				return last, false, err
			}
		}

		status, err := GetDriveImportStatus(ticket, userAccessToken)
		if err != nil {
			if IsRateLimitError(err) {
				return last, false, &DrivePollError{Err: err, RateLimited: true}
			}
			lastErr = err
			continue
		}
		last = status

		if status.Ready() {
			return status, false, nil
		}
		if status.Failed() {
			msg := status.JobErrorMsg
			if msg == "" {
				msg = fmt.Sprintf("job_status=%d", status.JobStatus)
			}
			return status, false, fmt.Errorf("导入任务失败: %s (ticket=%s)", msg, ticket)
		}
	}
	if last == nil && lastErr != nil {
		return nil, false, &DrivePollError{Err: lastErr, AllFailed: true}
	}
	return last, true, nil
}

// ==================== 通用异步任务（file move 用） ====================

// DriveTaskCheckStatus 通用异步任务状态（/drive/v1/files/task_check，文件夹移动/删除共用）
type DriveTaskCheckStatus struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"` // success / failed / fail / process 等
}

// Ready 任务成功完成。
func (s *DriveTaskCheckStatus) Ready() bool {
	return s != nil && strings.EqualFold(strings.TrimSpace(s.Status), "success")
}

// Failed 任务失败。task_check 被多个异步流程复用：有的后端返回 "failed"，删除任务返回更短的 "fail"，两者都是失败终态。
func (s *DriveTaskCheckStatus) Failed() bool {
	if s == nil {
		return false
	}
	st := strings.TrimSpace(s.Status)
	return strings.EqualFold(st, "failed") || strings.EqualFold(st, "fail")
}

// Pending 任务尚未结束。
func (s *DriveTaskCheckStatus) Pending() bool {
	return s != nil && !s.Ready() && !s.Failed()
}

// DrivePollError 表示异步任务已创建、但轮询未能拿到终态：
//   - RateLimited：查询被限流，已立即停止轮询（继续只会加剧限流）；
//   - AllFailed：轮询窗口内每次查询都失败（瞬时错误已忽略重试）。
//
// 调用方应提示用户稍后用 drive task-result 续查，而不是重新创建任务。
type DrivePollError struct {
	Err         error
	RateLimited bool
	AllFailed   bool
}

func (e *DrivePollError) Error() string {
	switch {
	case e.RateLimited:
		return fmt.Sprintf("查询异步任务状态被限流，已停止轮询（任务已创建，请稍后续查）: %v", e.Err)
	case e.AllFailed:
		return fmt.Sprintf("异步任务已创建，但每次状态查询都失败: %v", e.Err)
	}
	return e.Err.Error()
}

func (e *DrivePollError) Unwrap() error { return e.Err }

// pollSleep 在两次轮询之间等待；Ctrl-C 时立即返回。
func pollSleep(d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-runctx.Root().Done():
		return fmt.Errorf("轮询已取消: %w", runctx.Root().Err())
	case <-timer.C:
		return nil
	}
}

func driveTaskCheckPath(taskID string) string {
	query := url.Values{}
	query.Set("task_id", taskID)
	return "/open-apis/drive/v1/files/task_check?" + query.Encode()
}

// GetDriveTaskCheck 查询通用异步任务状态
// API: GET /open-apis/drive/v1/files/task_check?task_id=xxx
func GetDriveTaskCheck(taskID, userAccessToken string) (*DriveTaskCheckStatus, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	apiPath := driveTaskCheckPath(taskID)
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询任务状态失败: %w", err)
	}
	if err := CheckAPIResponse("查询任务状态", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data struct {
			Status string `json:"status"`
			Result *struct {
				Status string `json:"status"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析任务状态失败: %w", err)
	}
	status := apiResp.Data.Status
	if status == "" && apiResp.Data.Result != nil {
		status = apiResp.Data.Result.Status
	}
	return &DriveTaskCheckStatus{TaskID: taskID, Status: status}, nil
}

// WaitDriveTaskCheckWithBound 有界轮询通用任务（用于 folder move/delete 等）。
// 瞬时查询错误忽略并继续轮询；限流立即停止；成功/失败（failed 或 fail）为终态。
func WaitDriveTaskCheckWithBound(taskID, userAccessToken string) (*DriveTaskCheckStatus, bool, error) {
	var last *DriveTaskCheckStatus
	var lastErr error
	for attempt := 1; attempt <= DriveMoveMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := pollSleep(DriveMovePollInterval); err != nil {
				return last, false, err
			}
		}

		status, err := GetDriveTaskCheck(taskID, userAccessToken)
		if err != nil {
			if IsRateLimitError(err) {
				return last, false, &DrivePollError{Err: err, RateLimited: true}
			}
			lastErr = err
			continue
		}
		last = status

		if status.Ready() {
			return status, false, nil
		}
		if status.Failed() {
			return status, false, fmt.Errorf("任务失败 (task_id=%s, status=%s)", taskID, status.Status)
		}
	}
	if last == nil && lastErr != nil {
		return nil, false, &DrivePollError{Err: lastErr, AllFailed: true}
	}
	return last, true, nil
}
