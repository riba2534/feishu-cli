package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// slidesMediaParentType 是 slides 后端唯一接受的 medias/upload_all parent_type
// 已经 lark-cli 经验证过 slide_image / slides_image / slides_file 都会被拒，
// 只有 slide_file 才能拿到可在 slide XML <img src="..."> 引用的 file_token。
// 同时只接受单分片 upload_all 接口（最大 20 MB），upload_prepare 不支持。
const slidesMediaParentType = "slide_file"

const (
	defaultPresentationWidth  = 960
	defaultPresentationHeight = 540
)

// CreateSlidesResult 创建演示文稿后返回的数据
type CreateSlidesResult struct {
	XmlPresentationID string `json:"xml_presentation_id"`
	RevisionID        int    `json:"revision_id,omitempty"`
	Title             string `json:"title,omitempty"`
}

// CreateSlidesOptions 创建演示文稿可选参数
type CreateSlidesOptions struct {
	Title           string // 演示文稿标题，默认 "Untitled"
	Width           int    // 默认 960
	Height          int    // 默认 540
	UserAccessToken string // 可选 User Access Token
}

// CreateSlides 通过 slides_ai openapi 创建一个空白演示文稿
// API: POST /open-apis/slides_ai/v1/xml_presentations
// body: {"xml_presentation": {"content": "<presentation ...><title>...</title></presentation>"}}
// 权限: slides:presentation:create / slides:presentation:write_only
func CreateSlides(opts CreateSlidesOptions) (*CreateSlidesResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	title := opts.Title
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}
	width := opts.Width
	if width <= 0 {
		width = defaultPresentationWidth
	}
	height := opts.Height
	if height <= 0 {
		height = defaultPresentationHeight
	}

	content := buildPresentationXML(title, width, height)
	reqBody := map[string]any{
		"xml_presentation": map[string]any{
			"content": content,
		},
	}

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if opts.UserAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(opts.UserAccessToken)
	}

	resp, err := client.Post(Context(), "/open-apis/slides_ai/v1/xml_presentations", reqBody, tokenType, reqOpts...)
	if err != nil {
		return nil, fmt.Errorf("创建 slides 失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建 slides 失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			XmlPresentationID string `json:"xml_presentation_id"`
			RevisionID        int    `json:"revision_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 slides 创建响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建 slides 失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data.XmlPresentationID == "" {
		return nil, fmt.Errorf("创建 slides 成功但未返回 xml_presentation_id")
	}

	return &CreateSlidesResult{
		XmlPresentationID: apiResp.Data.XmlPresentationID,
		RevisionID:        apiResp.Data.RevisionID,
		Title:             title,
	}, nil
}

// UploadSlidesMedia 把本地图片上传到 slides 演示文稿，返回的 file_token 可作为 <img src="..."> 使用
// 必须用 parent_type=slide_file（lark-cli 实测，其他值都会被拒），且只能走单分片 upload_all（最大 20 MB）
// 权限: docs:document.media:upload
func UploadSlidesMedia(filePath, fileName, presentationID, userAccessToken string) (string, error) {
	token, _, err := UploadMediaWithExtra(filePath, slidesMediaParentType, presentationID, fileName, "", userAccessToken)
	return token, err
}

// buildPresentationXML 构造最小可用的 presentation XML，新建空白演示文稿用
func buildPresentationXML(title string, width, height int) string {
	return fmt.Sprintf(
		`<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="%d" height="%d"><title>%s</title></presentation>`,
		width, height, xmlEscape(title),
	)
}

// xmlEscape 对 XML 文本节点的特殊字符做转义
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
