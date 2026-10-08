package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

// Comment 评论信息。
//
// 字段名与服务端 /drive/v1/files/:token/comments 返回的 item 一致，可直接反序列化；
// reply_list 原样保留服务端结构（replies[].content.elements 中的 text_run / docs_link / person 等），
// content 为根回复（即评论正文）渲染出的可读文本。
type Comment struct {
	CommentID    string `json:"comment_id"`
	UserID       string `json:"user_id,omitempty"`
	CreateTime   int    `json:"create_time,omitempty"`
	UpdateTime   int    `json:"update_time,omitempty"`
	IsSolved     bool   `json:"is_solved"`
	SolvedTime   int    `json:"solved_time,omitempty"`
	SolverUserID string `json:"solver_user_id,omitempty"`
	IsWhole      bool   `json:"is_whole"`
	// Quote 划词评论选中的原文；IsWhole=true 时为空
	Quote string `json:"quote,omitempty"`
	// Content 评论正文（根回复）的可读文本：@人渲染为 @user_id，文档链接渲染为 URL
	Content string `json:"content,omitempty"`
	// ReplyList 服务端原始回复列表 {"replies":[{reply_id,user_id,create_time,content:{elements:[...]}, extra}]}
	ReplyList json.RawMessage `json:"reply_list,omitempty"`
	// ReplyHasMore / ReplyPageToken 是该评论下回复的分页信息（回复过多时需用 comment reply list 续翻）
	ReplyHasMore   bool   `json:"has_more,omitempty"`
	ReplyPageToken string `json:"page_token,omitempty"`
}

// ListCommentsOptions 评论列表查询参数。
type ListCommentsOptions struct {
	FileToken string
	FileType  string
	PageSize  int
	PageToken string
	// IsSolved 非 nil 时按解决状态过滤（true=已解决，false=未解决）；nil 表示全部
	IsSolved *bool
	// IsWhole 非 nil 时按评论范围过滤（true=全文评论，false=局部评论）；nil 表示全部
	IsWhole *bool
}

// commentReplyWire 是服务端回复对象的原始形状。
type commentReplyWire struct {
	ReplyID    string `json:"reply_id"`
	UserID     string `json:"user_id"`
	CreateTime int    `json:"create_time"`
	UpdateTime int    `json:"update_time"`
	Content    *struct {
		Elements []json.RawMessage `json:"elements"`
	} `json:"content"`
	Extra json.RawMessage `json:"extra"`
}

func (w commentReplyWire) toReply() *CommentReply {
	r := &CommentReply{
		ReplyID:    w.ReplyID,
		UserID:     w.UserID,
		CreateTime: w.CreateTime,
		UpdateTime: w.UpdateTime,
	}
	if w.Content != nil {
		r.Elements = w.Content.Elements
		r.Content = RenderCommentElements(w.Content.Elements)
	}
	if len(bytes.TrimSpace(w.Extra)) > 0 && string(bytes.TrimSpace(w.Extra)) != "null" {
		r.Extra = w.Extra
	}
	return r
}

// commentElementView 是渲染可读文本时使用的元素视图（未知字段不影响原始结构的保留）。
type commentElementView struct {
	Type    string `json:"type"`
	TextRun *struct {
		Text string `json:"text"`
	} `json:"text_run"`
	DocsLink *struct {
		URL string `json:"url"`
	} `json:"docs_link"`
	Person *struct {
		UserID string `json:"user_id"`
	} `json:"person"`
}

// RenderCommentElements 把服务端 reply content.elements 渲染为可读文本：
// text_run → 文本，docs_link → URL，person → @user_id；无法识别的元素渲染为 [type]。
func RenderCommentElements(elements []json.RawMessage) string {
	var b strings.Builder
	for _, raw := range elements {
		var el commentElementView
		if err := json.Unmarshal(raw, &el); err != nil {
			continue
		}
		switch {
		case el.TextRun != nil:
			b.WriteString(el.TextRun.Text)
		case el.DocsLink != nil:
			b.WriteString(el.DocsLink.URL)
		case el.Person != nil:
			b.WriteString("@" + el.Person.UserID)
		case el.Type != "":
			b.WriteString("[" + el.Type + "]")
		}
	}
	return b.String()
}

// Replies 解析 Comment.ReplyList（服务端原始结构）为回复列表，供文本模式渲染。
func (c *Comment) Replies() []*CommentReply {
	if c == nil || len(bytes.TrimSpace(c.ReplyList)) == 0 {
		return nil
	}
	var list struct {
		Replies []commentReplyWire `json:"replies"`
	}
	if err := json.Unmarshal(c.ReplyList, &list); err != nil {
		return nil
	}
	out := make([]*CommentReply, 0, len(list.Replies))
	for _, w := range list.Replies {
		out = append(out, w.toReply())
	}
	// 实测 comments 列表里的 reply_list 不保证按时间排序；按创建时间升序排列，首条即根回复（评论正文）
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreateTime != out[j].CreateTime {
			return out[i].CreateTime < out[j].CreateTime
		}
		return lessNumericID(out[i].ReplyID, out[j].ReplyID)
	})
	return out
}

// lessNumericID 比较数字字符串 ID（长度优先，再按字典序），非数字时退化为字典序。
func lessNumericID(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// fillCommentContent 用根回复（评论正文）填充 Content。
func fillCommentContent(c *Comment) {
	if c == nil || c.Content != "" {
		return
	}
	if replies := c.Replies(); len(replies) > 0 {
		c.Content = replies[0].Content
	}
}

func decodeCommentItems(raw []json.RawMessage) ([]*Comment, error) {
	comments := make([]*Comment, 0, len(raw))
	for _, item := range raw {
		var c Comment
		if err := json.Unmarshal(item, &c); err != nil {
			return nil, fmt.Errorf("解析评论失败: %w", err)
		}
		fillCommentContent(&c)
		comments = append(comments, &c)
	}
	return comments, nil
}

// ListComments 获取文档评论列表（不过滤解决状态/范围）。
// userAccessToken 非空时使用 User Token（用户身份），否则使用 App Token（租户身份）。
// 文档归个人所有但 App 未被加为协作者时，App Token 会得到 1069303 forbidden；
// 此时调用方应传入 User Token，让请求以文档所有者身份发出。
func ListComments(fileToken string, fileType string, pageSize int, pageToken, userAccessToken string) ([]*Comment, string, bool, error) {
	return ListCommentsWithOptions(ListCommentsOptions{
		FileToken: fileToken,
		FileType:  fileType,
		PageSize:  pageSize,
		PageToken: pageToken,
	}, userAccessToken)
}

// ListCommentsWithOptions 获取文档评论列表，支持按解决状态 / 评论范围过滤。
// 走原始 HTTP（GET /open-apis/drive/v1/files/:file_token/comments），完整保留服务端 reply_list 结构。
func ListCommentsWithOptions(opts ListCommentsOptions, userAccessToken string) ([]*Comment, string, bool, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	query := url.Values{}
	query.Set("file_type", opts.FileType)
	if opts.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		query.Set("page_token", opts.PageToken)
	}
	if opts.IsSolved != nil {
		query.Set("is_solved", strconv.FormatBool(*opts.IsSolved))
	}
	if opts.IsWhole != nil {
		query.Set("is_whole", strconv.FormatBool(*opts.IsWhole))
	}
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments?%s", url.PathEscape(opts.FileToken), query.Encode())

	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), apiPath, nil, tokenType, reqOpts...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取评论列表失败: %w", err)
	}
	if err := CheckAPIResponse("获取评论列表", resp); err != nil {
		return nil, "", false, err
	}

	var apiResp struct {
		Data struct {
			Items     []json.RawMessage `json:"items"`
			HasMore   bool              `json:"has_more"`
			PageToken string            `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, "", false, fmt.Errorf("解析评论列表响应失败: %w", err)
	}
	comments, err := decodeCommentItems(apiResp.Data.Items)
	if err != nil {
		return nil, "", false, err
	}
	return comments, apiResp.Data.PageToken, apiResp.Data.HasMore, nil
}

// BatchGetComments 按评论 ID 批量获取评论（POST /open-apis/drive/v1/files/:file_token/comments/batch_query）。
// 单次最多 100 个 ID，完整保留服务端 reply_list 结构。
func BatchGetComments(fileToken, fileType string, commentIDs []string, userAccessToken string) ([]*Comment, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments/batch_query?file_type=%s",
		url.PathEscape(fileToken), url.QueryEscape(fileType))
	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, map[string]any{"comment_ids": commentIDs}, tokenType, reqOpts...)
	if err != nil {
		return nil, fmt.Errorf("批量获取评论失败: %w", err)
	}
	if err := CheckAPIResponse("批量获取评论", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析批量评论响应失败: %w", err)
	}
	return decodeCommentItems(apiResp.Data.Items)
}

// CreateComment 创建评论
// userAccessToken 非空时以用户身份创建评论，否则以 App/Bot 身份创建。
// 推荐传入 User Token：Bot 身份发的评论只能被同一 App 自己删除，且很多文档对 Bot 写权限受限。
func CreateComment(fileToken string, fileType string, content string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	textRun := larkdrive.NewTextRunBuilder().
		Text(content).
		Build()
	element := larkdrive.NewReplyElementBuilder().
		Type("text_run").
		TextRun(textRun).
		Build()
	replyContent := larkdrive.NewReplyContentBuilder().
		Elements([]*larkdrive.ReplyElement{element}).
		Build()
	reply := larkdrive.NewFileCommentReplyBuilder().
		Content(replyContent).
		Build()

	req := larkdrive.NewCreateFileCommentReqBuilder().
		FileToken(fileToken).
		FileType(fileType).
		FileComment(larkdrive.NewFileCommentBuilder().
			ReplyList(larkdrive.NewReplyListBuilder().
				Replies([]*larkdrive.FileCommentReply{reply}).
				Build()).
			Build()).
		Build()

	opts := UserTokenOption(userAccessToken)
	resp, err := client.Drive.FileComment.Create(Context(), req, opts...)
	if err != nil {
		return "", fmt.Errorf("创建评论失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("创建评论失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data != nil && resp.Data.CommentId != nil {
		return *resp.Data.CommentId, nil
	}

	return "", nil
}

// GetComment 获取单条评论详情，完整保留服务端 reply_list 结构。
//
// 实测 GET /open-apis/drive/v1/files/:file_token/comments/:comment_id 对 docx 评论返回 1069307 not exist，
// 因此与官方一致改走 comments/batch_query（单个 ID）。
// userAccessToken 非空时使用 User Token，否则使用 App Token；个人文档/未给 App 授权时必须传 User Token。
func GetComment(fileToken string, commentID string, fileType string, userAccessToken string) (*Comment, error) {
	comments, err := BatchGetComments(fileToken, fileType, []string{commentID}, userAccessToken)
	if err != nil {
		return nil, err
	}
	for _, c := range comments {
		if c != nil && c.CommentID == commentID {
			return c, nil
		}
	}
	if len(comments) == 1 && comments[0] != nil {
		return comments[0], nil
	}
	return nil, fmt.Errorf("评论不存在: %s", commentID)
}

// 飞书 Open API 没有「删除整条评论」的端点（SDK fileComment 仅有
// BatchQuery/Create/Get/List/Patch，无 Delete）。评论本质是一条回复线程，
// 只能通过 DeleteCommentReply 删除单条回复，或用 PatchComment 标记已解决。
// 因此此处不提供 DeleteComment，相关指引见 cmd/delete_comment.go。

// PatchComment 更新评论解决状态
// userAccessToken 非空时使用 User Token，否则使用 App Token。
// 个人文档/未给 App 授权时必须传 User Token，否则会得到 1069303 forbidden。
func PatchComment(fileToken, commentID, fileType string, isSolved bool, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewPatchFileCommentReqBuilder().
		FileToken(fileToken).
		CommentId(commentID).
		FileType(fileType).
		Body(larkdrive.NewPatchFileCommentReqBodyBuilder().
			IsSolved(isSolved).
			Build()).
		Build()

	opts := UserTokenOption(userAccessToken)
	resp, err := client.Drive.FileComment.Patch(Context(), req, opts...)
	if err != nil {
		return fmt.Errorf("更新评论状态失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新评论状态失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// CommentReply 评论回复信息。
//
// content 为可读文本（text_run 原文 + @user_id + 链接 URL）；elements 原样保留服务端
// content.elements 结构（text_run / docs_link / person 等），供需要精确结构的调用方使用。
type CommentReply struct {
	ReplyID    string            `json:"reply_id"`
	UserID     string            `json:"user_id,omitempty"`
	Content    string            `json:"content,omitempty"`
	Elements   []json.RawMessage `json:"elements,omitempty"`
	CreateTime int               `json:"create_time,omitempty"`
	UpdateTime int               `json:"update_time,omitempty"`
	// Extra 回复的其他内容（如图片 token），服务端原始结构
	Extra json.RawMessage `json:"extra,omitempty"`
}

// ListCommentReplies 获取评论回复列表（GET /open-apis/drive/v1/files/:file_token/comments/:comment_id/replies）。
// userAccessToken 非空时使用 User Token（用户身份），否则使用 App Token（租户身份）。
func ListCommentReplies(fileToken, commentID, fileType string, pageSize int, pageToken, userAccessToken string) ([]*CommentReply, string, bool, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	query := url.Values{}
	query.Set("file_type", fileType)
	if pageSize > 0 {
		query.Set("page_size", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments/%s/replies?%s",
		url.PathEscape(fileToken), url.PathEscape(commentID), query.Encode())

	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), apiPath, nil, tokenType, reqOpts...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取评论回复列表失败: %w", err)
	}
	if err := CheckAPIResponse("获取评论回复列表", resp); err != nil {
		return nil, "", false, err
	}

	var apiResp struct {
		Data struct {
			Items     []commentReplyWire `json:"items"`
			HasMore   bool               `json:"has_more"`
			PageToken string             `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, "", false, fmt.Errorf("解析评论回复列表响应失败: %w", err)
	}
	replies := make([]*CommentReply, 0, len(apiResp.Data.Items))
	for _, w := range apiResp.Data.Items {
		replies = append(replies, w.toReply())
	}
	return replies, apiResp.Data.PageToken, apiResp.Data.HasMore, nil
}

// DeleteCommentReply 删除评论回复
// 注意：飞书 Open API 只允许回复作者身份删除。用户回复需使用作者的 User Token，
// Bot 回复需使用创建它的同一 App 的 Bot Token。
func DeleteCommentReply(fileToken, commentID, replyID, fileType, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkdrive.NewDeleteFileCommentReplyReqBuilder().
		FileToken(fileToken).
		CommentId(commentID).
		ReplyId(replyID).
		FileType(fileType).
		Build()

	opts := UserTokenOption(userAccessToken)
	resp, err := client.Drive.FileCommentReply.Delete(Context(), req, opts...)
	if err != nil {
		return fmt.Errorf("删除评论回复失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除评论回复失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// CreateCommentReply 为已有评论添加回复
//
// 飞书 Open SDK v3.5.3 尚未封装此接口（只暴露 List/Delete/Update），
// 此处用通用 HTTP client 直接调用 Open API：
//
//	POST /open-apis/drive/v1/files/{file_token}/comments/{comment_id}/replies?file_type=docx
//
// 权限要求（User Token）：docs:document.comment:create
// App Token 同样可以调用（tenant 身份），此时回复人显示为 Bot；删除该回复时需继续使用
// 创建它的同一 App 身份，用户身份创建的回复则需使用作者的 User Token。
func CreateCommentReply(fileToken, commentID, fileType, content, userAccessToken string) (*CommentReply, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"content": map[string]any{
			"elements": []map[string]any{
				{
					"type": "text_run",
					"text_run": map[string]any{
						"text": content,
					},
				},
			},
		},
	}

	apiPath := fmt.Sprintf(
		"/open-apis/drive/v1/files/%s/comments/%s/replies?file_type=%s",
		url.PathEscape(fileToken),
		url.PathEscape(commentID),
		url.QueryEscape(fileType),
	)

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建评论回复失败: %w", err)
	}
	// 先解析业务信封再看 HTTP 状态：飞书业务错误常随 HTTP 400 下发
	if err := CheckAPIResponse("创建评论回复", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data commentReplyWire `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return apiResp.Data.toReply(), nil
}

// UpdateCommentReply 整体替换一条评论回复的内容
// （PUT /open-apis/drive/v1/files/:file_token/comments/:comment_id/replies/:reply_id?file_type=）。
// elements 为服务端 v1 结构（text_run / docs_link / person）。只有回复作者身份可以修改，否则服务端返回 1069303。
func UpdateCommentReply(fileToken, commentID, replyID, fileType string, elements []map[string]any, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/comments/%s/replies/%s?file_type=%s",
		url.PathEscape(fileToken), url.PathEscape(commentID), url.PathEscape(replyID), url.QueryEscape(fileType))
	body := map[string]any{"content": map[string]any{"elements": elements}}
	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Put(Context(), apiPath, body, tokenType, reqOpts...)
	if err != nil {
		return fmt.Errorf("更新评论回复失败: %w", err)
	}
	return CheckAPIResponse("更新评论回复", resp)
}

// ReactCommentReply 对评论回复添加或取消表情回应
// （POST /open-apis/drive/v2/files/:file_token/comments/reaction?file_type=，body {action, reaction_type, reply_id}）。
// action 为 add / delete；add 与 delete 都是幂等的，delete 只取消当前身份的回应。
func ReactCommentReply(fileToken, fileType, replyID, reactionType, action, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	apiPath := fmt.Sprintf("/open-apis/drive/v2/files/%s/comments/reaction?file_type=%s",
		url.PathEscape(fileToken), url.QueryEscape(fileType))
	body := map[string]any{
		"action":        action,
		"reaction_type": reactionType,
		"reply_id":      replyID,
	}
	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, body, tokenType, reqOpts...)
	if err != nil {
		return fmt.Errorf("更新回复表情失败: %w", err)
	}
	return CheckAPIResponse("更新回复表情", resp)
}
