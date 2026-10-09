package cmd

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
)

// savedMedia 是素材下载落盘结果。
type savedMedia struct {
	Path        string
	Size        int64
	ContentType string
}

// saveMediaDownload 把已打开的素材下载流保存到 output，规则与 doc media-download 一致：
//   - output 带扩展名：直接原子写入；
//   - output 不带扩展名：先写同目录临时文件，按内容识别扩展名（识别不出再看 Content-Type /
//     Content-Disposition），校验最终路径与覆盖后再原子改名，失败不留半截文件。
//
// 目标已存在且未传 overwrite 时以退出码 2 拒绝。
func saveMediaDownload(d *client.DriveDownload, output string, overwrite bool) (*savedMedia, error) {
	contentType := d.Header().Get("Content-Type")
	if mediaHasExplicitExtension(output) {
		if err := ensureNotOverwriting(output, overwrite); err != nil {
			return nil, err
		}
		n, err := d.SaveTo(output)
		if err != nil {
			return nil, err
		}
		return &savedMedia{Path: output, Size: n, ContentType: contentType}, nil
	}

	tmp, err := os.CreateTemp(filepath.Dir(output), ".feishu-media-*.part")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	n, err := d.SaveTo(tmpPath)
	if err != nil {
		return nil, err
	}
	finalPath := output
	ext := sniffMediaExtension(tmpPath)
	if ext == "" {
		ext = mediaExtensionFromHeader(d.Header())
	}
	if ext != "" {
		finalPath = strings.TrimSuffix(output, ".") + ext
	}
	if err := validateOutputPath(finalPath, ""); err != nil {
		return nil, fmt.Errorf("输出路径不安全: %w", err)
	}
	if err := ensureNotOverwriting(finalPath, overwrite); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}
	_ = os.Chmod(finalPath, 0o644)
	return &savedMedia{Path: finalPath, Size: n, ContentType: contentType}, nil
}

// mediaMimeExtensions 是按响应头补扩展名时的 MIME 映射（内容识别不出时兜底）。
var mediaMimeExtensions = map[string]string{
	"application/msword":            ".doc",
	"application/pdf":               ".pdf",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/xml": ".xml",
	"application/zip": ".zip",
	"audio/mpeg":      ".mp3",
	"image/bmp":       ".bmp",
	"image/gif":       ".gif",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/svg+xml":   ".svg",
	"image/tiff":      ".tiff",
	"image/webp":      ".webp",
	"text/csv":        ".csv",
	"text/html":       ".html",
	"text/plain":      ".txt",
	"text/xml":        ".xml",
	"video/mp4":       ".mp4",
}

// mediaExtensionFromHeader 按 Content-Type、再按 Content-Disposition 文件名推断扩展名；推断不出返回空串。
func mediaExtensionFromHeader(h http.Header) string {
	if h == nil {
		return ""
	}
	if ct := h.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil {
			mediaType = strings.TrimSpace(strings.Split(ct, ";")[0])
		}
		if ext, ok := mediaMimeExtensions[strings.ToLower(mediaType)]; ok {
			return ext
		}
	}
	if cd := h.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			name := params["filename"]
			if ext := filepath.Ext(filepath.Base(name)); ext != "" && ext != "." && !strings.ContainsAny(ext, `/\`) {
				return ext
			}
		}
	}
	return ""
}
