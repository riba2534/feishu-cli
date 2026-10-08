package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GetMailAttachmentDownloadURLs 批量获取邮件附件（含内联图片）的预签名下载链接。
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/{message_id}/attachments/download_url?attachment_ids=a&attachment_ids=b
// 每批最多 20 个 ID（与官方一致）。返回 attachment_id → download_url，以及服务端 failed_ids。
func GetMailAttachmentDownloadURLs(mailboxID, messageID string, attachmentIDs []string, userAccessToken string) (map[string]string, []string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	urls := make(map[string]string, len(attachmentIDs))
	var failed []string
	const batchSize = 20
	for i := 0; i < len(attachmentIDs); i += batchSize {
		end := i + batchSize
		if end > len(attachmentIDs) {
			end = len(attachmentIDs)
		}
		q := url.Values{}
		for _, id := range attachmentIDs[i:end] {
			q.Add("attachment_ids", id)
		}
		apiPath := mailboxPath(mailboxID, "messages", messageID, "attachments", "download_url") + "?" + q.Encode()
		data, err := callMailAPI(http.MethodGet, apiPath, nil, userAccessToken)
		if err != nil {
			return nil, nil, err
		}
		var resp struct {
			DownloadURLs []struct {
				AttachmentID string `json:"attachment_id"`
				DownloadURL  string `json:"download_url"`
			} `json:"download_urls"`
			FailedIDs []string `json:"failed_ids"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, nil, fmt.Errorf("解析附件下载链接响应失败: %w", err)
		}
		for _, item := range resp.DownloadURLs {
			if item.AttachmentID != "" {
				urls[item.AttachmentID] = item.DownloadURL
			}
		}
		failed = append(failed, resp.FailedIDs...)
	}
	return urls, failed, nil
}

// DownloadPresignedBytes 把预签名 URL 的内容下载到内存（用于邮件附件转发等小文件场景）。
//   - 只允许 https，复用 validateDownloadURL 的 SSRF 防护（拒绝 localhost / 内网 IP），每次重定向重新校验；
//   - 不携带任何鉴权头：预签名 URL 自带授权参数，附带 Bearer 会把 token 泄露给目标主机；
//   - 超过 maxBytes 直接报错，避免无界内存占用。
func DownloadPresignedBytes(rawURL string, maxBytes int64) ([]byte, error) {
	if !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		return nil, errors.New("下载链接必须是 https 地址")
	}
	if err := validateDownloadURL(rawURL); err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= downloadMaxRedirects {
				return fmt.Errorf("下载重定向次数超过 %d", downloadMaxRedirects)
			}
			if req.URL.Scheme != "https" {
				return errors.New("下载重定向不允许从 HTTPS 降级")
			}
			return validateDownloadURL(req.URL.String())
		},
	}
	req, err := http.NewRequestWithContext(Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造下载请求失败: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取下载内容失败: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("内容超过 %dMB 下载上限", maxBytes/1024/1024)
	}
	return data, nil
}
