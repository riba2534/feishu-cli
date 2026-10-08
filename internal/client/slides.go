package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// isOfficePresentation 判定演示文稿 token 是否为导入的 Office deck。
// 规则统一由 IsLocalOfficeToken 维护（前缀 fake_office_/local_office_，或长度 ≥25 且固定偏移为 OFL0X），
// 避免 slides 与 sheets 各存一份副本、格式变化时漏改。
func isOfficePresentation(token string) bool {
	return IsLocalOfficeToken(token)
}

// slidesMediaParentType 根据演示文稿 token 返回上传 media 时使用的 parent_type。
// 导入型 Office deck 使用 "office_slide_file"，原生 Slides 演示文稿使用 "slide_file"。
// 同时只接受单分片 upload_all 接口（最大 20 MB），upload_prepare 不支持。
func slidesMediaParentType(presentationToken string) string {
	if isOfficePresentation(presentationToken) {
		return "office_slide_file"
	}
	return "slide_file"
}

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
	// 先解析业务信封再看 HTTP 状态：飞书大量业务错误（如 99991672 缺 scope）随 HTTP 400 下发
	if err := CheckAPIResponse("创建 slides", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data struct {
			XmlPresentationID string `json:"xml_presentation_id"`
			RevisionID        int    `json:"revision_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 slides 创建响应失败: %w", err)
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
// parent_type 根据 presentationID 自动选择：普通 deck 用 slide_file，imported Office deck 用 office_slide_file
// 只能走单分片 upload_all（最大 20 MB）
// 权限: docs:document.media:upload
func UploadSlidesMedia(filePath, fileName, presentationID, userAccessToken string) (string, error) {
	parentType := slidesMediaParentType(presentationID)
	token, _, err := UploadMediaWithExtra(filePath, parentType, presentationID, fileName, "", userAccessToken)
	return token, err
}

// GetSlidesResult 读取演示文稿返回的数据
type GetSlidesResult struct {
	XmlPresentationID string `json:"xml_presentation_id"`
	Content           string `json:"content"`
	RevisionID        int    `json:"revision_id"`
}

// GetSlides 读取指定 XML 演示文稿的全文信息
// API: GET /open-apis/slides_ai/v1/xml_presentations/{xml_presentation_id}
// 权限: slides:presentation:read
//
// revisionID 原样下发（-1 表示最新）。实测服务端对正整数版本号忽略、始终返回最新版本，
// 0 则报 3350001；过去这里把 0 静默改成 -1，掩盖了调用方的错误输入，现在交给命令层校验。
func GetSlides(presentationID string, revisionID int, userAccessToken ...string) (*GetSlidesResult, error) {
	return GetSlidesWithOptions(presentationID, revisionID, false, firstString(userAccessToken))
}

// GetSlidesWithOptions 同 GetSlides，removeAttrID=true 时请求服务端去掉返回 XML 中的 id 属性（只读查看用）。
func GetSlidesWithOptions(presentationID string, revisionID int, removeAttrID bool, userAccessToken string) (*GetSlidesResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("revision_id", fmt.Sprintf("%d", revisionID))
	if removeAttrID {
		q.Set("remove_attr_id", "true")
	}
	apiPath := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s?%s", url.PathEscape(presentationID), q.Encode())

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(userAccessToken)
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, reqOpts...)
	if err != nil {
		return nil, fmt.Errorf("读取 slides 失败: %w", err)
	}
	if err := CheckAPIResponse("读取 slides", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data struct {
			XmlPresentation struct {
				Content        string `json:"content"`
				PresentationID string `json:"presentation_id"`
				RevisionID     int    `json:"revision_id"`
			} `json:"xml_presentation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 slides 响应失败: %w", err)
	}

	if strings.TrimSpace(apiResp.Data.XmlPresentation.Content) == "" {
		return nil, fmt.Errorf("读取 slides 失败: 返回的演示文稿内容为空")
	}

	presID := apiResp.Data.XmlPresentation.PresentationID
	if presID == "" {
		presID = presentationID
	}

	return &GetSlidesResult{
		XmlPresentationID: presID,
		Content:           apiResp.Data.XmlPresentation.Content,
		RevisionID:        apiResp.Data.XmlPresentation.RevisionID,
	}, nil
}

// buildPresentationXML 构造最小可用的 presentation XML，新建空白演示文稿用
func buildPresentationXML(title string, width, height int) string {
	return fmt.Sprintf(
		`<presentation xmlns="https://www.larkoffice.com/sml/2.0" width="%d" height="%d"><title>%s</title></presentation>`,
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

// SlidesCall 调用 slides_ai JSON 端点并返回 data 子对象（不存在时返回空 map）。
//
// 身份：userAccessToken 非空走 User Token，否则走 Tenant Token。
// 错误：先按飞书业务信封解析（业务错误常随 HTTP 400 下发），返回 *APIError（Error() 为
// "<action>失败: code=N, msg=..."），HasAPICode / AsAPIError 照常可用。
// query 中 nil 值跳过，[]string 展开为重复参数。
func SlidesCall(action, method, path string, query map[string]any, body any, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	queryParams := make(larkcore.QueryParams)
	for k, v := range query {
		switch val := v.(type) {
		case nil:
		case []string:
			for _, item := range val {
				queryParams.Add(k, item)
			}
		default:
			queryParams.Set(k, fmt.Sprintf("%v", v))
		}
	}
	tokenTypes := []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}
	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		tokenTypes = []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}
		opts = append(opts, larkcore.WithUserAccessToken(userAccessToken))
	}
	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   path,
		Body:                      body,
		QueryParams:               queryParams,
		SupportedAccessTokenTypes: tokenTypes,
	}
	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	dec := json.NewDecoder(strings.NewReader(string(resp.RawBody)))
	dec.UseNumber()
	if err := dec.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if envelope.Data == nil {
		envelope.Data = map[string]any{}
	}
	return envelope.Data, nil
}

// SlidesPresentationPath 返回 /open-apis/slides_ai/v1/xml_presentations/{id}{suffix}。
func SlidesPresentationPath(presentationID, suffix string) string {
	return fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s%s", url.PathEscape(presentationID), suffix)
}
