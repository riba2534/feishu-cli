package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// UploadDocMedia 上传文档素材（docx_image / docx_file 等）并返回 file_token。
//
// 对齐官方 doc_media_upload：≤20MB 走 medias/upload_all；>20MB 走
// upload_prepare / upload_part / upload_finish 分片上传（此前文档素材只有 upload_all，
// 超过 20MB 的图片/附件/视频直接失败）。extra 携带 drive_route_token=docID，
// 素材按文档路由鉴权。
func UploadDocMedia(filePath, parentType, parentNode, fileName, docID, userAccessToken string) (string, error) {
	// 本地文件内容会上传到飞书：拒绝敏感目录（~/.ssh、~/.feishu-cli、/etc 等），命令层漏校验时兜底
	if err := safefile.ValidateInputPath(filePath); err != nil {
		return "", err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	if !stat.Mode().IsRegular() {
		return "", fmt.Errorf("%s 不是普通文件", filePath)
	}
	if stat.Size() <= 0 {
		return "", fmt.Errorf("文件为空: %s", filePath)
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	extra := ""
	if docID != "" {
		b, _ := json.Marshal(map[string]string{"drive_route_token": docID})
		extra = string(b)
	}
	if !DriveNeedsMultipart(stat.Size()) {
		token, _, err := UploadMediaWithExtra(filePath, parentType, parentNode, fileName, extra, userAccessToken)
		return token, err
	}
	return uploadDocMediaMultipart(filePath, parentType, parentNode, fileName, extra, stat.Size(), userAccessToken)
}

// uploadDocMediaMultipart 分片上传文档素材。分片按服务端 upload_prepare 返回的计划切分，
// 单个分片遇到限流/5xx 时重试（同一 upload_id + seq 重传是幂等的）。
func uploadDocMediaMultipart(filePath, parentType, parentNode, fileName, extra string, fileSize int64, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	prepareBody := map[string]any{
		"file_name":   fileName,
		"parent_type": parentType,
		"parent_node": parentNode, // upload_prepare 要求显式携带
		"size":        fileSize,
	}
	if extra != "" {
		prepareBody["extra"] = extra
	}
	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_prepare", prepareBody, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("初始化素材分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("初始化素材分片上传", prepareResp); err != nil {
		return "", err
	}
	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, fileSize)
	if err != nil {
		return "", fmt.Errorf("初始化素材分片上传失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "素材分片上传: %s，%d 片 × %s\n", fileName, session.BlockNum, formatSize(int(session.BlockSize)))

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
			return "", fmt.Errorf("读取素材分片 %d 失败: %w", seq+1, readErr)
		}
		chunk := buffer[:n]
		res := DoVoidWithRetry(func() (http.Header, error) {
			fd := larkcore.NewFormdata().
				AddField("upload_id", session.UploadID).
				AddField("seq", fmt.Sprintf("%d", seq)).
				AddField("size", fmt.Sprintf("%d", n)).
				AddFile("file", bytes.NewReader(chunk))
			partResp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/medias/upload_part", fd, tokenType, opts...)
			if err != nil {
				return nil, err
			}
			return partResp.Header, CheckAPIResponse(fmt.Sprintf("上传素材分片 %d/%d", seq+1, session.BlockNum), partResp)
		}, RetryConfig{MaxRetries: 3, MaxTotalAttempts: 6, RetryOnRateLimit: true})
		if res.Err != nil {
			return "", fmt.Errorf("上传素材分片 %d/%d 失败: %w", seq+1, session.BlockNum, res.Err)
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
		return "", fmt.Errorf("完成素材分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成素材分片上传", finishResp); err != nil {
		return "", err
	}
	var out struct {
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(finishResp.RawBody, &out); err != nil {
		return "", fmt.Errorf("解析素材分片上传结果失败: %w", err)
	}
	if out.Data.FileToken == "" {
		return "", fmt.Errorf("素材分片上传完成但未返回 file_token")
	}
	return out.Data.FileToken, nil
}
