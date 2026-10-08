package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// 多维表格附件素材：parent_type 固定 bitable_file，parent_node 为 base_token。
const (
	BitableAttachmentParentType = "bitable_file"
	// BitableAttachmentMaxFileSize 单个附件上限 2GB（与官方 +record-upload-attachment 一致）。
	BitableAttachmentMaxFileSize int64 = 2 * 1024 * 1024 * 1024
)

// UploadBitableAttachment 把本地文件上传为多维表格附件素材，返回 file_token。
//
//   - ≤20MB：POST /drive/v1/medias/upload_all（单次上传上限 20MB）
//   - >20MB：upload_prepare → upload_part（逐片）→ upload_finish，与官方一致自动分片
//
// 上传后还需 append_attachments 把 file_token 追加到单元格（由调用方完成）。
func UploadBitableAttachment(filePath, baseToken, userAccessToken string) (string, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	if stat.IsDir() {
		return "", fmt.Errorf("%s 是目录，不是文件", filePath)
	}
	if stat.Size() > BitableAttachmentMaxFileSize {
		return "", fmt.Errorf("文件 %s 大小 %s 超过多维表格附件上限 2GB", filePath, formatSize(int(stat.Size())))
	}
	fileName := filepath.Base(filePath)
	if !DriveNeedsMultipart(stat.Size()) {
		token, _, err := UploadMedia(filePath, BitableAttachmentParentType, baseToken, fileName, userAccessToken)
		return token, err
	}
	return uploadBitableAttachmentMultipart(filePath, fileName, stat.Size(), baseToken, userAccessToken)
}

func uploadBitableAttachmentMultipart(filePath, fileName string, fileSize int64, baseToken, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_prepare", map[string]any{
		"file_name":   fileName,
		"parent_type": BitableAttachmentParentType,
		"parent_node": baseToken,
		"size":        fileSize,
	}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("初始化附件分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("初始化附件分片上传", prepareResp); err != nil {
		return "", err
	}
	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, fileSize)
	if err != nil {
		return "", fmt.Errorf("初始化附件分片上传失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "附件分片上传: %s，%d 片 × %s\n", fileName, session.BlockNum, formatSize(int(session.BlockSize)))

	src, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	buffer := make([]byte, int(session.BlockSize))
	remaining := fileSize
	for seq := int64(0); seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if chunkSize > remaining {
			chunkSize = remaining
		}
		n, readErr := io.ReadFull(src, buffer[:int(chunkSize)])
		if readErr != nil {
			return "", fmt.Errorf("读取附件分片失败: %w", readErr)
		}
		fd := larkcore.NewFormdata().
			AddField("upload_id", session.UploadID).
			AddField("seq", fmt.Sprintf("%d", seq)).
			AddField("size", fmt.Sprintf("%d", n)).
			AddFile("file", bytes.NewReader(buffer[:n]))
		partResp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/medias/upload_part", fd, tokenType, opts...)
		if err != nil {
			return "", fmt.Errorf("上传附件分片 %d/%d 失败: %w", seq+1, session.BlockNum, err)
		}
		if err := CheckAPIResponse(fmt.Sprintf("上传附件分片 %d/%d", seq+1, session.BlockNum), partResp); err != nil {
			return "", err
		}
		fmt.Fprintf(os.Stderr, "  分片 %d/%d 上传完成 (%s)\n", seq+1, session.BlockNum, formatSize(n))
		remaining -= int64(n)
	}
	if remaining != 0 {
		return "", fmt.Errorf("upload_prepare 分片计划不一致: 结束后仍剩 %d 字节", remaining)
	}

	finishResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_finish", map[string]any{
		"upload_id": session.UploadID,
		"block_num": session.BlockNum,
	}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("完成附件分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成附件分片上传", finishResp); err != nil {
		return "", err
	}
	var apiResp struct {
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(finishResp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析附件分片上传完成响应失败: %w", err)
	}
	if apiResp.Data.FileToken == "" {
		return "", fmt.Errorf("附件分片上传完成但未返回 file_token")
	}
	return apiResp.Data.FileToken, nil
}
