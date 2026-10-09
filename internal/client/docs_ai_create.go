package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

// docs_ai 服务端建文档（POST /open-apis/docs_ai/v1/documents，对齐官方 docs +create 的异步协议）：
//   - 请求带 extra_param={"open_create_async":true}；小文档直接返回 data.document，
//     大文档返回 data.task，需要轮询 GET /open-apis/docs_ai/v1/async_tasks/{task_id}；
//   - 轮询中空 status 视为 processing；只重试 GET，绝不重放 POST（实测客户端超时后服务端仍会建出文档）；
//   - expired / execution_interrupted 映射为超时，提示分批创建；
//   - succeeded 时 result.create_document 是 JSON 字符串，解码后即同步返回形状的 data。

const (
	docsAICreateAsyncExtraParam     = `{"open_create_async":true}`
	docsAICreateRequestTimeout      = 10 * time.Minute
	docsAICreateMaxWait             = 10 * time.Minute
	docsAICreateDefaultPollInterval = 3 * time.Second
	docsAICreateMinPollInterval     = 100 * time.Millisecond
	docsAICreateMaxPollInterval     = 10 * time.Second
	docsAICreateBatchHint           = "请先用较少内容创建文档（doc create --content 部分内容），再用 `feishu-cli doc content-update <document_id> --mode append` 分批追加剩余内容"
)

// docsAICreateSleep 可在测试中替换。
var docsAICreateSleep = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// docsAICreateMaxWaitOverride 测试用：>0 时替换最长等待时间。
var docsAICreateMaxWaitOverride time.Duration

type docsAIAsyncTask struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Stage       string `json:"stage"`
	PollAfterMS int    `json:"poll_after_ms"`
	Result      *struct {
		CreateDocument string `json:"create_document"`
	} `json:"result"`
	Failure *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"failure"`
}

// CreateDocsAIDocument 用 docs_ai 创建文档；body 至少含 format 与 content，函数会补 extra_param。
// 返回与同步创建相同形状的 data（含 document.document_id / url / revision_id），以及 log_id。
// data.result 为 failed / partial_success 时同时返回 *DocsAIResultError。
func CreateDocsAIDocument(body map[string]any, userAccessToken string) (map[string]any, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	if _, ok := body["extra_param"]; !ok {
		body["extra_param"] = docsAICreateAsyncExtraParam
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := c.Post(ContextWithTimeout(docsAICreateRequestTimeout), "/open-apis/docs_ai/v1/documents", body, tokenType, opts...)
	if err != nil {
		// POST 不可重放：请求可能已在服务端生效，提示用户核对而不是自动重试
		return nil, fmt.Errorf("创建文档请求失败（请求可能已在服务端生效，请先在云空间确认是否已生成文档，避免重复创建）: %w", err)
	}
	data, logID, err := decodeDocsAIData("创建文档", resp.StatusCode, resp.Header, resp.RawBody)
	if err != nil {
		return nil, err
	}
	if rerr := classifyDocsAIResult("创建文档", data, logID); rerr != nil {
		return data, rerr
	}
	task, err := decodeDocsAITask(data)
	if err != nil {
		return nil, err
	}
	if task != nil {
		maxWait := docsAICreateMaxWait
		if docsAICreateMaxWaitOverride > 0 {
			maxWait = docsAICreateMaxWaitOverride
		}
		ctx, cancel := context.WithTimeout(runctx.Root(), maxWait)
		defer cancel()
		data, logID, err = pollDocsAICreateTask(ctx, task, logID, userAccessToken)
		if err != nil {
			return nil, err
		}
		if rerr := classifyDocsAIResult("创建文档", data, logID); rerr != nil {
			return data, rerr
		}
	}
	if logID != "" {
		data["log_id"] = logID
	}
	return data, nil
}

func decodeDocsAITask(data map[string]any) (*docsAIAsyncTask, error) {
	raw, ok := data["task"]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("解析创建任务失败: %w", err)
	}
	var task docsAIAsyncTask
	if err := json.Unmarshal(b, &task); err != nil {
		return nil, fmt.Errorf("解析创建任务失败: %w", err)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, fmt.Errorf("创建文档返回了异步任务但缺少 task_id")
	}
	return &task, nil
}

func docsAIPollInterval(ms int) time.Duration {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d <= 0:
		return docsAICreateDefaultPollInterval
	case d < docsAICreateMinPollInterval:
		return docsAICreateMinPollInterval
	case d > docsAICreateMaxPollInterval:
		return docsAICreateMaxPollInterval
	}
	return d
}

// pollDocsAICreateTask 轮询异步建文档任务。只重试 GET；第一次读取不等待（服务端会挂起等待）。
func pollDocsAICreateTask(ctx context.Context, task *docsAIAsyncTask, logID, userAccessToken string) (map[string]any, string, error) {
	taskID := strings.TrimSpace(task.TaskID)
	var delay time.Duration
	for {
		if err := ctx.Err(); err != nil {
			return nil, logID, docsAIWaitError(err, taskID, logID)
		}
		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "succeeded":
			if task.Result == nil || strings.TrimSpace(task.Result.CreateDocument) == "" {
				return nil, logID, fmt.Errorf("异步建文档成功但缺少 create_document 结果（task_id=%s, log_id=%s）", taskID, logID)
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(task.Result.CreateDocument), &data); err != nil || data == nil {
				return nil, logID, fmt.Errorf("异步建文档结果不是合法 JSON（task_id=%s）: %v", taskID, err)
			}
			return data, logID, nil
		case "failed", "expired":
			return nil, logID, docsAITaskFailure(task, logID)
		case "", "processing":
			// 空状态按处理中继续，避免把稀疏响应误判为失败后重复创建
		default:
			return nil, logID, fmt.Errorf("异步建文档返回未知状态 %q（task_id=%s, log_id=%s）", task.Status, taskID, logID)
		}

		if err := docsAICreateSleep(ctx, delay); err != nil {
			return nil, logID, docsAIWaitError(err, taskID, logID)
		}
		polled, polledLogID, err := getDocsAIAsyncTask(ctx, taskID, userAccessToken)
		if polledLogID != "" {
			logID = polledLogID
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, logID, docsAIWaitError(ctx.Err(), taskID, logID)
			}
			if !IsRetryableError(err) && !isTransportError(err) {
				return nil, logID, fmt.Errorf("查询建文档任务失败（task_id=%s）: %w", taskID, err)
			}
			// 只重试只读 GET：失败后退避，再读一次
			delay = min(max(min(delay, docsAICreateMaxPollInterval)*2, docsAICreateDefaultPollInterval), docsAICreateMaxPollInterval)
			continue
		}
		next, err := decodeDocsAITask(polled)
		if err != nil {
			return nil, logID, err
		}
		if next == nil {
			return nil, logID, fmt.Errorf("查询建文档任务的响应缺少 task（task_id=%s）", taskID)
		}
		if next.TaskID != "" && next.TaskID != taskID {
			return nil, logID, fmt.Errorf("查询建文档任务返回了不一致的 task_id（期望 %s，实际 %s）", taskID, next.TaskID)
		}
		task = next
		delay = docsAIPollInterval(task.PollAfterMS)
	}
}

func getDocsAIAsyncTask(ctx context.Context, taskID, userAccessToken string) (map[string]any, string, error) {
	c, err := GetClient()
	if err != nil {
		return nil, "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	resp, err := c.Get(reqCtx, "/open-apis/docs_ai/v1/async_tasks/"+url.PathEscape(taskID), nil, tokenType, opts...)
	if err != nil {
		return nil, "", err
	}
	return decodeDocsAIData("查询建文档任务", resp.StatusCode, resp.Header, resp.RawBody)
}

// isTransportError 判断是否为连接/超时类传输错误（只读 GET 可安全重试）。
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"timeout", "connection reset", "connection refused", "eof", "broken pipe", "no such host", "tls handshake"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func docsAIWaitError(err error, taskID, logID string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return clierr.Network(fmt.Errorf("文档处理时间过长，已停止等待（task_id=%s, log_id=%s）；文档可能仍在后台生成，请稍后在云空间确认，避免重复创建。%s", taskID, logID, docsAICreateBatchHint))
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("已取消等待建文档任务（task_id=%s）；文档可能仍在后台生成", taskID)
	}
	return err
}

func docsAITaskFailure(task *docsAIAsyncTask, logID string) error {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	code, msg := "", status
	if task.Failure != nil {
		code = strings.TrimSpace(task.Failure.Code)
		if m := strings.TrimSpace(task.Failure.Message); m != "" {
			msg = m
		}
	}
	if status == "expired" || code == "execution_interrupted" {
		return clierr.Network(fmt.Errorf("文档处理时间过长（status=%s, code=%s, log_id=%s）。%s", status, code, logID, docsAICreateBatchHint))
	}
	if code != "" {
		msg += "（code: " + code + "）"
	}
	return fmt.Errorf("创建文档失败: %s（task_id=%s, log_id=%s）", msg, task.TaskID, logID)
}
