package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// DocumentCover 是 docx 文档封面（GET /open-apis/docx/v1/documents/:document_id 的 document.cover）。
// Token 为空表示文档没有封面。偏移比例为视图相对原图中心的偏移 / 原图边长，0 为居中。
type DocumentCover struct {
	Token        string   `json:"token"`
	OffsetRatioX *float64 `json:"offset_ratio_x,omitempty"`
	OffsetRatioY *float64 `json:"offset_ratio_y,omitempty"`
}

// GetDocumentCover 读取文档封面元数据；文档没有封面时返回 Token 为空的 DocumentCover。
func GetDocumentCover(documentID, userAccessToken string) (*DocumentCover, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), "/open-apis/docx/v1/documents/"+url.PathEscape(documentID), nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("读取文档封面失败: %w", err)
	}
	if err := CheckAPIResponse("读取文档封面", resp); err != nil {
		return nil, err
	}
	var out struct {
		Data struct {
			Document struct {
				Cover *DocumentCover `json:"cover"`
			} `json:"document"`
			Cover *DocumentCover `json:"cover"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &out); err != nil {
		return nil, fmt.Errorf("解析文档封面失败: %w", err)
	}
	cover := out.Data.Document.Cover
	if cover == nil {
		cover = out.Data.Cover
	}
	if cover == nil {
		cover = &DocumentCover{}
	}
	return cover, nil
}

// UpdateDocumentCover 设置或删除文档封面：PATCH /open-apis/docx/v1/documents/:document_id，
// body {"update_cover":{"cover":{...}}}；cover 为 nil 时发送 "cover": null 表示删除封面。
func UpdateDocumentCover(documentID string, cover *DocumentCover, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	var coverBody any // nil 序列化为 JSON null
	if cover != nil {
		coverBody = cover
	}
	body := map[string]any{"update_cover": map[string]any{"cover": coverBody}}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Patch(Context(), "/open-apis/docx/v1/documents/"+url.PathEscape(documentID), body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("更新文档封面失败: %w", err)
	}
	return CheckAPIResponse("更新文档封面", resp)
}

// MediaPreviewTypeSourceFile 是 preview_download 的 preview_type=16（返回源文件），与官方 +media-preview 一致。
const MediaPreviewTypeSourceFile = "16"

// OpenMediaDownload 打开素材下载流：GET /open-apis/drive/v1/medias/:file_token/download（文档封面图等）。
// timeout > 0 时为总时长上限；0 表示只受空闲超时约束。
func OpenMediaDownload(fileToken, userAccessToken string, timeout time.Duration) (*DriveDownload, error) {
	return openBearerMediaDownload("下载素材", "/open-apis/drive/v1/medias/"+url.PathEscape(fileToken)+"/download", userAccessToken, timeout)
}

// OpenMediaPreviewDownload 打开素材预览下载流：
// GET /open-apis/drive/v1/medias/:file_token/preview_download?preview_type=16，支持文档素材与评论图片 token。
func OpenMediaPreviewDownload(fileToken, userAccessToken string, timeout time.Duration) (*DriveDownload, error) {
	apiPath := "/open-apis/drive/v1/medias/" + url.PathEscape(fileToken) + "/preview_download?preview_type=" + MediaPreviewTypeSourceFile
	return openBearerMediaDownload("预览素材", apiPath, userAccessToken, timeout)
}

func openBearerMediaDownload(action, apiPath, userAccessToken string, timeout time.Duration) (*DriveDownload, error) {
	bearer, err := driveDownloadBearer(userAccessToken)
	if err != nil {
		return nil, err
	}
	ctx, cancel := downloadContext(timeout)
	stream, err := openDownloadStream(ctx, downloadStreamSpec{
		Action: action,
		URL:    buildOpenAPIURL(apiPath),
		Bearer: bearer,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	return &DriveDownload{stream: stream, cancel: cancel}, nil
}
