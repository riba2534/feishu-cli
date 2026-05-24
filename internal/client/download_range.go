package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var rangeDownloadChunkSize int64 = 8 * 1024 * 1024

func newBearerDownloadRequest(reqURL, bearerToken, byteRange string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	return req, nil
}

type downloadAPIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
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

func downloadBearerURLByRange(action, reqURL, outputPath, bearerToken string, timeout time.Duration) error {
	if rangeDownloadChunkSize <= 0 {
		return fmt.Errorf("%s失败: Range 分片大小非法: %d", action, rangeDownloadChunkSize)
	}

	httpClient := &http.Client{Timeout: timeout}
	var outFile *os.File
	var total int64 = -1
	var nextStart int64

	cleanupOutput := false
	defer func() {
		if outFile != nil {
			_ = outFile.Close()
		}
		if cleanupOutput {
			_ = os.Remove(outputPath)
		}
	}()

	for {
		end := nextStart + rangeDownloadChunkSize - 1
		byteRange := fmt.Sprintf("bytes=%d-%d", nextStart, end)
		req, err := newBearerDownloadRequest(reqURL, bearerToken, byteRange)
		if err != nil {
			return fmt.Errorf("%s失败: %w", action, err)
		}

		httpResp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("%s失败: Range %s: %w", action, byteRange, err)
		}

		if httpResp.StatusCode == http.StatusOK && nextStart == 0 {
			if err := writeStreamToFile(httpResp.Body, outputPath); err != nil {
				_ = httpResp.Body.Close()
				return fmt.Errorf("保存文件失败: %w", err)
			}
			_ = httpResp.Body.Close()
			return nil
		}

		if httpResp.StatusCode != http.StatusPartialContent {
			apiErr, parseErr := parseDownloadAPIError(action, httpResp)
			_ = httpResp.Body.Close()
			if parseErr != nil {
				return parseErr
			}
			return fmt.Errorf("%s失败: Range %s: code=%d, msg=%s", action, byteRange, apiErr.Code, apiErr.Msg)
		}

		rangeStart, rangeEnd, rangeTotal, err := parseContentRange(httpResp.Header.Get("Content-Range"))
		if err != nil {
			_ = httpResp.Body.Close()
			return fmt.Errorf("%s失败: 解析 Content-Range 失败: %w", action, err)
		}
		if rangeStart != nextStart {
			_ = httpResp.Body.Close()
			return fmt.Errorf("%s失败: Range 响应起点不匹配: got %d, want %d", action, rangeStart, nextStart)
		}
		if total < 0 {
			total = rangeTotal
		} else if total != rangeTotal {
			_ = httpResp.Body.Close()
			return fmt.Errorf("%s失败: 文件大小变化: got %d, want %d", action, rangeTotal, total)
		}

		if outFile == nil {
			outFile, err = os.Create(outputPath)
			if err != nil {
				_ = httpResp.Body.Close()
				return fmt.Errorf("保存文件失败: 创建输出文件失败: %w", err)
			}
			cleanupOutput = true
		}

		written, err := io.Copy(outFile, httpResp.Body)
		_ = httpResp.Body.Close()
		if err != nil {
			return fmt.Errorf("保存文件失败: 写入分片失败: %w", err)
		}
		expected := rangeEnd - rangeStart + 1
		if written != expected {
			return fmt.Errorf("%s失败: Range %s 写入大小不匹配: got %d, want %d", action, byteRange, written, expected)
		}

		nextStart = rangeEnd + 1
		if nextStart >= total {
			break
		}
	}

	if outFile != nil {
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("保存文件失败: 关闭输出文件失败: %w", err)
		}
		outFile = nil
	}
	cleanupOutput = false
	return nil
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

func writeStreamToFile(reader io.Reader, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}

	_, copyErr := io.Copy(outFile, reader)
	if closeErr := outFile.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(outputPath)
		return fmt.Errorf("写入文件失败: %w", copyErr)
	}
	return nil
}
