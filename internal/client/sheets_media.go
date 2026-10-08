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

// sheets_media.go —— 电子表格图片素材上传：≤20MB 走 medias/upload_all，>20MB 走
// medias/upload_prepare → upload_part → upload_finish（对齐官方 uploadSheetImage）。
// upload_all 对超限文件只回一个不含大小信息的 1061002 "params error"，调用方无从排查，
// 因此按大小在本地分流。

// UploadSheetImageMediaAuto 上传本地图片作为电子表格素材并返回 file_token，按文件大小自动选择
// 单次上传或分片上传；parent_type 按表格 token 自动选择（见 sheetMediaParentType）。
func UploadSheetImageMediaAuto(filePath, spreadsheetToken, fileName string, userAccessToken ...string) (string, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("读取图片文件信息失败: %w", err)
	}
	if !stat.Mode().IsRegular() {
		return "", fmt.Errorf("%s 不是普通文件", filePath)
	}
	if !DriveNeedsMultipart(stat.Size()) {
		return UploadSheetImageMedia(filePath, spreadsheetToken, fileName, userAccessToken...)
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	token, err := uploadMediaMultipart(filePath, fileName, stat.Size(), sheetMediaParentType(spreadsheetToken), spreadsheetToken, firstString(userAccessToken))
	if err != nil {
		return "", fmt.Errorf("上传浮动图片素材失败: %w", err)
	}
	return token, nil
}

// SheetImageUsesMultipart 报告该文件上传时是否会走分片接口（供 dry-run / 提示使用，stat 失败按单次上传处理）。
func SheetImageUsesMultipart(filePath string) bool {
	stat, err := os.Stat(filePath)
	if err != nil || !stat.Mode().IsRegular() {
		return false
	}
	return DriveNeedsMultipart(stat.Size())
}

// uploadMediaMultipart 以 medias 分片接口上传文件，parent_node 为素材挂载的文档（这里是表格 token）。
func uploadMediaMultipart(filePath, fileName string, fileSize int64, parentType, parentNode, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	prepareResp, err := cli.Post(Context(), "/open-apis/drive/v1/medias/upload_prepare", map[string]any{
		"file_name":   fileName,
		"parent_type": parentType,
		"parent_node": parentNode,
		"size":        fileSize,
	}, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("初始化分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("初始化分片上传", prepareResp); err != nil {
		return "", err
	}
	session, err := parseMultipartSessionFromAPI(prepareResp.RawBody, fileSize)
	if err != nil {
		return "", fmt.Errorf("初始化分片上传失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "图片素材分片上传: %s，%d 片 × %s\n", fileName, session.BlockNum, formatSize(int(session.BlockSize)))

	src, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	buffer := make([]byte, int(session.BlockSize))
	remaining := fileSize
	for seq := int64(0); seq < session.BlockNum; seq++ {
		chunkSize := session.BlockSize
		if remaining > 0 && chunkSize > remaining {
			chunkSize = remaining
		}
		n, readErr := io.ReadFull(src, buffer[:int(chunkSize)])
		if readErr != nil {
			return "", fmt.Errorf("读取分片失败: %w", readErr)
		}
		fd := larkcore.NewFormdata().
			AddField("upload_id", session.UploadID).
			AddField("seq", fmt.Sprintf("%d", seq)).
			AddField("size", fmt.Sprintf("%d", n)).
			AddFile("file", bytes.NewReader(buffer[:n]))
		partResp, err := cli.Post(ContextWithTimeout(downloadTimeout), "/open-apis/drive/v1/medias/upload_part", fd, tokenType, opts...)
		if err != nil {
			return "", fmt.Errorf("上传分片 %d/%d 失败: %w", seq+1, session.BlockNum, err)
		}
		if err := CheckAPIResponse(fmt.Sprintf("上传分片 %d/%d", seq+1, session.BlockNum), partResp); err != nil {
			return "", err
		}
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
		return "", fmt.Errorf("完成分片上传失败: %w", err)
	}
	if err := CheckAPIResponse("完成分片上传", finishResp); err != nil {
		return "", err
	}
	var apiResp struct {
		Data struct {
			FileToken string `json:"file_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(finishResp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析 upload_finish 响应失败: %w", err)
	}
	if apiResp.Data.FileToken == "" {
		return "", fmt.Errorf("分片上传完成但未返回 file_token")
	}
	return apiResp.Data.FileToken, nil
}
