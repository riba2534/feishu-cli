package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

const (
	maxIMFileSize  = 30 << 20 // 30 MB，IM 文件上传限制
	maxIMImageSize = 10 << 20 // 10 MB，IM 图片上传限制
)

// fileExtToIMType maps common file extensions to Feishu IM file_type values.
// Falls back to "stream" for unknown extensions.
func fileExtToIMType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".opus", ".ogg":
		return "opus"
	case ".mp4":
		return "mp4"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "doc"
	case ".xls", ".xlsx":
		return "xls"
	case ".ppt", ".pptx":
		return "ppt"
	default:
		return "stream"
	}
}

// UploadIMFile uploads a local file via the IM API (/open-apis/im/v1/files)
// and returns the file_key. file_type is inferred from the filename.
func UploadIMFile(filePath string, fileName string) (string, error) {
	return UploadIMFileWithOptions(filePath, fileName, "", 0)
}

// UploadIMFileWithOptions 上传 IM 文件，并允许调用方指定 file_type 与音视频时长。
// fileType 为空时按文件扩展名推断；durationMs <= 0 时不提交 duration。
func UploadIMFileWithOptions(filePath string, fileName string, fileType string, durationMs int) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	if stat.Size() > maxIMFileSize {
		return "", fmt.Errorf("文件大小 %s 超过 IM 上传限制（30MB），请使用 file upload 命令上传到云空间", formatSize(int(stat.Size())))
	}
	if stat.Size() == 0 {
		return "", fmt.Errorf("文件为空，飞书 IM 不允许上传空文件")
	}

	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	if fileType == "" {
		fileType = fileExtToIMType(fileName)
	}
	switch fileType {
	case "opus", "mp4", "pdf", "doc", "xls", "ppt", "stream":
	default:
		return "", fmt.Errorf("无效的 IM file_type: %s", fileType)
	}

	bodyBuilder := larkim.NewCreateFileReqBodyBuilder().
		FileType(fileType).
		FileName(fileName).
		File(f)
	if durationMs > 0 {
		bodyBuilder.Duration(durationMs)
	}
	req := larkim.NewCreateFileReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Im.File.Create(ContextWithTimeout(downloadTimeout), req)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("上传文件失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.FileKey == nil {
		return "", fmt.Errorf("上传文件成功但未返回 file_key")
	}

	return *resp.Data.FileKey, nil
}

// UploadIMImage uploads a local image via the IM API (/open-apis/im/v1/images)
// and returns the image_key that can be used directly in msg send --msg-type image.
// imageType can be "message" (default) for sending as message image, or "avatar" for avatar images.
func UploadIMImage(filePath string, imageType string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("获取图片信息失败: %w", err)
	}
	if stat.Size() > maxIMImageSize {
		return "", fmt.Errorf("图片大小 %s 超过 IM 上传限制（10MB）", formatSize(int(stat.Size())))
	}

	if imageType == "" {
		imageType = "message"
	}

	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType(imageType).
			Image(f).
			Build()).
		Build()

	resp, err := client.Im.Image.Create(ContextWithTimeout(downloadTimeout), req)
	if err != nil {
		return "", fmt.Errorf("上传图片失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("上传图片失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("上传图片成功但未返回 image_key")
	}

	return *resp.Data.ImageKey, nil
}
