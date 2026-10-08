package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/runctx"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// 下载传输参数（对齐官方 extension/download：8MiB 分片、每片有界重试、空闲超时而非总时长）。
// 均为包级变量，便于测试缩小。
var (
	// rangeDownloadChunkSize 单个 Range 分片请求的字节数。
	rangeDownloadChunkSize int64 = 8 * 1024 * 1024
	// downloadPartRetries 单个分片（或整包流的同一位置）允许的重试次数。
	downloadPartRetries = 3
	// downloadIdleTimeout 等待响应头或单次读取的最长空闲时间；只要还在持续收到数据就不会超时。
	downloadIdleTimeout = 60 * time.Second
	// downloadRetryBaseDelay 分片重试的指数退避基数。
	downloadRetryBaseDelay = 200 * time.Millisecond
)

const maxDownloadAPIErrorProbeBytes int64 = 1 << 20

func newBearerDownloadRequest(reqURL, bearerToken, byteRange string) (*http.Request, error) {
	return newBearerDownloadRequestWithContext(runctx.Root(), reqURL, bearerToken, byteRange)
}

func newBearerDownloadRequestWithContext(ctx context.Context, reqURL, bearerToken, byteRange string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	return req, nil
}

type downloadAPIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func inspectDownloadAPIErrorResponse(httpResp *http.Response) (io.Reader, *downloadAPIError, error) {
	bufferedBody := bufio.NewReader(httpResp.Body)
	if !downloadResponseMayContainAPIError(httpResp, bufferedBody) {
		return bufferedBody, nil, nil
	}

	var probe bytes.Buffer
	probeReader := io.TeeReader(io.LimitReader(bufferedBody, maxDownloadAPIErrorProbeBytes+1), &probe)
	body, err := io.ReadAll(probeReader)
	replayReader := io.MultiReader(bytes.NewReader(probe.Bytes()), bufferedBody)
	if err != nil {
		return replayReader, nil, err
	}
	if int64(len(body)) > maxDownloadAPIErrorProbeBytes {
		return replayReader, nil, nil
	}

	var apiErr downloadAPIError
	if err := json.Unmarshal(bytes.TrimSpace(body), &apiErr); err == nil && apiErr.Code != 0 {
		return nil, &apiErr, nil
	}
	return replayReader, nil, nil
}

func downloadResponseMayContainAPIError(httpResp *http.Response, bufferedBody *bufio.Reader) bool {
	if strings.TrimSpace(httpResp.Header.Get("Content-Disposition")) != "" {
		return false
	}
	if isJSONContentType(httpResp.Header.Get("Content-Type")) {
		return true
	}

	firstByte, ok := peekFirstNonSpaceByte(bufferedBody)
	return ok && firstByte == '{'
}

func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func peekFirstNonSpaceByte(bufferedBody *bufio.Reader) (byte, bool) {
	const maxProbe = 512
	for size := 1; size <= maxProbe; size++ {
		buf, err := bufferedBody.Peek(size)
		if len(buf) == 0 {
			return 0, false
		}
		for _, b := range buf {
			if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
				return b, true
			}
		}
		if err != nil {
			return 0, false
		}
	}
	return 0, false
}

func parseDownloadAPIError(action string, httpResp *http.Response) (*downloadAPIError, error) {
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s失败: 读取响应失败: %w", action, err)
	}

	var apiErr downloadAPIError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Code != 0 {
		return &apiErr, nil
	}
	return nil, fmt.Errorf("%s失败: HTTP %d, body: %s", action, httpResp.StatusCode, strings.TrimSpace(string(body)))
}

// downloadBearerURLByRange 以 Range 分片方式下载 reqURL 到 outputPath（已知整包下载会触发大小限制时使用）。
//
// timeout > 0 时作为整个下载的总时长上限（调用方显式要求）；无论是否设置，单次等待都受
// downloadIdleTimeout 空闲超时约束，单个分片失败按 downloadPartRetries 有界重试并从断点续传。
// 写盘走同目录临时文件 + rename：失败、超时或 Ctrl-C 时不会留下半截文件，也不会删除/破坏已存在的同名文件。
func downloadBearerURLByRange(action, reqURL, outputPath, bearerToken string, timeout time.Duration) error {
	if rangeDownloadChunkSize <= 0 {
		return fmt.Errorf("%s失败: Range 分片大小非法: %d", action, rangeDownloadChunkSize)
	}
	_, _, err := downloadStreamToFile(downloadStreamSpec{
		Action:      action,
		URL:         reqURL,
		Bearer:      bearerToken,
		StartRanged: true,
	}, outputPath, timeout)
	return err
}

// downloadStreamSpec 描述一次流式下载。
type downloadStreamSpec struct {
	Action string // 中文动作，如 "下载文件"
	URL    string
	Bearer string // 为空时不带 Authorization（预签名 URL）
	// HTTPClient 为空时使用 rawHTTPClient()（官方 host 白名单 + 重定向剥离凭证）。
	// 预签名 URL 指向非 Open API 主机，需由调用方传入不做白名单的客户端。
	HTTPClient *http.Client
	// StartRanged=true 直接按 Range 分片下载；false 先整包 GET，遇"文件超出下载大小限制"再切 Range。
	StartRanged bool
	// MaxBytes > 0 时超过该大小立即失败（不落盘）。
	MaxBytes int64
}

// downloadStreamToFile 打开下载流并原子写入 outputPath，返回写入字节数与首个成功响应的 Header。
func downloadStreamToFile(spec downloadStreamSpec, outputPath string, timeout time.Duration) (int64, http.Header, error) {
	ctx, cancel := downloadContext(timeout)
	defer cancel()
	stream, err := openDownloadStream(ctx, spec)
	if err != nil {
		return 0, nil, err
	}
	defer stream.Close()
	n, err := safefile.AtomicWriteFrom(outputPath, stream, 0o644)
	if err != nil {
		if streamErr := stream.Err(); streamErr != nil {
			return n, nil, streamErr
		}
		return n, nil, fmt.Errorf("保存文件失败: %w", err)
	}
	return n, stream.Header(), nil
}

// downloadContext 从进程根 context（已接 Ctrl-C）派生；timeout > 0 时附加总时长上限。
func downloadContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(runctx.Root(), timeout)
	}
	return context.WithCancel(runctx.Root())
}

// downloadAttemptError 标记一次请求失败是否可重试。
type downloadAttemptError struct {
	err       error
	retryable bool
}

func (e *downloadAttemptError) Error() string { return e.err.Error() }
func (e *downloadAttemptError) Unwrap() error { return e.err }

func retryableDownloadErr(err error) error { return &downloadAttemptError{err: err, retryable: true} }
func nonRetryableDownloadErr(err error) error {
	return &downloadAttemptError{err: err, retryable: false}
}

// idleWatchdog 在 idle 时间内没有任何进展（响应头或数据）时取消当前请求。
type idleWatchdog struct {
	timer *time.Timer
	idle  time.Duration
	fired atomic.Bool
}

func startIdleWatchdog(idle time.Duration, cancel context.CancelFunc) *idleWatchdog {
	w := &idleWatchdog{idle: idle}
	if idle > 0 {
		w.timer = time.AfterFunc(idle, func() {
			w.fired.Store(true)
			cancel()
		})
	}
	return w
}

func (w *idleWatchdog) kick() {
	if w != nil && w.timer != nil {
		w.timer.Reset(w.idle)
	}
}

func (w *idleWatchdog) stop() {
	if w != nil && w.timer != nil {
		w.timer.Stop()
	}
}

func (w *idleWatchdog) timedOut() bool { return w != nil && w.fired.Load() }

// downloadStream 是一个"逻辑上连续"的下载流：
//   - 整包模式：一次 GET 200；中途断流时用 Range 从已交付位置续传；
//   - 分片模式：按 rangeDownloadChunkSize 逐片 GET 206，校验 Content-Range 起点/总大小；
//   - 每个分片（续传位置）最多重试 downloadPartRetries 次（网络错误、空闲超时、5xx、限流）；
//   - 4xx 业务错误、协议错误、用户取消（Ctrl-C）或总时长超时不重试。
type downloadStream struct {
	ctx    context.Context
	spec   downloadStreamSpec
	client *http.Client

	body     io.ReadCloser
	cancel   context.CancelFunc
	watchdog *idleWatchdog

	ranged   bool  // 当前是否处于 Range 分片模式
	offset   int64 // 已交付给调用方的字节数
	total    int64 // 文件总大小；-1 表示未知
	partEnd  int64 // 分片模式下当前响应的最后一个字节（含）
	attempts int   // 当前位置已重试次数
	// sinceRetry 自上次重试以来新收到的字节数；整包模式下累计满一个分片大小即恢复重试额度
	sinceRetry int64
	header     http.Header
	done       bool
	err        error // 终态错误（sticky）
}

// openDownloadStream 发出首个请求并校验响应；返回的流读到 EOF 即表示完整、长度已校验。
func openDownloadStream(ctx context.Context, spec downloadStreamSpec) (*downloadStream, error) {
	if spec.Action == "" {
		spec.Action = "下载文件"
	}
	if spec.StartRanged && rangeDownloadChunkSize <= 0 {
		return nil, fmt.Errorf("%s失败: Range 分片大小非法: %d", spec.Action, rangeDownloadChunkSize)
	}
	hc := spec.HTTPClient
	if hc == nil {
		hc = rawHTTPClient()
	}
	s := &downloadStream{ctx: ctx, spec: spec, client: hc, ranged: spec.StartRanged, total: -1}
	if err := s.open(); err != nil {
		s.closeBody()
		return nil, err
	}
	return s, nil
}

// Header 返回首个成功响应的 Header（含 Content-Disposition 等）。
func (s *downloadStream) Header() http.Header { return s.header }

// Size 返回已知的文件总大小，未知时为 -1。
func (s *downloadStream) Size() int64 { return s.total }

// Err 返回流的终态错误（读取失败时比写盘层包装的错误更准确）。
func (s *downloadStream) Err() error { return s.err }

func (s *downloadStream) Close() error {
	s.closeBody()
	return nil
}

func (s *downloadStream) closeBody() {
	if s.watchdog != nil {
		s.watchdog.stop()
		s.watchdog = nil
	}
	if s.body != nil {
		_ = s.body.Close()
		s.body = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *downloadStream) Read(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.done {
		return 0, io.EOF
	}
	for {
		if s.body == nil {
			if s.total >= 0 && s.offset >= s.total {
				s.done = true
				return 0, io.EOF
			}
			if err := s.open(); err != nil {
				s.err = err
				return 0, err
			}
		}

		n, rerr := s.body.Read(p)
		if n > 0 {
			s.watchdog.kick()
			s.offset += int64(n)
			s.sinceRetry += int64(n)
			if s.attempts > 0 && s.sinceRetry >= rangeDownloadChunkSize {
				s.attempts = 0
			}
			if s.ranged && s.offset > s.partEnd+1 {
				s.closeBody()
				s.err = fmt.Errorf("%s失败: Range 响应数据超出声明范围（至 %d）", s.spec.Action, s.partEnd)
				return 0, s.err
			}
			if s.spec.MaxBytes > 0 && s.offset > s.spec.MaxBytes {
				s.closeBody()
				s.err = fmt.Errorf("文件超过大小限制 (%d MB)", s.spec.MaxBytes/(1024*1024))
				return 0, s.err
			}
			if s.total >= 0 && s.offset > s.total {
				s.closeBody()
				s.err = fmt.Errorf("%s失败: 收到的数据超过声明大小 %d", s.spec.Action, s.total)
				return 0, s.err
			}
		}
		if rerr == nil {
			if n > 0 {
				return n, nil
			}
			continue
		}

		if errors.Is(rerr, io.EOF) {
			s.closeBody()
			if s.ranged {
				if s.offset != s.partEnd+1 {
					// 分片提前结束：视为瞬时错误，从断点续传
					if err := s.consumeRetry(fmt.Errorf("%s失败: 分片提前结束（收到 %d 字节，期望至 %d）", s.spec.Action, s.offset, s.partEnd+1)); err != nil {
						return n, err
					}
				} else {
					s.attempts = 0
					if s.offset >= s.total {
						s.done = true
						if n > 0 {
							return n, nil
						}
						return 0, io.EOF
					}
				}
			} else {
				if s.total >= 0 && s.offset < s.total {
					if err := s.consumeRetry(fmt.Errorf("%s失败: 响应提前结束（收到 %d/%d 字节）", s.spec.Action, s.offset, s.total)); err != nil {
						return n, err
					}
				} else {
					s.done = true
					if n > 0 {
						return n, nil
					}
					return 0, io.EOF
				}
			}
			if n > 0 {
				return n, nil
			}
			continue
		}

		// 读取中途出错（网络中断、空闲超时、取消）
		timedOut := s.watchdog.timedOut()
		s.closeBody()
		if ctxErr := s.ctx.Err(); ctxErr != nil {
			s.err = fmt.Errorf("%s失败: %w", s.spec.Action, ctxErr)
			return n, s.err
		}
		cause := rerr
		if timedOut {
			cause = fmt.Errorf("%s 内未收到数据（空闲超时）", downloadIdleTimeout)
		}
		if err := s.consumeRetry(fmt.Errorf("%s失败: 读取响应中断: %w", s.spec.Action, cause)); err != nil {
			return n, err
		}
		if n > 0 {
			return n, nil
		}
	}
}

// consumeRetry 消耗当前位置的一次重试额度并退避；额度耗尽时设置终态错误。
func (s *downloadStream) consumeRetry(cause error) error {
	if s.attempts >= downloadPartRetries {
		s.err = fmt.Errorf("%w（已重试 %d 次）", cause, s.attempts)
		return s.err
	}
	s.attempts++
	s.sinceRetry = 0
	if err := s.backoff(); err != nil {
		s.err = err
		return err
	}
	return nil
}

func (s *downloadStream) backoff() error {
	base := downloadRetryBaseDelay
	if base <= 0 {
		return nil
	}
	shift := s.attempts - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 4 {
		shift = 4
	}
	delay := base * time.Duration(1<<uint(shift))
	delay += time.Duration(rand.Int63n(int64(delay)/10 + 1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return fmt.Errorf("%s失败: %w", s.spec.Action, s.ctx.Err())
	case <-timer.C:
		return nil
	}
}

// open 在当前位置建立响应，可重试错误按额度重试。
func (s *downloadStream) open() error {
	for {
		err := s.openOnce()
		if err == nil {
			return nil
		}
		var ae *downloadAttemptError
		if errors.As(err, &ae) {
			if ae.retryable && s.ctx.Err() == nil && s.attempts < downloadPartRetries {
				s.attempts++
				s.sinceRetry = 0
				if berr := s.backoff(); berr != nil {
					return berr
				}
				continue
			}
			if ae.retryable && s.attempts > 0 {
				return fmt.Errorf("%w（已重试 %d 次）", ae.err, s.attempts)
			}
			return ae.err
		}
		return err
	}
}

// fetch 发出一次 GET；成功时返回响应及其专属 cancel/watchdog（由调用方挂到 stream 上）。
func (s *downloadStream) fetch(byteRange string) (*http.Response, context.CancelFunc, *idleWatchdog, error) {
	reqCtx, cancel := context.WithCancel(s.ctx)
	wd := startIdleWatchdog(downloadIdleTimeout, cancel)
	req, err := newBearerDownloadRequestWithContext(reqCtx, s.spec.URL, s.spec.Bearer, byteRange)
	if err != nil {
		wd.stop()
		cancel()
		return nil, nil, nil, nonRetryableDownloadErr(fmt.Errorf("%s失败: %w", s.spec.Action, err))
	}
	// 关闭传输压缩，保证字节偏移与 Content-Range 对应原始表示
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.client.Do(req)
	if err != nil {
		timedOut := wd.timedOut()
		wd.stop()
		cancel()
		if ctxErr := s.ctx.Err(); ctxErr != nil {
			return nil, nil, nil, nonRetryableDownloadErr(fmt.Errorf("%s失败: %w", s.spec.Action, ctxErr))
		}
		label := ""
		if byteRange != "" {
			label = " Range " + byteRange
		}
		if timedOut {
			return nil, nil, nil, retryableDownloadErr(fmt.Errorf("%s失败:%s 等待响应超过 %s（空闲超时）", s.spec.Action, label, downloadIdleTimeout))
		}
		return nil, nil, nil, retryableDownloadErr(fmt.Errorf("%s失败:%s %w", s.spec.Action, label, err))
	}
	wd.kick()
	return resp, cancel, wd, nil
}

func (s *downloadStream) attach(resp *http.Response, body io.Reader, cancel context.CancelFunc, wd *idleWatchdog) {
	s.body = &readCloser{Reader: body, Closer: resp.Body}
	s.cancel = cancel
	s.watchdog = wd
	if s.header == nil {
		s.header = resp.Header.Clone()
	}
}

type readCloser struct {
	io.Reader
	io.Closer
}

func (s *downloadStream) openOnce() error {
	if s.ranged {
		return s.openRange()
	}
	if s.offset > 0 {
		// 整包流中途断开：用 Range 从断点续传
		return s.openRange()
	}
	resp, cancel, wd, err := s.fetch("")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := s.responseError(resp)
		wd.stop()
		cancel()
		if s.isSizeLimit(apiErr) {
			s.ranged = true
			return s.openRange()
		}
		return apiErr
	}
	body, apiErr := s.inspect200(resp)
	if apiErr != nil {
		_ = resp.Body.Close()
		wd.stop()
		cancel()
		if s.isSizeLimit(apiErr) {
			s.ranged = true
			return s.openRange()
		}
		return apiErr
	}
	if s.spec.MaxBytes > 0 && resp.ContentLength > s.spec.MaxBytes {
		_ = resp.Body.Close()
		wd.stop()
		cancel()
		return nonRetryableDownloadErr(fmt.Errorf("文件超过大小限制: %d MB (限制 %d MB)", resp.ContentLength/(1024*1024), s.spec.MaxBytes/(1024*1024)))
	}
	s.total = -1
	if resp.ContentLength >= 0 && resp.Header.Get("Content-Length") != "" {
		s.total = resp.ContentLength
	}
	s.attach(resp, body, cancel, wd)
	return nil
}

func (s *downloadStream) openRange() error {
	start := s.offset
	end := start + rangeDownloadChunkSize - 1
	byteRange := fmt.Sprintf("bytes=%d-%d", start, end)
	resp, cancel, wd, err := s.fetch(byteRange)
	if err != nil {
		return err
	}
	release := func() {
		_ = resp.Body.Close()
		wd.stop()
		cancel()
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		rangeStart, rangeEnd, rangeTotal, perr := parseContentRange(resp.Header.Get("Content-Range"))
		if perr != nil {
			release()
			return nonRetryableDownloadErr(fmt.Errorf("%s失败: 解析 Content-Range 失败: %w", s.spec.Action, perr))
		}
		if rangeStart != start {
			release()
			return nonRetryableDownloadErr(fmt.Errorf("%s失败: Range 响应起点不匹配: got %d, want %d", s.spec.Action, rangeStart, start))
		}
		if rangeEnd > end {
			release()
			return nonRetryableDownloadErr(fmt.Errorf("%s失败: Range 响应 %d-%d 超出请求范围 %s", s.spec.Action, rangeStart, rangeEnd, byteRange))
		}
		if s.total >= 0 && s.total != rangeTotal {
			release()
			return nonRetryableDownloadErr(fmt.Errorf("%s失败: 文件大小变化: got %d, want %d", s.spec.Action, rangeTotal, s.total))
		}
		if s.spec.MaxBytes > 0 && rangeTotal > s.spec.MaxBytes {
			release()
			return nonRetryableDownloadErr(fmt.Errorf("文件超过大小限制: %d MB (限制 %d MB)", rangeTotal/(1024*1024), s.spec.MaxBytes/(1024*1024)))
		}
		s.total = rangeTotal
		s.partEnd = rangeEnd
		s.ranged = true
		s.attach(resp, resp.Body, cancel, wd)
		return nil
	case http.StatusOK:
		// 服务端忽略 Range 返回整包：跳过已交付的字节后按整包模式继续
		body, apiErr := s.inspect200(resp)
		if apiErr != nil {
			release()
			return apiErr
		}
		if start > 0 {
			if _, cerr := io.CopyN(io.Discard, body, start); cerr != nil {
				release()
				return retryableDownloadErr(fmt.Errorf("%s失败: 续传时跳过已下载数据失败: %w", s.spec.Action, cerr))
			}
		}
		s.ranged = false
		if resp.ContentLength >= 0 && resp.Header.Get("Content-Length") != "" {
			if s.total >= 0 && s.total != resp.ContentLength {
				release()
				return nonRetryableDownloadErr(fmt.Errorf("%s失败: 文件大小变化: got %d, want %d", s.spec.Action, resp.ContentLength, s.total))
			}
			s.total = resp.ContentLength
		}
		s.attach(resp, body, cancel, wd)
		return nil
	case http.StatusRequestedRangeNotSatisfiable, http.StatusBadRequest:
		apiErr := s.responseError(resp)
		wd.stop()
		cancel()
		_, isEnvelope := AsAPIError(apiErr)
		if start == 0 && !isEnvelope {
			// 首片 Range 被拒（例如空文件）：退回整包 GET
			s.ranged = false
			resp2, cancel2, wd2, ferr := s.fetch("")
			if ferr != nil {
				return ferr
			}
			if resp2.StatusCode != http.StatusOK {
				e := s.responseError(resp2)
				wd2.stop()
				cancel2()
				return e
			}
			body, apiErr2 := s.inspect200(resp2)
			if apiErr2 != nil {
				_ = resp2.Body.Close()
				wd2.stop()
				cancel2()
				return apiErr2
			}
			s.total = -1
			if resp2.ContentLength >= 0 && resp2.Header.Get("Content-Length") != "" {
				s.total = resp2.ContentLength
			}
			s.attach(resp2, body, cancel2, wd2)
			return nil
		}
		return apiErr
	default:
		apiErr := s.responseError(resp)
		wd.stop()
		cancel()
		return apiErr
	}
}

// inspect200 检查 HTTP 200 响应体是否其实是飞书业务错误信封（权限不足等常以 200 + JSON 下发）。
func (s *downloadStream) inspect200(resp *http.Response) (io.Reader, error) {
	buffered := bufio.NewReader(resp.Body)
	if !downloadResponseMayContainAPIError(resp, buffered) {
		return buffered, nil
	}
	var probe bytes.Buffer
	body, err := io.ReadAll(io.TeeReader(io.LimitReader(buffered, maxDownloadAPIErrorProbeBytes+1), &probe))
	replay := io.MultiReader(bytes.NewReader(probe.Bytes()), buffered)
	if err != nil {
		return nil, retryableDownloadErr(fmt.Errorf("%s失败: 读取响应失败: %w", s.spec.Action, err))
	}
	if int64(len(body)) > maxDownloadAPIErrorProbeBytes {
		return replay, nil
	}
	if info, ok := apidiag.Parse(resp.StatusCode, resp.Header, body); ok {
		return nil, classifyDownloadAPIError(&APIError{Action: s.spec.Action, Info: info})
	}
	return replay, nil
}

// responseError 读取非 2xx 响应体（有上限）并转为可分类的错误；会关闭 resp.Body。
func (s *downloadStream) responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDownloadAPIErrorProbeBytes))
	_ = resp.Body.Close()
	err := ParseAPIResponse(s.spec.Action, resp.StatusCode, resp.Header, body)
	if err == nil {
		err = fmt.Errorf("%s失败: 非预期的 HTTP %d", s.spec.Action, resp.StatusCode)
	}
	if apiErr, ok := AsAPIError(err); ok {
		return classifyDownloadAPIError(apiErr)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return retryableDownloadErr(err)
	}
	return nonRetryableDownloadErr(err)
}

func classifyDownloadAPIError(apiErr *APIError) error {
	if apiErr.HTTPStatus == http.StatusTooManyRequests || apiErr.HTTPStatus >= 500 || apiErr.Code == 99991400 {
		return retryableDownloadErr(apiErr)
	}
	return nonRetryableDownloadErr(apiErr)
}

func (s *downloadStream) isSizeLimit(err error) bool {
	if s.spec.StartRanged {
		return false
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		return false
	}
	return isDownloadFileSizeLimitError(apiErr.Code, apiErr.Msg, nil)
}

func parseContentRange(contentRange string) (start, end, total int64, err error) {
	const prefix = "bytes "
	if !strings.HasPrefix(contentRange, prefix) {
		return 0, 0, 0, fmt.Errorf("缺少 bytes 前缀: %q", contentRange)
	}

	rangePart, totalPart, ok := strings.Cut(strings.TrimPrefix(contentRange, prefix), "/")
	if !ok || rangePart == "" || totalPart == "" || totalPart == "*" {
		return 0, 0, 0, fmt.Errorf("格式非法: %q", contentRange)
	}

	startPart, endPart, ok := strings.Cut(rangePart, "-")
	if !ok || startPart == "" || endPart == "" {
		return 0, 0, 0, fmt.Errorf("范围非法: %q", contentRange)
	}

	start, err = strconv.ParseInt(startPart, 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	end, err = strconv.ParseInt(endPart, 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	total, err = strconv.ParseInt(totalPart, 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	if start < 0 || end < start || total <= end {
		return 0, 0, 0, fmt.Errorf("范围越界: %q", contentRange)
	}
	return start, end, total, nil
}

// writeStreamToFile 把 reader 原子写入 outputPath（同目录临时文件 + rename）。
// 读取或写入失败时目标文件保持原样：既不留半截文件，也不会删除已存在的同名文件。
func writeStreamToFile(reader io.Reader, outputPath string) error {
	if _, err := safefile.AtomicWriteFrom(outputPath, reader, 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}
