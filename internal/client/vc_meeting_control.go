package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// 会中控制（对齐官方 shortcuts/vc 的 +meeting-countdown / +meeting-message-send / +meeting-invite /
// +meeting-end / +meeting-screenshot）。端点均在 /open-apis/vc/v1/bots/ 下，未收录于内嵌 catalog。

// VCBotCountdown 操作会中倒计时：POST /open-apis/vc/v1/bots/countdown（User / Bot）。
func VCBotCountdown(body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("操作会中倒计时", http.MethodPost, vcBase+"/bots/countdown", body, userAccessToken)
}

// VCBotMessageSend 发送会中消息或表情：POST /open-apis/vc/v1/bots/message（User / Bot）。
func VCBotMessageSend(body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("发送会中消息", http.MethodPost, vcBase+"/bots/message", body, userAccessToken)
}

// VCBotInvite 以应用机器人身份邀请参会人：POST /open-apis/vc/v1/bots/invite?user_id_type=open_id（仅 Bot）。
func VCBotInvite(body map[string]any) (json.RawMessage, error) {
	return vcCallAPI("机器人邀请参会人", http.MethodPost, vcBase+"/bots/invite?user_id_type=open_id", body, "")
}

// VCBotEndMeeting 以主持人机器人身份结束会议：POST /open-apis/vc/v1/bots/end（仅 Bot）。
func VCBotEndMeeting(meetingID string) (json.RawMessage, error) {
	return vcCallAPI("机器人结束会议", http.MethodPost, vcBase+"/bots/end", map[string]any{"meeting_id": meetingID}, "")
}

// VCBotScreenshotMaxBytes 会中截图响应体上限（对齐官方 8MB）。
const VCBotScreenshotMaxBytes = 8 << 20

// VCBotScreenshot 获取会中截图：POST /open-apis/vc/v1/bots/screenshot，成功时响应体是 JPEG 原始字节；
// 失败时为飞书 JSON 信封（按业务码解析，保留 log_id）。返回 JPEG 字节与 Content-Type。
func VCBotScreenshot(meetingID, userAccessToken string) ([]byte, string, error) {
	const action = "获取会中截图"
	bearer, err := driveDownloadBearer(userAccessToken)
	if err != nil {
		return nil, "", err
	}
	payload, err := json.Marshal(map[string]string{"meeting_id": meetingID})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(Context(), http.MethodPost, buildOpenAPIURL(vcBase+"/bots/screenshot"), bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("%s失败: %w", action, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := rawHTTPClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s失败: %w", action, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, VCBotScreenshotMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%s失败: 读取响应失败: %w", action, err)
	}
	if len(body) > VCBotScreenshotMaxBytes {
		return nil, "", fmt.Errorf("%s失败: 截图超过 %d 字节上限", action, VCBotScreenshotMaxBytes)
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if contentType != "image/jpeg" {
		// 非 JPEG：先按飞书业务信封解析（错误随 HTTP 400 下发时也能拿到 code/log_id）
		if perr := ParseAPIResponse(action, resp.StatusCode, resp.Header, body); perr != nil {
			return nil, "", perr
		}
		return nil, "", fmt.Errorf("%s失败: 响应类型 %q 不是 image/jpeg", action, resp.Header.Get("Content-Type"))
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("%s失败: HTTP %d", action, resp.StatusCode)
	}
	if !looksLikeJPEG(body) {
		return nil, "", fmt.Errorf("%s失败: 响应不是有效的 JPEG", action)
	}
	return body, contentType, nil
}

// looksLikeJPEG 校验 SOI/EOI 标记（FFD8 ... FFD9）。
func looksLikeJPEG(b []byte) bool {
	n := len(b)
	return n >= 4 && b[0] == 0xff && b[1] == 0xd8 && b[n-2] == 0xff && b[n-1] == 0xd9
}
