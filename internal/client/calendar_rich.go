package client

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"  // 注册 GIF 解码器，读取描述图片尺寸
	_ "image/jpeg" // 注册 JPEG 解码器
	_ "image/png"  // 注册 PNG 解码器
	"net/http"
	"os"
	"strings"

	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"
	"github.com/riba2534/feishu-cli/internal/config"
)

// ============================================================================
// 日程富文本描述（description_rich）：SDK 结构体没有该字段，按 SDK 构造好的日程序列化后
// 去掉纯文本 description、写入 description_rich，再手写请求发送。对齐官方 calendar +create/+update。
// ============================================================================

// calendarMediaParentType 日程描述图片上传到云空间时的 parent_type
const calendarMediaParentType = "calendar"

// BuildEventBodyWithRich 把 SDK 日程对象转成请求体，并用 description_rich 替换 description。
func BuildEventBodyWithRich(event *larkcalendar.CalendarEvent, descriptionRich string) (map[string]any, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("序列化日程失败: %w", err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("序列化日程失败: %w", err)
	}
	delete(body, "description")
	body["description_rich"] = descriptionRich
	return body, nil
}

// sendCalendarEventWithRich 以带 description_rich 的请求体创建（POST）或更新（PATCH）日程。
func sendCalendarEventWithRich(method, apiPath string, event *larkcalendar.CalendarEvent, descriptionRich, action, userAccessToken string) (*CalendarEvent, error) {
	body, err := BuildEventBodyWithRich(event, descriptionRich)
	if err != nil {
		return nil, err
	}
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	var status int
	var header http.Header
	var rawBody []byte
	switch method {
	case http.MethodPost:
		resp, e := cli.Post(Context(), apiPath, body, tokenType, opts...)
		if e != nil {
			return nil, fmt.Errorf("%s失败: %w", action, e)
		}
		status, header, rawBody = resp.StatusCode, resp.Header, resp.RawBody
	case http.MethodPatch:
		resp, e := cli.Patch(Context(), apiPath, body, tokenType, opts...)
		if e != nil {
			return nil, fmt.Errorf("%s失败: %w", action, e)
		}
		status, header, rawBody = resp.StatusCode, resp.Header, resp.RawBody
	default:
		return nil, fmt.Errorf("%s失败: 不支持的 HTTP 方法 %s", action, method)
	}
	if err := ParseAPIResponse(action, status, header, rawBody); err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Event *larkcalendar.CalendarEvent `json:"event"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if env.Data.Event == nil {
		return nil, fmt.Errorf("%s成功但未返回日程信息", action)
	}
	return convertEvent(env.Data.Event), nil
}

// UploadCalendarDescriptionImage 把本地图片上传到云空间（parent_type=calendar，parent_node=日历 ID），
// 返回可直接写进 description_rich 的图片预览 URL（带宽高与大小，客户端按原尺寸渲染）。
// 调用方负责在第一次网络请求前完成路径校验。
func UploadCalendarDescriptionImage(filePath, fileName, calendarID, userAccessToken string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("读取描述图片失败: %w", err)
	}
	fileToken, _, err := UploadMedia(filePath, calendarMediaParentType, calendarID, fileName, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("上传描述图片 %s 失败: %w", fileName, err)
	}
	w, h := imageDimensions(filePath)
	return CalendarImagePreviewURL(calendarImageHost(), fileToken, w, h, info.Size()), nil
}

// CalendarImagePreviewURL 构造云空间图片预览 URL（与官方 buildCalendarImagePreviewURL 一致）
func CalendarImagePreviewURL(host, fileToken string, width, height int, size int64) string {
	u := fmt.Sprintf("https://%s/space/api/box/stream/download/preview/%s?preview_type=16", host, fileToken)
	if width > 0 && height > 0 {
		u += fmt.Sprintf("&im_w=%d&im_h=%d", width, height)
	}
	if size > 0 {
		u += fmt.Sprintf("&im_size=%d", size)
	}
	return u
}

// calendarImageHost 按开放平台域名选择图片预览域名（Lark 国际版用 larksuite.com）
func calendarImageHost() string {
	if cfg := config.Get(); cfg != nil && strings.Contains(cfg.BaseURL, "larksuite.com") {
		return "internal-api-drive-stream.larksuite.com" // 官方公共图片预览域名，非企业域名 privacy-scan: allow
	}
	return "internal-api-drive-stream.feishu.cn" // 官方公共图片预览域名，非企业域名 privacy-scan: allow
}

// imageDimensions 读取图片宽高；无法识别时返回 0,0（URL 不带尺寸参数）
func imageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
