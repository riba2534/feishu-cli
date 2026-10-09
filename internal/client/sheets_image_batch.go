package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/safefile"
)

const (
	DefaultSheetImageBatchWorkers  = 2
	DefaultSheetImageBatchMaxBytes = 20 << 20
	MaxSheetImageBatchItems        = 500
	maxSheetImageVerifyRanges      = 10
)

// BatchWriteSheetImageItem 批量图片单项配置
type BatchWriteSheetImageItem struct {
	Cell string `json:"cell"`           // 目标单格，如 "0b1212!B2:B2"
	URL  string `json:"url,omitempty"`  // 网络图片 URL（HTTPS）
	Path string `json:"path,omitempty"` // 本地图片文件路径
	Name string `json:"name,omitempty"` // 图片文件名（可选）
}

// BatchWriteSheetImageOutcome 单项处理结果
type BatchWriteSheetImageOutcome struct {
	Cell       string `json:"cell"`
	Source     string `json:"source"`
	Status     string `json:"status"` // "verified", "written", "failed", "skipped"
	ImageToken string `json:"image_token,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BatchWriteSheetImageResult 批处理结果汇总
type BatchWriteSheetImageResult struct {
	Total    int                           `json:"total"`
	Written  int                           `json:"written"`
	Failed   int                           `json:"failed"`
	Verified bool                          `json:"verified"`
	Outcomes []BatchWriteSheetImageOutcome `json:"outcomes"`
}

// BatchWriteSheetImageOptions 批处理选项
type BatchWriteSheetImageOptions struct {
	Workers         int
	MaxBytes        int64
	AllowPrivateNet bool
}

// BatchWriteSheetImages 批量写入单元格原生图片并通过 V3 回读校验
func BatchWriteSheetImages(ctx context.Context, spreadsheetToken, sheetID string, items []BatchWriteSheetImageItem, opts BatchWriteSheetImageOptions, userAccessToken ...string) (*BatchWriteSheetImageResult, error) {
	result := &BatchWriteSheetImageResult{
		Total:    len(items),
		Outcomes: make([]BatchWriteSheetImageOutcome, len(items)),
	}
	if len(items) == 0 {
		result.Verified = true
		return result, nil
	}
	if len(items) > MaxSheetImageBatchItems {
		return result, fmt.Errorf("批量图片数量超过上限 %d（当前 %d）", MaxSheetImageBatchItems, len(items))
	}

	workers := opts.Workers
	if workers < 1 {
		workers = DefaultSheetImageBatchWorkers
	} else if workers > 10 {
		workers = 10
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultSheetImageBatchMaxBytes
	}

	uat := firstString(userAccessToken)

	// 若环境中配置了代理（如 HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:7890），在 Control 钩子中精准豁免代理自身的 IP:Port
	proxyAllowedAddrs := make(map[string]struct{})
	for _, envKey := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		val := strings.TrimSpace(os.Getenv(envKey))
		if val == "" {
			continue
		}
		if !strings.Contains(val, "://") {
			val = "http://" + val
		}
		if pURL, err := url.Parse(val); err == nil && pURL.Host != "" {
			pHost := pURL.Hostname()
			if pHost == "" {
				continue
			}
			pPort := pURL.Port()
			if pPort == "" {
				switch strings.ToLower(pURL.Scheme) {
				case "https":
					pPort = "443"
				case "socks5", "socks5h":
					pPort = "1080"
				default:
					pPort = "80"
				}
			}
			if pip := net.ParseIP(pHost); pip != nil {
				proxyAllowedAddrs[net.JoinHostPort(pip.String(), pPort)] = struct{}{}
			} else {
				if pIPs, err := net.LookupIP(pHost); err == nil {
					for _, ip := range pIPs {
						proxyAllowedAddrs[net.JoinHostPort(ip.String(), pPort)] = struct{}{}
					}
				}
			}
		}
	}

	// 共享 HTTP Client，复用连接池与环境代理；通过 Dialer.Control 在底层建立 TCP 握手前拦截受限 IP（防 DNS Rebinding / TOCTOU）
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			// 精确比对代理服务器端点（IP:Port），仅放行代理监听端口
			if _, ok := proxyAllowedAddrs[address]; ok {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip != nil {
				if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return fmt.Errorf("禁止连接到本地回环或受限地址: %s", ip.String())
				}
				if !opts.AllowPrivateNet && isBlockedIP(ip) {
					return fmt.Errorf("禁止连接到私有网络地址: %s（如需使用内网图片源请设置 --allow-private-net）", ip.String())
				}
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		MaxIdleConns:        workers * 2,
		MaxIdleConnsPerHost: workers,
		IdleConnTimeout:     60 * time.Second,
	}
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("图片下载重定向次数超过 5")
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return errors.New("图片下载不允许从 HTTPS 降级到 HTTP")
			}
			return validateSheetImageURL(req.URL.String(), opts.AllowPrivateNet)
		},
	}
	defer transport.CloseIdleConnections()

	jobs := make(chan int)
	// 飞书要求同一文档串行写入；下载仍并发，每个 worker 最多保留一个待写临时文件。
	writeSlot := make(chan struct{}, 1)
	var wg sync.WaitGroup
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				result.Outcomes[index] = writeSingleSheetImage(ctx, httpClient, spreadsheetToken, items[index], maxBytes, opts.AllowPrivateNet, uat, writeSlot)
			}
		}()
	}

	for i := range items {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			for j := i; j < len(items); j++ {
				if result.Outcomes[j].Status == "" {
					result.Outcomes[j].Cell = items[j].Cell
					result.Outcomes[j].Status = "skipped"
					result.Outcomes[j].Error = "任务被中断，跳过处理"
				}
			}
			tallyBatchResult(result)
			return result, fmt.Errorf("批量写入被中断: %w", ctx.Err())
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		tallyBatchResult(result)
		return result, fmt.Errorf("批量写入被中断: %w", err)
	}

	// 回读验证阶段
	verifyIndexes := make([]int, 0, len(items))
	for i := range result.Outcomes {
		if result.Outcomes[i].Status == "written" {
			verifyIndexes = append(verifyIndexes, i)
		}
	}

	for start := 0; start < len(verifyIndexes); start += maxSheetImageVerifyRanges {
		if err := ctx.Err(); err != nil {
			tallyBatchResult(result)
			return result, fmt.Errorf("回读验证被中断: %w", err)
		}
		end := start + maxSheetImageVerifyRanges
		if end > len(verifyIndexes) {
			end = len(verifyIndexes)
		}
		indexes := verifyIndexes[start:end]
		ranges := make([]string, len(indexes))
		for i, index := range indexes {
			ranges[i] = items[index].Cell
		}

		var readRanges []*CellRangeV3
		var readErr error
	readLoop:
		for rAttempt := 0; rAttempt < 2; rAttempt++ {
			readCtx, rCancel := context.WithTimeout(ctx, 30*time.Second)
			readRanges, readErr = ReadCellsRichV3(readCtx, spreadsheetToken, sheetID, ranges, "", "", "", uat)
			rCancel()
			if readErr == nil {
				break
			}
			select {
			case <-ctx.Done():
				readErr = ctx.Err()
				break readLoop
			case <-time.After(500 * time.Millisecond):
			}
		}

		if readErr != nil {
			for _, index := range indexes {
				result.Outcomes[index].Status = "failed"
				result.Outcomes[index].Error = fmt.Sprintf("写入成功但回读调用失败: %v", readErr)
			}
			continue
		}

		verifiedMap := verifiedSheetImagesByCell(readRanges)
		for _, index := range indexes {
			coord := extractCellCoordinate(items[index].Cell)
			token := verifiedMap[coord]
			if token == "" {
				// 若第一次未读到 token，进行一次微小退避单格重查（防服务端主从复制延迟）
				var secondErr error
				select {
				case <-ctx.Done():
					result.Outcomes[index].Status = "failed"
					result.Outcomes[index].Error = fmt.Sprintf("写入成功但回读被中断: %v", ctx.Err())
					continue
				case <-time.After(500 * time.Millisecond):
					readCtx, rCancel := context.WithTimeout(ctx, 10*time.Second)
					var secondRanges []*CellRangeV3
					secondRanges, secondErr = ReadCellsRichV3(readCtx, spreadsheetToken, sheetID, []string{items[index].Cell}, "", "", "", uat)
					rCancel()
					secondMap := verifiedSheetImagesByCell(secondRanges)
					token = secondMap[coord]
				}
				if token == "" {
					result.Outcomes[index].Status = "failed"
					if secondErr != nil {
						result.Outcomes[index].Error = fmt.Sprintf("写入成功但回读重查失败: %v", secondErr)
					} else {
						result.Outcomes[index].Error = "写入成功但回读未取得原生 image_token（可能存在服务端同步延迟）"
					}
					continue
				}
			}
			result.Outcomes[index].Status = "verified"
			result.Outcomes[index].ImageToken = token
		}
	}

	tallyBatchResult(result)
	if !result.Verified {
		return result, fmt.Errorf("批量图片写入存在 %d 个失败项", result.Failed)
	}
	return result, nil
}

func writeSingleSheetImage(ctx context.Context, httpClient *http.Client, spreadsheetToken string, item BatchWriteSheetImageItem, maxBytes int64, allowPrivateNet bool, uat string, writeSlot chan struct{}) BatchWriteSheetImageOutcome {
	source := "path"
	if item.URL != "" {
		source = "url"
	}
	outcome := BatchWriteSheetImageOutcome{Cell: item.Cell, Source: source, Status: "failed"}

	imagePath := item.Path
	suggestedName := ""
	cleanupTemp := false

	if item.URL != "" {
		var err error
		imagePath, suggestedName, err = downloadSheetImage(ctx, httpClient, item.URL, maxBytes, allowPrivateNet)
		if err != nil {
			outcome.Error = err.Error()
			return outcome
		}
		cleanupTemp = true
	}
	if cleanupTemp {
		defer os.Remove(imagePath)
	}

	ext, err := validateLocalSheetImage(imagePath, maxBytes)
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}

	// 用户指定的名称优先级最高，避免临时文件名覆盖业务名称
	finalName := strings.TrimSpace(item.Name)
	if finalName == "" {
		if suggestedName != "" {
			finalName = suggestedName
		} else {
			finalName = filepath.Base(imagePath)
		}
	}
	normalizedPath, normalizedExt, err := normalizeSheetImageFile(imagePath, ext, maxBytes)
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	if normalizedPath != imagePath {
		defer os.Remove(normalizedPath)
		if cleanupTemp {
			// 转换完成即可释放下载原件，等待写入时只保留转换后的文件。
			_ = os.Remove(imagePath)
		}
		imagePath = normalizedPath
	}
	ext = normalizedExt
	finalName = normalizeSheetImageName(finalName, ext)

	select {
	case <-ctx.Done():
		outcome.Error = fmt.Sprintf("等待图片写入被中断: %v", ctx.Err())
		return outcome
	case writeSlot <- struct{}{}:
	}
	defer func() { <-writeSlot }()

	err = retrySheetImageWrite(ctx, func() error {
		writeCtx, writeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer writeCancel()
		return WriteSheetImage(writeCtx, spreadsheetToken, item.Cell, imagePath, finalName, uat)
	})

	if err == nil {
		outcome.Status = "written"
		return outcome
	}

	outcome.Error = fmt.Sprintf("图片写入失败: %v", err)
	return outcome
}

// retrySheetImageWrite 补充表格特有的限流/前序操作未完成错误，不改变其他业务的重试规则。
func retrySheetImageWrite(ctx context.Context, write func() error) error {
	failures := 0
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := write()
		if err == nil {
			return nil
		}
		rateLimited := IsRateLimitError(err) || HasAPICode(err, 90217)
		retryable := rateLimited || HasAPICode(err, 90235) || IsRetryableError(err)
		if !rateLimited {
			failures++
		}
		if !retryable || IsPermanentError(err) || failures > 2 || attempt >= 4 {
			return err
		}
		// values_image 尚未透出响应 Header，沿用共享的全抖动指数退避。
		timer := time.NewTimer(GetRetryWaitDuration(nil, attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func downloadSheetImage(ctx context.Context, httpClient *http.Client, rawURL string, maxBytes int64, allowPrivateNet bool) (string, string, error) {
	if err := validateSheetImageURL(rawURL, allowPrivateNet); err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("创建图片下载请求失败: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("图片下载返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return "", "", fmt.Errorf("图片大小超过上限 %d 字节（Content-Length: %d）", maxBytes, resp.ContentLength)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("读取图片响应失败: %w", err)
	}
	if resp.ContentLength > 0 && int64(len(data)) != resp.ContentLength {
		return "", "", fmt.Errorf("图片下载不完整（预期 %d 字节，实际接收 %d 字节）", resp.ContentLength, len(data))
	}
	if int64(len(data)) > maxBytes {
		return "", "", fmt.Errorf("图片大小超过上限 %d 字节", maxBytes)
	}

	ext, ok := detectSheetImageExtension(data)
	if !ok {
		return "", "", fmt.Errorf("响应不是支持的图片类型（当前 %s）", http.DetectContentType(data))
	}

	file, err := os.CreateTemp("", "feishu-sheet-img-*"+ext)
	if err != nil {
		return "", "", fmt.Errorf("创建临时图片文件失败: %w", err)
	}
	tmpPath := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("写入临时图片文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("关闭临时图片文件失败: %w", err)
	}

	suggestedName := ""
	if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
		base := path.Base(parsed.Path)
		if base != "" && base != "." && base != "/" {
			suggestedName = base
		}
	}

	return tmpPath, normalizeSheetImageName(suggestedName, ext), nil
}

func validateSheetImageURL(raw string, allowPrivateNet bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("图片 URL 为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("解析图片 URL 失败: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("图片 URL 必须是 HTTPS 地址（当前 %q）", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("图片 URL 缺少 host")
	}
	if parsed.User != nil {
		return errors.New("图片 URL 不允许包含凭据")
	}

	lowered := strings.ToLower(host)
	if lowered == "localhost" || strings.HasSuffix(lowered, ".localhost") {
		return errors.New("图片 URL 不允许指向 localhost")
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("图片 URL 不允许指向本地回环或受限地址: %s", ip.String())
		}
		if !allowPrivateNet && isBlockedIP(ip) {
			return fmt.Errorf("图片 URL 指向私有网络地址: %s（如需使用内网图片源请设置 --allow-private-net）", ip.String())
		}
	} else if !allowPrivateNet {
		ips, lookupErr := net.LookupIP(host)
		if lookupErr != nil {
			return fmt.Errorf("解析图片域名 %s 失败: %w", host, lookupErr)
		}
		for _, ip := range ips {
			if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isBlockedIP(ip) {
				return fmt.Errorf("图片 URL 域名解析到私有或受限地址 %s（如需使用内网图片源请设置 --allow-private-net）", ip.String())
			}
		}
	}

	return nil
}

func validateLocalSheetImage(path string, maxBytes int64) (string, error) {
	// 本地文件内容会上传到飞书：拒绝敏感目录（~/.ssh、~/.feishu-cli、/etc 等），命令层漏校验时兜底
	if err := safefile.ValidateInputPath(path); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取图片文件失败: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("读取图片文件信息失败: %w", err)
	}
	if !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > maxBytes {
		return "", fmt.Errorf("图片必须是 1-%d 字节的普通文件", maxBytes)
	}

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("读取图片文件失败: %w", err)
	}
	ext, ok := detectSheetImageExtension(header[:n])
	if !ok {
		return "", fmt.Errorf("文件不是支持的图片类型（当前 %s）", http.DetectContentType(header[:n]))
	}
	return ext, nil
}

// detectSheetImageExtension 按文件内容识别格式，补齐标准库未识别的 TIFF/BPG/HEIC。
// JFIF、EXIF 均为 JPEG 的容器约定，统一通过 JPEG 文件头识别。
func detectSheetImageExtension(data []byte) (string, bool) {
	if ext, ok := supportedSheetImageMIMEExt(http.DetectContentType(data)); ok {
		return ext, true
	}
	if bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")) ||
		bytes.HasPrefix(data, []byte("II+\x00\x08\x00\x00\x00")) || bytes.HasPrefix(data, []byte("MM\x00+\x00\x08\x00\x00")) {
		return ".tiff", true
	}
	if bytes.HasPrefix(data, []byte("BPG\xfb")) {
		return ".bpg", true
	}
	if len(data) >= 16 && string(data[4:8]) == "ftyp" {
		boxSize := uint64(binary.BigEndian.Uint32(data[:4]))
		if boxSize >= 16 {
			// 仅读取 ftyp box 中的 major/compatible brands，跳过 minor_version。
			for offset := 8; offset+4 <= len(data) && uint64(offset+4) <= boxSize; offset += 4 {
				if offset == 12 {
					continue
				}
				switch string(data[offset : offset+4]) {
				case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs":
					return ".heic", true
				}
			}
		}
	}
	return "", false
}

func supportedSheetImageMIMEExt(mimeType string) (string, bool) {
	switch mimeType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	case "image/bmp", "image/x-ms-bmp":
		return ".bmp", true
	default:
		return "", false
	}
}

// normalizeSheetImageName 保留与实际格式相符的后缀，缺失或不符时按内容补正。
func normalizeSheetImageName(name, ext string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "image"
	}
	current := filepath.Ext(name)
	canonical := strings.ToLower(current)
	switch canonical {
	case ".jpeg", ".jfif", ".exif":
		canonical = ".jpg"
	case ".tif":
		canonical = ".tiff"
	}
	if canonical == ext {
		return name
	}
	return strings.TrimSuffix(name, current) + ext
}

// extractCellCoordinate 提取单格坐标（如 "0b1212!B2:B2" -> "B2", "B2:B2" -> "B2", "B2" -> "B2"）
func extractCellCoordinate(rangeStr string) string {
	s := strings.TrimSpace(rangeStr)
	if idx := strings.LastIndex(s, "!"); idx >= 0 {
		s = s[idx+1:]
	}
	if idx := strings.Index(s, ":"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, "$", "")
	return strings.ToUpper(strings.TrimSpace(s))
}

func verifiedSheetImagesByCell(ranges []*CellRangeV3) map[string]string {
	result := make(map[string]string)
	for _, valueRange := range ranges {
		if valueRange == nil || len(valueRange.Values) < 1 || len(valueRange.Values[0]) < 1 {
			continue
		}
		coord := extractCellCoordinate(valueRange.Range)
		elements := valueRange.Values[0][0]
		for _, elem := range elements {
			if elem != nil && elem.Type == "image" && elem.Image != nil && elem.Image.ImageToken != "" {
				result[coord] = elem.Image.ImageToken
				break
			}
		}
	}
	return result
}

func tallyBatchResult(result *BatchWriteSheetImageResult) {
	if result == nil {
		return
	}
	result.Written = 0
	result.Failed = 0
	for _, outcome := range result.Outcomes {
		if outcome.Status == "verified" {
			result.Written++
		} else {
			result.Failed++
		}
	}
	result.Verified = result.Total > 0 && result.Failed == 0 && result.Written == result.Total
}
