package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

// Comment 评论信息
type Comment struct {
	CommentID    string          `json:"comment_id"`
	UserID       string          `json:"user_id,omitempty"`
	CreateTime   int             `json:"create_time,omitempty"`
	UpdateTime   int             `json:"update_time,omitempty"`
	IsSolved     bool            `json:"is_solved"`
	SolvedTime   int             `json:"solved_time,omitempty"`
	SolverUserID string          `json:"solver_user_id,omitempty"`
	IsWhole      bool            `json:"is_whole"`
	// Quote 划词评论选中的原文；IsWhole=true 时为空
	Quote   string          `json:"quote,omitempty"`
	Content *CommentContent `json:"reply_list,omitempty"`
}

// CommentContent 评论内容
type CommentContent struct {
	Elements []CommentElement `json:"elements,omitempty"`
}

// CommentElement 评论元素
type CommentElement struct {
	Type     string `json:"type"`
	TextRun  string `json:"text_run,omitempty"`
	DocsLink string `json:"docs_link,omitempty"`
	Person   string `json:"person,omitempty"`
}

// ListComments 获取文档评论列表
func ListComments(fileToken string, fileType string, pageSize int, pageToken string) ([]*Comment, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkdrive.NewListFileCommentReqBuilder().
		FileToken(fileToken).
		FileType(fileType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Drive.FileComment.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, "", false, fmt.Errorf("获取评论列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取评论列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var comments []*Comment
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			comments = append(comments, &Comment{
				CommentID:    StringVal(item.CommentId),
				UserID:       StringVal(item.UserId),
				CreateTime:   IntVal(item.CreateTime),
				UpdateTime:   IntVal(item.UpdateTime),
				IsSolved:     BoolVal(item.IsSolved),
				SolvedTime:   IntVal(item.SolvedTime),
				SolverUserID: StringVal(item.SolverUserId),
				IsWhole:      BoolVal(item.IsWhole),
				Quote:        StringVal(item.Quote),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return comments, nextPageToken, hasMore, nil
}

// CreateComment 创建评论
func CreateComment(fileToken string, fileType string, content string) (string, error) {
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

	resp, err := client.Drive.FileComment.Create(Context(), req)
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

// GetComment 获取评论详情
func GetComment(fileToken string, commentID string, fileType string) (*Comment, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdrive.NewGetFileCommentReqBuilder().
		FileToken(fileToken).
		CommentId(commentID).
		FileType(fileType).
		Build()

	resp, err := client.Drive.FileComment.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取评论详情失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取评论详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("评论不存在")
	}

	return &Comment{
		CommentID:  StringVal(resp.Data.CommentId),
		UserID:     StringVal(resp.Data.UserId),
		CreateTime: IntVal(resp.Data.CreateTime),
		IsSolved:   BoolVal(resp.Data.IsSolved),
		IsWhole:    BoolVal(resp.Data.IsWhole),
		Quote:      StringVal(resp.Data.Quote),
	}, nil
}

// DeleteComment 删除评论
// 注意：当前飞书 SDK 版本不支持删除评论 API
func DeleteComment(fileToken string, commentID string, fileType string) error {
	return fmt.Errorf("删除评论功能暂不支持：当前 SDK 版本未提供删除评论 API")
}

// PatchComment 更新评论解决状态
func PatchComment(fileToken, commentID, fileType string, isSolved bool) error {
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

	resp, err := client.Drive.FileComment.Patch(Context(), req)
	if err != nil {
		return fmt.Errorf("更新评论状态失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新评论状态失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// CommentReply 评论回复信息
type CommentReply struct {
	ReplyID    string `json:"reply_id"`
	UserID     string `json:"user_id,omitempty"`
	Content    string `json:"content,omitempty"`
	CreateTime int    `json:"create_time,omitempty"`
	UpdateTime int    `json:"update_time,omitempty"`
}

// ListCommentReplies 获取评论回复列表
// userAccessToken 非空时使用 User Token（用户身份），否则使用 App Token（租户身份）。
func ListCommentReplies(fileToken, commentID, fileType string, pageSize int, pageToken, userAccessToken string) ([]*CommentReply, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkdrive.NewListFileCommentReplyReqBuilder().
		FileToken(fileToken).
		CommentId(commentID).
		FileType(fileType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	opts := UserTokenOption(userAccessToken)
	resp, err := client.Drive.FileCommentReply.List(Context(), reqBuilder.Build(), opts...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取评论回复列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取评论回复列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var replies []*CommentReply
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			var content string
			if item.Content != nil && item.Content.Elements != nil {
				for _, el := range item.Content.Elements {
					if el != nil && el.TextRun != nil && el.TextRun.Text != nil {
						content += *el.TextRun.Text
					}
				}
			}
			replies = append(replies, &CommentReply{
				ReplyID:    StringVal(item.ReplyId),
				UserID:     StringVal(item.UserId),
				Content:    content,
				CreateTime: IntVal(item.CreateTime),
				UpdateTime: IntVal(item.UpdateTime),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return replies, nextPageToken, hasMore, nil
}

// DeleteCommentReply 删除评论回复
// 注意：飞书 Open API 只允许回复作者本人删除（App Bot 身份会得到 1069303 forbidden），
// 因此调用方几乎总是需要提供 userAccessToken（回复作者的 User Token）。
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
// App Token 同样可以调用（tenant 身份），但飞书侧多数场景推荐用户身份发起回复，
// 否则回复人会显示为 Bot，且该回复无法通过 DeleteCommentReply 删除（只有作者能删）。
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建评论回复失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ReplyID    string `json:"reply_id"`
			UserID     string `json:"user_id"`
			CreateTime int    `json:"create_time"`
			UpdateTime int    `json:"update_time"`
			Content    *struct {
				Elements []struct {
					Type    string `json:"type"`
					TextRun *struct {
						Text string `json:"text"`
					} `json:"text_run,omitempty"`
				} `json:"elements"`
			} `json:"content,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建评论回复失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	reply := &CommentReply{
		ReplyID:    apiResp.Data.ReplyID,
		UserID:     apiResp.Data.UserID,
		CreateTime: apiResp.Data.CreateTime,
		UpdateTime: apiResp.Data.UpdateTime,
	}
	if apiResp.Data.Content != nil {
		for _, el := range apiResp.Data.Content.Elements {
			if el.TextRun != nil {
				reply.Content += el.TextRun.Text
			}
		}
	}
	return reply, nil
}
