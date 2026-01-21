package client

import (
	"fmt"

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
	Content      *CommentContent `json:"reply_list,omitempty"`
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
			comment := &Comment{}
			if item.CommentId != nil {
				comment.CommentID = *item.CommentId
			}
			if item.UserId != nil {
				comment.UserID = *item.UserId
			}
			if item.CreateTime != nil {
				comment.CreateTime = *item.CreateTime
			}
			if item.UpdateTime != nil {
				comment.UpdateTime = *item.UpdateTime
			}
			if item.IsSolved != nil {
				comment.IsSolved = *item.IsSolved
			}
			if item.SolvedTime != nil {
				comment.SolvedTime = *item.SolvedTime
			}
			if item.SolverUserId != nil {
				comment.SolverUserID = *item.SolverUserId
			}
			if item.IsWhole != nil {
				comment.IsWhole = *item.IsWhole
			}
			comments = append(comments, comment)
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		if resp.Data.PageToken != nil {
			nextPageToken = *resp.Data.PageToken
		}
		if resp.Data.HasMore != nil {
			hasMore = *resp.Data.HasMore
		}
	}

	return comments, nextPageToken, hasMore, nil
}

// CreateComment 创建评论
func CreateComment(fileToken string, fileType string, content string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	// 构建评论内容
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

	comment := &Comment{}
	if resp.Data.CommentId != nil {
		comment.CommentID = *resp.Data.CommentId
	}
	if resp.Data.UserId != nil {
		comment.UserID = *resp.Data.UserId
	}
	if resp.Data.CreateTime != nil {
		comment.CreateTime = *resp.Data.CreateTime
	}
	if resp.Data.IsSolved != nil {
		comment.IsSolved = *resp.Data.IsSolved
	}
	if resp.Data.IsWhole != nil {
		comment.IsWhole = *resp.Data.IsWhole
	}

	return comment, nil
}

// DeleteComment 删除评论
// 注意：当前飞书 SDK 版本不支持删除评论 API
func DeleteComment(fileToken string, commentID string, fileType string) error {
	return fmt.Errorf("删除评论功能暂不支持：当前 SDK 版本未提供删除评论 API")
}
